package prwatch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

var (
	ErrProposalPending = errors.New("a proposal waits on your approval")
	ErrNotPending      = errors.New("the proposal does not wait on a decision")
	ErrBadApprovalMode = errors.New("invalid approval mode: use auto or manual")
	ErrUnknownCommit   = errors.New("the proposal has no such commit")
	ErrUnknownFile     = errors.New("the proposal does not change such a file")
)

const maxDiff = 1 << 20

type Decision struct {
	Edits      map[int64]string
	Drop       []int64
	RejectPush bool
	StopAsking bool
}

type Rejection struct {
	Reason  string
	Discard bool
}

type ApprovalChange struct {
	Mode              *store.ApprovalMode
	AutoApproveRebase *bool
	Release           bool
}

type ProposalView struct {
	store.Proposal
	Replies []ReplyView
}

type ReplyView struct {
	store.ProposalReply
	Answers *store.Activity
}

type CodeQuery struct {
	Commit string
	Path   string
}

type ProposalDetail struct {
	ProposalView
	ProposalCode
	CodeError string
}

type ProposalCode struct {
	Commits   []gitrelease.Commit
	HeldBack  []string
	Commit    string
	Base      string
	Files     []gitrelease.File
	Diff      string
	Truncated bool
}

func (s *Service) hostedWatch(ctx context.Context, id int64) (store.Watch, error) {
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	if !s.hosted(w) {
		return store.Watch{}, ErrSelfWatch
	}
	return w, nil
}

func (s *Service) activeGatedWatch(ctx context.Context, id int64) (store.Watch, error) {
	w, err := s.hostedWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	if w.Status != store.WatchActive {
		return store.Watch{}, ErrWatchStopped
	}
	return w, nil
}

func (s *Service) Proposals(ctx context.Context, id int64) ([]ProposalView, error) {
	w, err := s.hostedWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	ps, err := s.store.ListProposals(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ProposalView, 0, len(ps))
	for _, p := range ps {
		v, err := s.view(ctx, w, p)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) Proposal(ctx context.Context, id int64, number int, q CodeQuery) (ProposalDetail, error) {
	w, err := s.hostedWatch(ctx, id)
	if err != nil {
		return ProposalDetail{}, err
	}
	p, err := s.store.GetProposal(ctx, w.ID, number)
	if err != nil {
		return ProposalDetail{}, err
	}
	v, err := s.view(ctx, w, p)
	if err != nil {
		return ProposalDetail{}, err
	}
	d := ProposalDetail{ProposalView: v}
	code, err := s.readCode(ctx, w, p, q)
	if errors.Is(err, ErrUnknownCommit) || errors.Is(err, ErrUnknownFile) {
		return ProposalDetail{}, err
	}
	if err != nil {
		s.log.Warn("read the code of a proposal", "watch", w.ID, "proposal", number, "err", err)
		d.CodeError = err.Error()
		return d, nil
	}
	d.ProposalCode = code
	return d, nil
}

func (s *Service) readCode(ctx context.Context, w store.Watch, p store.Proposal, q CodeQuery) (ProposalCode, error) {
	work := p.WorkSHA
	if work == "" {
		head, err := s.rel.Head(ctx, w.WorktreeDir)
		if err != nil {
			return ProposalCode{}, err
		}
		work = head
	}
	onlyReplies := work == p.HeadSHA && q == CodeQuery{}
	if onlyReplies {
		return ProposalCode{}, nil
	}
	var c ProposalCode
	var err error
	if c.Commits, err = s.rel.Log(ctx, w.WorktreeDir, p.HeadSHA, work); err != nil {
		return ProposalCode{}, err
	}
	if c.HeldBack, err = s.heldBack(ctx, w, p); err != nil {
		return ProposalCode{}, err
	}
	c.Base = p.HeadSHA
	to := work
	if q.Commit != "" {
		if to, err = commitOf(c.Commits, q.Commit); err != nil {
			return ProposalCode{}, err
		}
		if c.Base, err = s.rel.Parent(ctx, w.WorktreeDir, to); err != nil {
			return ProposalCode{}, err
		}
		c.Commit = to
	}
	if c.Files, err = s.rel.Files(ctx, w.WorktreeDir, c.Base, to); err != nil {
		return ProposalCode{}, err
	}
	var paths []string
	if q.Path != "" {
		if c.Files, err = fileOf(c.Files, q.Path); err != nil {
			return ProposalCode{}, err
		}
		paths = []string{q.Path}
	}
	if c.Diff, c.Truncated, err = s.rel.Diff(ctx, w.WorktreeDir, c.Base, to, maxDiff, paths...); err != nil {
		return ProposalCode{}, err
	}
	return c, nil
}

func fileOf(files []gitrelease.File, path string) ([]gitrelease.File, error) {
	i := slices.IndexFunc(files, func(f gitrelease.File) bool { return f.Path == path })
	if i < 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnknownFile, path)
	}
	return files[i : i+1], nil
}

