package prwatch

import (
	"cmp"
	"context"
	"errors"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (s *Service) gates(w store.Watch) bool {
	return s.hosted(w) && s.runs(w) && w.WorktreeDir != ""
}

func (s *Service) pushes(w store.Watch) bool {
	return !agent.IsDependabot(w.Author)
}

func (s *Service) quiet(w store.Watch) bool {
	l := s.sessions.get(w.ID)
	return l == nil || l.State().EndsTurn()
}

func (s *Service) syncWork(ctx context.Context, w store.Watch) {
	if !s.gates(w) || !s.quiet(w) {
		return
	}
	if _, open, err := s.store.ActiveProposal(ctx, w.ID); err != nil || open {
		return
	}
	remote, err := s.rel.Fetch(ctx, w.WorktreeDir, w.HeadRef)
	if err != nil {
		s.log.Warn("read the pull request branch before a message", "watch", w.ID, "err", err)
		return
	}
	work, err := s.rel.Head(ctx, w.WorktreeDir)
	if err != nil || work == remote {
		return
	}
	if !s.pushes(w) {
		s.follow(ctx, w, remote)
		return
	}
	behind, err := s.rel.Contains(ctx, w.WorktreeDir, remote, work)
	if err != nil || !behind {
		return
	}
	if err := s.rel.FastForward(ctx, w.WorktreeDir, remote); err != nil {
		s.log.Warn("bring the work branch up to the pull request branch", "watch", w.ID, "err", err)
	}
}

func (s *Service) follow(ctx context.Context, w store.Watch, remote string) {
	if err := s.rel.Reset(ctx, w.WorktreeDir, remote); err != nil {
		s.log.Warn("move the work branch to the pull request branch", "watch", w.ID, "err", err)
	}
}

func (s *Service) workBranch(ctx context.Context, id int64) string {
	w, ok := s.gatedWatch(ctx, id)
	if !ok {
		return ""
	}
	work, err := s.rel.Head(ctx, w.WorktreeDir)
	if err != nil {
		return ""
	}
	return work
}

func (s *Service) startTurn(id int64, work string) {
	ctx := context.WithoutCancel(s.background())
	unlock := s.locks.lock(id)
	defer unlock()
	w, ok := s.gatedWatch(ctx, id)
	if !ok {
		return
	}
	_, err := s.openTurnAt(ctx, w, work)
	if err != nil && !errors.Is(err, ErrProposalPending) {
		s.log.Error("open the proposal of a turn", "watch", id, "err", err)
	}
}

func (s *Service) gatedWatch(ctx context.Context, id int64) (store.Watch, bool) {
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return store.Watch{}, false
	}
	return w, w.Status == store.WatchActive && s.gates(w) && !s.withAuthor(w)
}

func (s *Service) openTurn(ctx context.Context, w store.Watch) (store.Proposal, error) {
	return s.openTurnAt(ctx, w, "")
}

func (s *Service) openTurnAt(ctx context.Context, w store.Watch, start string) (store.Proposal, error) {
	if p, open, err := s.store.ActiveProposal(ctx, w.ID); err != nil || open {
		return p, err
	}
	if err := s.refusePending(ctx, w); err != nil {
		return store.Proposal{}, err
	}
	failed, superseding, err := s.store.LatestProposal(ctx, w.ID, store.ProposalFailed)
	if err != nil {
		return store.Proposal{}, err
	}
	head, base, work := s.turnStart(ctx, w, cmp.Or(w.HandbackStart, start), failed, superseding)
	p, err := s.store.OpenProposal(ctx, w.ID, head, base, work, s.now())
	if err != nil {
		return p, err
	}
	s.clearHandbackStart(ctx, w)
	if !superseding {
		return p, nil
	}
	return p, s.store.SupersedeProposal(ctx, failed.ID, p.ID)
}

func (s *Service) turnStart(ctx context.Context, w store.Watch, start string, failed store.Proposal, superseding bool) (head, base, work string) {
	head, base, work = s.turnBase(ctx, w, start)
	if superseding && s.waitsOnItsHead(ctx, w, failed, head) {
		return failed.HeadSHA, failed.BaseSHA, failed.StartSHA
	}
	return head, base, work
}

