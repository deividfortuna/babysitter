package prwatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

var (
	ErrNotTakenOver    = errors.New("the session is not taken over, so there is nothing to hand back")
	ErrAuthorRunning   = errors.New("the session is still open in your terminal")
	ErrUnconfirmedWork = errors.New("the worktree has work that is not on the pull request")
	ErrUnpushedCommits = errors.New("the daemon never pushes a Dependabot branch")
)

type HandbackOptions struct {
	Confirm bool
	Force   bool
}

type WorkError struct {
	Commits []gitrelease.Commit
	Files   []string
}

func (e *WorkError) Error() string {
	return ErrUnconfirmedWork.Error() + ": " + workWord(e.Commits, e.Files)
}

func (e *WorkError) Is(target error) bool { return target == ErrUnconfirmedWork }

type authorWork struct {
	head    string
	work    string
	commits []gitrelease.Commit
	files   []string
}

func (a authorWork) empty() bool {
	return len(a.commits) == 0 && len(a.files) == 0
}

func workWord(commits []gitrelease.Commit, files []string) string {
	var parts []string
	if len(commits) > 0 {
		parts = append(parts, textx.Plural(len(commits), "commit")+" that the pull request does not have")
	}
	if len(files) > 0 {
		parts = append(parts, textx.Count(len(files), "change that is not committed", "changes that are not committed"))
	}
	return textx.JoinAnd(parts)
}

func (s *Service) Handback(ctx context.Context, id int64, o HandbackOptions) (store.Watch, error) {
	unlock := s.locks.Lock(id)
	defer unlock()
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	if err := s.refuseHandback(w, o); err != nil {
		return store.Watch{}, err
	}
	work, err := s.readAuthorWork(ctx, w)
	if err != nil {
		return store.Watch{}, err
	}
	if err := s.confirmWork(w, work, o); err != nil {
		return store.Watch{}, err
	}
	msg, err := handbackMessage(w, work)
	if err != nil {
		return store.Watch{}, err
	}
	if err := s.store.SetWatchHandbackStart(ctx, id, work.head); err != nil {
		return store.Watch{}, err
	}
	takenOverAt, takenOverPID := w.TakenOverAt, w.TakenOverPID
	if w, err = s.store.SetWatchTakeover(ctx, id, nil, 0); err != nil {
		return store.Watch{}, err
	}
	w.HandbackStart = work.head

	text := "you handed the session back; the agent continues the same conversation"
	if !work.empty() {
		text += ", with " + workWord(work.commits, work.files)
	}
	if _, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityHandedBack, Ref: stampRef("handback", s.now()), Summary: text,
		Payload: mustJSON(map[string]any{"by": "author", "commits": len(work.commits), "files": len(work.files), "message": msg}),
	}); err != nil {
		_, undoErr := s.store.SetWatchTakeover(ctx, id, takenOverAt, takenOverPID)
		return store.Watch{}, errors.Join(err, undoErr)
	}
	s.tellHandback(ctx, w)
	s.Kick(w.ID)
	back, err := s.store.GetWatch(ctx, id)
	if err != nil {
		s.log.Warn("read the watch after the hand-back", "watch", id, "err", err)
		return w, nil
	}
	return back, nil
}

func (s *Service) refuseHandback(w store.Watch, o HandbackOptions) error {
	switch {
	case w.Status != store.WatchActive:
		return ErrWatchStopped
	case !s.withAuthor(w):
		return ErrNotTakenOver
	case !o.Force && s.alive(w.TakenOverPID):
		return fmt.Errorf("%w (pid %d)", ErrAuthorRunning, w.TakenOverPID)
	}
	return nil
}

func (s *Service) readAuthorWork(ctx context.Context, w store.Watch) (authorWork, error) {
	head, err := s.rel.Fetch(ctx, w.WorktreeDir, w.HeadRef)
	if err != nil {
		s.log.Warn("read the pull request branch before a hand-back", "watch", w.ID, "err", err)
		head = w.HeadSHA
	}
	work, err := s.rel.Head(ctx, w.WorktreeDir)
	if err != nil {
		return authorWork{}, fmt.Errorf("read the work branch: %w", err)
	}
	commits, err := s.rel.Log(ctx, w.WorktreeDir, head, work)
	if err != nil {
		return authorWork{}, fmt.Errorf("read the commits of the work branch: %w", err)
	}
	files, err := s.rel.Dirty(ctx, w.WorktreeDir)
	if err != nil {
		return authorWork{}, fmt.Errorf("read the changes of the worktree: %w", err)
	}
	return authorWork{head: head, work: work, commits: commits, files: files}, nil
}

func (s *Service) confirmWork(w store.Watch, work authorWork, o HandbackOptions) error {
	if len(work.commits) > 0 && !s.pushes(w) {
		return fmt.Errorf("%w: push your %s with git push origin HEAD:%s first, then hand back",
			ErrUnpushedCommits, textx.Plural(len(work.commits), "commit"), w.HeadRef)
	}
	if !work.empty() && !o.Confirm {
		return &WorkError{Commits: work.commits, Files: work.files}
	}
	return nil
}

func handbackMessage(w store.Watch, work authorWork) (string, error) {
	commits := make([]agent.Commit, 0, len(work.commits))
	for _, c := range work.commits {
		commits = append(commits, agent.Commit{SHA: textx.ShortSHA(c.SHA), Subject: c.Subject})
	}
	return agent.HandbackMessage(agent.Handback{
		PR: pullRequestOf(w), WorkBranch: w.WorkBranch, Work: textx.ShortSHA(work.work), Commits: commits, Files: work.files,
	})
}

func (s *Service) tellHandback(ctx context.Context, w store.Watch) {
	untold, err := s.store.UnnudgedOfKind(ctx, w.ID, store.ActivityHandedBack)
	if err != nil {
		s.log.Error("read the hand-back the agent has not got", "watch", w.ID, "err", err)
		return
	}
	if len(untold) == 0 {
		return
	}
	msg := storedMessage(untold[len(untold)-1])
	if msg == "" {
		return
	}
	if _, err := s.deliver(ctx, w, msg, "told the agent the session is back", deliverAuthor, untold); err != nil {
		s.agentFailed(ctx, w, "give the session back to the agent", err)
	}
}

func storedMessage(a store.Activity) string {
	var p struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(a.Payload, &p) != nil {
		return ""
	}
	return p.Message
}

func (s *Service) clearHandbackStart(ctx context.Context, w store.Watch) {
	if w.HandbackStart == "" {
		return
	}
	if err := s.store.SetWatchHandbackStart(ctx, w.ID, ""); err != nil {
		s.log.Error("clear the start of the turn after a hand-back", "watch", w.ID, "err", err)
	}
}