func commitOf(commits []gitrelease.Commit, commit string) (string, error) {
	names := func(c gitrelease.Commit) bool { return c.SHA == commit || abbreviates(commit, c.SHA) }
	i := slices.IndexFunc(commits, names)
	if i < 0 {
		return "", fmt.Errorf("%w: %s", ErrUnknownCommit, commit)
	}
	return commits[i].SHA, nil
}

func (s *Service) heldBack(ctx context.Context, w store.Watch, p store.Proposal) ([]string, error) {
	if p.StartSHA == "" || p.StartSHA == p.HeadSHA {
		return nil, nil
	}
	commits, err := s.rel.Log(ctx, w.WorktreeDir, p.HeadSHA, p.StartSHA)
	if err != nil {
		return nil, err
	}
	shas := make([]string, 0, len(commits))
	for _, c := range commits {
		shas = append(shas, c.SHA)
	}
	return shas, nil
}

func (s *Service) view(ctx context.Context, w store.Watch, p store.Proposal) (ProposalView, error) {
	replies, err := s.store.ProposalReplies(ctx, p.ID)
	if err != nil {
		return ProposalView{}, err
	}
	v := ProposalView{Proposal: p, Replies: make([]ReplyView, 0, len(replies))}
	for _, r := range replies {
		rv := ReplyView{ProposalReply: r}
		if r.InReplyTo != 0 {
			row, ok, err := s.store.ActivityByRef(ctx, w.ID, r.InReplyKind.Activity(), fmt.Sprint(r.InReplyTo))
			if err != nil {
				return ProposalView{}, err
			}
			if ok {
				rv.Answers = &row
			}
		}
		v.Replies = append(v.Replies, rv)
	}
	return v, nil
}

func (s *Service) Approve(ctx context.Context, id int64, number int, d Decision) (store.Proposal, error) {
	w, err := s.activeGatedWatch(ctx, id)
	if err != nil {
		return store.Proposal{}, err
	}
	unlock := s.locks.Lock(w.ID)
	defer unlock()
	p, err := s.pendingNumber(ctx, w, number)
	if err != nil {
		return store.Proposal{}, err
	}
	return s.approve(ctx, w, p, d)
}

func (s *Service) pendingNumber(ctx context.Context, w store.Watch, number int) (store.Proposal, error) {
	p, err := s.store.GetProposal(ctx, w.ID, number)
	if err != nil {
		return store.Proposal{}, err
	}
	if p.Status != store.ProposalPending {
		return store.Proposal{}, fmt.Errorf("%w: proposal %d is %s", ErrNotPending, number, p.Status)
	}
	return p, nil
}

func (s *Service) approve(ctx context.Context, w store.Watch, p store.Proposal, d Decision) (store.Proposal, error) {
	told, err := s.applyDecision(ctx, p, d)
	if err != nil {
		return store.Proposal{}, err
	}
	if err := s.store.ApproveProposal(ctx, p.ID, d.RejectPush && p.HasPush, s.now()); err != nil {
		return store.Proposal{}, err
	}
	if p, err = s.store.GetProposal(ctx, w.ID, p.Number); err != nil {
		return store.Proposal{}, err
	}
	if _, err := s.release(ctx, w, p, false); err != nil {
		return store.Proposal{}, err
	}
	if err := s.forgetAnswered(ctx, w, told.unanswered); err != nil {
		return store.Proposal{}, err
	}
	if d.StopAsking {
		if _, err := s.store.SetWatchApproval(ctx, w.ID, store.ApprovalAuto, w.AutoApproveRebase); err != nil {
			return store.Proposal{}, err
		}
	}
	told.PushRejected = d.RejectPush && p.HasPush
	if told.changed() {
		s.tellDecision(ctx, w, p, told)
	}
	s.Kick(w.ID)
	return s.store.GetProposal(ctx, w.ID, p.Number)
}

type decided struct {
	Edited       []agent.DecidedReply
	Dropped      []agent.DecidedReply
	PushRejected bool
	unanswered   []store.SeenItem
}