func (s *Service) waitsOnItsHead(ctx context.Context, w store.Watch, p store.Proposal, head string) bool {
	if !p.HasPush {
		return false
	}
	onBranch, err := s.rel.Contains(ctx, w.WorktreeDir, head, p.HeadSHA)
	if err != nil || !onBranch {
		return false
	}
	landed, err := s.rel.Contains(ctx, w.WorktreeDir, head, p.WorkSHA)
	return err == nil && !landed
}

func (s *Service) turnBase(ctx context.Context, w store.Watch, start string) (head, base, work string) {
	var workErr error
	work = start
	if work == "" {
		work, workErr = s.rel.Head(ctx, w.WorktreeDir)
	}
	head, err := s.rel.Fetch(ctx, w.WorktreeDir, w.HeadRef)
	if err != nil {
		s.log.Warn("read the pull request branch at the start of a turn", "watch", w.ID, "err", err)
		head = w.HeadSHA
	}
	if workErr != nil {
		return head, head, ""
	}
	base, err = s.rel.MergeBase(ctx, w.WorktreeDir, work, head)
	if err != nil {
		return head, head, work
	}
	return head, base, work
}

func (s *Service) queueEndTurn(id int64, seq uint64) {
	s.spawn(s.turns.queue(id, func() { s.endTurn(id, seq) }))
}

func (s *Service) endTurn(id int64, seq uint64) {
	ctx := context.WithoutCancel(s.background())
	unlock := s.locks.lock(id)
	defer unlock()
	if !s.endsCurrentTurn(id, seq) {
		return
	}
	w, err := s.store.GetWatch(ctx, id)
	if err != nil || w.Status != store.WatchActive {
		return
	}
	p, open, err := s.store.ActiveProposal(ctx, id)
	if err != nil || !open {
		return
	}
	if err := s.closeTurn(ctx, w, p); err != nil {
		s.log.Error("close the turn of the agent", "watch", id, "err", err)
	}
}

func (s *Service) endsCurrentTurn(id int64, seq uint64) bool {
	l := s.sessions.get(id)
	if l == nil {
		return false
	}
	return l.turnSeq() == seq && l.State().EndsTurn()
}

func (s *Service) closeCutTurn(ctx context.Context, w store.Watch) {
	p, open, err := s.store.ActiveProposal(ctx, w.ID)
	if err != nil || !open {
		return
	}
	if err := s.closeTurn(ctx, w, p); err != nil {
		s.log.Error("close the turn a shutdown cut short", "watch", w.ID, "err", err)
	}
}

func (s *Service) closeTurn(ctx context.Context, w store.Watch, p store.Proposal) error {
	work, err := s.rel.Head(ctx, w.WorktreeDir)
	if err != nil {
		return err
	}
	replies, err := s.store.ProposalReplies(ctx, p.ID)
	if err != nil {
		return err
	}
	hasPush := s.pushes(w) && work != p.HeadSHA && work != p.StartSHA
	if !hasPush && len(replies) == 0 {
		return s.store.DeleteProposal(ctx, p.ID)
	}
	if err := s.store.EndProposal(ctx, p.ID, work, hasPush, s.now()); err != nil {
		return err
	}
	p.WorkSHA, p.HasPush = work, hasPush
	heldBack, err := s.carriesHeldBack(ctx, w, p)
	if err != nil {
		return err
	}
	if w.Asks() || heldBack {
		return s.offer(ctx, w, p, false)
	}
	_, err = s.release(ctx, w, p, false)
	return err
}

func (s *Service) carriesHeldBack(ctx context.Context, w store.Watch, p store.Proposal) (bool, error) {
	if !p.HasPush || p.StartSHA == "" {
		return false, nil
	}
	onHead, err := s.rel.Contains(ctx, w.WorktreeDir, p.HeadSHA, p.StartSHA)
	return !onHead, err
}