func (d decided) changed() bool {
	return d.PushRejected || len(d.Edited) > 0 || len(d.Dropped) > 0
}

func (s *Service) applyDecision(ctx context.Context, p store.Proposal, d Decision) (decided, error) {
	replies, err := s.store.ProposalReplies(ctx, p.ID)
	if err != nil {
		return decided{}, err
	}
	byID := map[int64]store.ProposalReply{}
	for _, r := range replies {
		byID[r.ID] = r
	}
	var out decided
	for replyID, body := range d.Edits {
		body = strings.TrimSpace(redact.Text(body))
		if body == "" {
			return decided{}, ErrEmptyReply
		}
		if err := s.store.EditReply(ctx, p.ID, replyID, body); err != nil {
			return decided{}, err
		}
		out.Edited = append(out.Edited, agent.DecidedReply{InReplyTo: byID[replyID].InReplyTo, Text: body})
	}
	for _, replyID := range d.Drop {
		if err := s.store.WithdrawReply(ctx, p.ID, replyID); err != nil {
			return decided{}, err
		}
		out.Dropped = append(out.Dropped, agent.DecidedReply{InReplyTo: byID[replyID].InReplyTo})
		out.unanswered = append(out.unanswered, byID[replyID].Target())
	}
	return out, nil
}

func (s *Service) forgetAnswered(ctx context.Context, w store.Watch, comments []store.SeenItem) error {
	for _, c := range comments {
		if c.ID == 0 {
			continue
		}
		if err := s.store.ForgetReviewItem(ctx, w.Key(), c); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) tellDecision(ctx context.Context, w store.Watch, p store.Proposal, told decided) {
	text, err := agent.DecisionMessage(agent.Decision{
		PR: pullRequestOf(w), Proposal: p.Number, PushRejected: told.PushRejected, Edited: told.Edited, Dropped: told.Dropped,
	})
	if err == nil {
		_, err = s.deliver(ctx, w, text, fmt.Sprintf("told the agent what you changed in proposal %d", p.Number), deliverRoutine, nil)
	}
	if err != nil {
		s.agentFailed(ctx, w, "tell the agent about your decision", err)
	}
}

func (s *Service) Reject(ctx context.Context, id int64, number int, r Rejection) (store.Proposal, error) {
	w, err := s.activeGatedWatch(ctx, id)
	if err != nil {
		return store.Proposal{}, err
	}
	unlock := s.locks.Lock(w.ID)
	defer unlock()
	p, err := s.store.GetProposal(ctx, w.ID, number)
	if err != nil {
		return store.Proposal{}, err
	}
	if !p.Status.Rejectable() {
		return store.Proposal{}, fmt.Errorf("%w: proposal %d is %s", ErrNotPending, number, p.Status)
	}
	out, err := s.wentOut(ctx, w, p)
	if err != nil {
		return store.Proposal{}, err
	}
	if out != "" {
		return store.Proposal{}, fmt.Errorf("%w: part of proposal %d is on GitHub, %s; retry it to send the rest", ErrNotPending, number, out)
	}
	reason := strings.TrimSpace(r.Reason)
	if err := s.store.RejectProposal(ctx, p.ID, reason, s.now()); err != nil {
		return store.Proposal{}, err
	}
	if err := s.forgetUnposted(ctx, w, p); err != nil {
		return store.Proposal{}, err
	}
	discarded := r.Discard && p.HasPush
	if discarded {
		if err := s.rel.Discard(ctx, w.WorktreeDir, p.HeadSHA); err != nil {
			return store.Proposal{}, err
		}
	}
	s.tellRejection(ctx, w, p, reason, discarded)
	s.Kick(w.ID)
	return s.store.GetProposal(ctx, w.ID, number)
}

func (s *Service) wentOut(ctx context.Context, w store.Watch, p store.Proposal) (string, error) {
	if p.Status != store.ProposalFailed {
		return "", nil
	}
	replies, err := s.store.ProposalReplies(ctx, p.ID)
	if err != nil {
		return "", err
	}
	if slices.ContainsFunc(replies, store.ProposalReply.Posted) {
		return "a reply is posted", nil
	}
	if !p.Pushes() {
		return "", nil
	}
	remote, err := s.rel.Fetch(ctx, w.WorktreeDir, w.HeadRef)
	if err != nil {
		return "", err
	}
	landed, err := s.rel.Contains(ctx, w.WorktreeDir, remote, p.WorkSHA)
	if err != nil || !landed {
		return "", err
	}
	return "its commits are on " + w.HeadRef, nil
}

func (s *Service) forgetUnposted(ctx context.Context, w store.Watch, p store.Proposal) error {
	replies, err := s.store.ProposalReplies(ctx, p.ID)
	if err != nil {
		return err
	}
	var gone []store.SeenItem
	for _, r := range replies {
		if !r.Posted() {
			gone = append(gone, r.Target())
		}
	}
	return s.forgetAnswered(ctx, w, gone)
}

func (s *Service) tellRejection(ctx context.Context, w store.Watch, p store.Proposal, reason string, discarded bool) {
	text, err := agent.DecisionMessage(agent.Decision{
		PR: pullRequestOf(w), Proposal: p.Number, Rejected: true, Discarded: discarded, Head: textx.ShortSHA(p.HeadSHA), Reason: reason,
	})
	policy, summary := deliverRoutine, fmt.Sprintf("told the agent you rejected proposal %d", p.Number)
	if reason != "" {
		policy, summary = deliverAuthor, fmt.Sprintf("you rejected proposal %d: %s", p.Number, firstLine(reason))
		s.syncWork(ctx, w)
	}
	if err == nil {
		_, err = s.deliver(ctx, w, text, summary, policy, nil)
	}
	if err != nil {
		s.agentFailed(ctx, w, "tell the agent about your decision", err)
	}
}

func (s *Service) SetApproval(ctx context.Context, id int64, c ApprovalChange) (store.Watch, error) {
	w, err := s.activeGatedWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	unlock := s.locks.Lock(w.ID)
	defer unlock()
	mode, autoRebase := w.ApprovalMode, w.AutoApproveRebase
	if c.Mode != nil {
		mode = *c.Mode
	}
	if c.AutoApproveRebase != nil {
		autoRebase = *c.AutoApproveRebase
	}
	if !mode.Valid() {
		return store.Watch{}, fmt.Errorf("%w: %q", ErrBadApprovalMode, mode)
	}
	p, pending, err := s.pendingProposal(ctx, w)
	if err != nil {
		return store.Watch{}, err
	}
	releases := mode == store.ApprovalAuto && pending
	if releases && !c.Release {
		return store.Watch{}, fmt.Errorf("%w: proposal %d; switching to auto releases it, so confirm the release", ErrProposalPending, p.Number)
	}
	if releases {
		w.AutoApproveRebase = autoRebase
		if _, err := s.approve(ctx, w, p, Decision{StopAsking: true}); err != nil {
			return store.Watch{}, err
		}
	}
	return s.store.SetWatchApproval(ctx, w.ID, mode, autoRebase)
}

func (s *Service) pendingProposal(ctx context.Context, w store.Watch) (store.Proposal, bool, error) {
	return s.store.LatestProposal(ctx, w.ID, store.ProposalPending)
}

func (s *Service) refusePending(ctx context.Context, w store.Watch) error {
	p, pending, err := s.pendingProposal(ctx, w)
	if err != nil || !pending {
		return err
	}
	return fmt.Errorf("%w: proposal %d; approve it, or reject it and say why", ErrProposalPending, p.Number)
}

func (s *Service) holdsForProposal(ctx context.Context, w store.Watch) bool {
	return s.refusePending(ctx, w) != nil
}

func (s *Service) proposalBlocker(ctx context.Context, w store.Watch) (string, error) {
	if !s.gates(w) {
		return "", nil
	}
	if p, open, err := s.store.ActiveProposal(ctx, w.ID); err != nil || open {
		return fmt.Sprintf("proposal %d of the agent has not gone out yet", p.Number), err
	}
	if p, ok, err := s.pendingProposal(ctx, w); err != nil || ok {
		return fmt.Sprintf("proposal %d waits on your approval", p.Number), err
	}
	p, ok, err := s.store.LatestProposal(ctx, w.ID, store.ProposalFailed)
	if err != nil || !ok {
		return "", err
	}
	if neverApproved(w, p) {
		return fmt.Sprintf("proposal %d of the agent did not go out; the agent resolves it in its next turn", p.Number), nil
	}
	return fmt.Sprintf("proposal %d of the agent did not go out; retry with `%s`", p.Number, s.retryCommand(w, p.Number)), nil
}

func (s *Service) offer(ctx context.Context, w store.Watch, p store.Proposal, rebased bool) error {
	if !rebased {
		if err := s.store.SetProposalOutcome(ctx, p.ID, store.ProposalPending, "", s.now()); err != nil {
			return err
		}
	}
	parts, err := s.proposalParts(ctx, w, p)
	if err != nil {
		return err
	}
	ref, summary := fmt.Sprint(p.Number), fmt.Sprintf("proposal %d waits on you: %s", p.Number, parts)
	if rebased {
		ref = fmt.Sprintf("%d rebased %s", p.Number, p.WorkSHA)
		summary = fmt.Sprintf("%s without conflicts and offered again: %s", moved(p), parts)
	}
	_, err = s.record(ctx, w, store.Activity{
		Kind: store.ActivityProposal, Ref: ref, At: s.now(),
		Summary: fmt.Sprintf("%s; read it with `%s`", summary, s.proposalsCommand(w, p.Number)),
		Payload: mustJSON(map[string]any{"proposal": p.Number, "rebased": rebased, "head_sha": p.HeadSHA, "work_sha": p.WorkSHA}),
	})
	return err
}

func moved(p store.Proposal) string {
	if p.MovedBy == store.BranchMerge {
		return fmt.Sprintf("%s merged into proposal %d", textx.ShortSHA(p.HeadSHA), p.Number)
	}
	return fmt.Sprintf("proposal %d rebased onto %s", p.Number, textx.ShortSHA(p.HeadSHA))
}

func (s *Service) proposalParts(ctx context.Context, w store.Watch, p store.Proposal) (string, error) {
	var parts []string
	if p.Pushes() {
		commits, err := s.rel.Log(ctx, w.WorktreeDir, p.HeadSHA, p.WorkSHA)
		if err != nil {
			return "", err
		}
		files, err := s.rel.Files(ctx, w.WorktreeDir, p.HeadSHA, p.WorkSHA)
		if err != nil {
			return "", err
		}
		parts = append(parts, textx.Plural(len(commits), "commit"), textx.Plural(len(files), "file"))
	}
	replies, err := s.store.ProposalReplies(ctx, p.ID)
	if err != nil {
		return "", err
	}
	if n := len(replies); n > 0 {
		parts = append(parts, textx.Count(n, "reply", "replies"))
	}
	return textx.JoinAnd(parts), nil
}

func (s *Service) proposalsCommand(w store.Watch, n int) string {
	return s.proposalCommand("proposals", w, n)
}

func (s *Service) staleProposal(ctx context.Context, w store.Watch) (store.Proposal, bool, error) {
	if !s.gates(w) || !s.pushes(w) {
		return store.Proposal{}, false, nil
	}
	if p, ok, err := s.pendingProposal(ctx, w); err != nil || ok {
		return p, ok && p.Pushes(), err
	}
	p, ok, err := s.store.LatestProposal(ctx, w.ID, store.ProposalFailed)
	approved := ok && p.ApprovedAt != nil && p.Pushes()
	return p, approved, err
}

func (s *Service) rebaseStale(ctx context.Context, w store.Watch) error {
	p, stale, err := s.staleProposal(ctx, w)
	if err != nil || !stale || w.HeadSHA == p.HeadSHA {
		return err
	}
	remote, err := s.rel.Fetch(ctx, w.WorktreeDir, w.HeadRef)
	if err != nil || remote == p.HeadSHA {
		return err
	}
	landed, err := s.rel.Contains(ctx, w.WorktreeDir, remote, p.WorkSHA)
	if err != nil || landed {
		return err
	}
	movable, err := s.movable(ctx, w, w.WorktreeDir, p.HeadSHA, p.WorkSHA, remote)
	if err != nil || !movable {
		return err
	}
	approvedAgain := p.ApprovedAt != nil && w.AutoApproveRebase
	if approvedAgain {
		if err := s.moveWork(ctx, w, &p, remote); err != nil {
			return s.staleConflict(ctx, w, p, err)
		}
		_, err := s.release(ctx, w, p, false)
		return err
	}
	work, err := s.moveOnto(ctx, w, p, remote)
	if err != nil {
		return s.staleConflict(ctx, w, p, err)
	}
	if err := s.store.MarkProposalMoved(ctx, p.ID, remote, work, w.BranchUpdate, s.now()); err != nil {
		return err
	}
	p, err = s.store.GetProposal(ctx, w.ID, p.Number)
	if err != nil {
		return err
	}
	return s.offer(ctx, w, p, true)
}

func (s *Service) staleConflict(ctx context.Context, w store.Watch, p store.Proposal, err error) error {
	var c *moveConflict
	if !errors.As(err, &c) {
		return err
	}
	if err := s.releaseFailed(ctx, w, p, &releaseError{what: c.action(p.Number), err: c}); err != nil {
		return err
	}
	s.handWork(ctx, w, p, c)
	return nil
}
