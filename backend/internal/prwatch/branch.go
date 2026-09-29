package prwatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

var ErrBadBranchUpdate = errors.New("invalid branch update: use rebase or merge")

var errHeadDidNotMove = errors.New("GitHub answered without a new head")

const branchUpdateTries = 3

func branchUpdateAfter(current store.BranchUpdate, change *store.BranchUpdate) (store.BranchUpdate, error) {
	if change == nil {
		return current, nil
	}
	if !change.Valid() {
		return "", fmt.Errorf("%w: %q", ErrBadBranchUpdate, *change)
	}
	return *change, nil
}

func branchUpdateRef(head string) string { return "branch_update@" + head }

type headTries struct {
	head  string
	count int
}

type branchTries struct {
	mu      sync.Mutex
	byWatch map[int64]headTries
}

func (t *branchTries) again(id int64, head string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.byWatch == nil {
		t.byWatch = map[int64]headTries{}
	}
	tries := t.byWatch[id]
	if tries.head != head {
		tries = headTries{head: head}
	}
	tries.count++
	t.byWatch[id] = tries
	return tries.count < branchUpdateTries
}

func (t *branchTries) forget(id int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.byWatch, id)
}

func (s *Service) updatesOnGitHub(w store.Watch) bool {
	return w.UpdateOnGitHub && s.hosted(w) && !agent.IsDependabot(w.Author)
}

func (s *Service) githubOwnsBehind(ctx context.Context, w store.Watch) (bool, error) {
	if !s.updatesOnGitHub(w) {
		return false, nil
	}
	failed, err := s.proposalFailed(ctx, w)
	if err != nil || failed {
		return false, err
	}
	tried, err := s.triedOnGitHub(ctx, w)
	return !tried, err
}

func (s *Service) updateBehind(ctx context.Context, client *github.Client, w store.Watch, nodeID string) error {
	owns, err := s.githubOwnsBehind(ctx, w)
	if err != nil || !owns {
		return err
	}
	behind, found, err := s.store.ActivityByRef(ctx, w.ID, store.ActivityBehind, "behind@"+w.HeadSHA)
	if err != nil {
		return err
	}
	waiting := found && behind.NudgedAt == nil
	if !waiting {
		return nil
	}
	inFlight, err := s.workInFlight(ctx, w)
	if err != nil || inFlight {
		return err
	}
	head, resp, err := ghclient.UpdatePullBranch(ctx, client, nodeID, string(w.BranchUpdate), w.HeadSHA)
	if err := s.afterWrite(ctx, resp, err); err != nil {
		return s.branchUpdateFailed(ctx, w, err)
	}
	moved := head != "" && head != w.HeadSHA
	if !moved {
		return s.branchUpdateFailed(ctx, w, errHeadDidNotMove)
	}
	s.tries.forget(w.ID)
	if _, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityBranchUpdated, Ref: branchUpdateRef(w.HeadSHA),
		Summary: "GitHub " + branchUpdateWord(w) + ", now at " + textx.ShortSHA(head),
		Payload: mustJSON(map[string]any{"method": w.BranchUpdate, "from": w.HeadSHA, "to": head, "base": w.BaseRef}),
	}); err != nil {
		return err
	}
	return s.store.MarkActivityNudged(ctx, []int64{behind.ID}, s.now())
}

func (s *Service) branchUpdateFailed(ctx context.Context, w store.Watch, failure error) error {
	if errors.Is(failure, ghclient.ErrPaused) {
		return failure
	}
	transient := !errors.Is(failure, ghclient.ErrBranchNotUpdated)
	if transient && s.tries.again(w.ID, w.HeadSHA) {
		s.log.Warn("GitHub did not answer the update of the branch, the next poll tries again", "watch", w.ID, "pr", prLabel(w), "err", redact.Text(failure.Error()))
		return nil
	}
	s.tries.forget(w.ID)
	return s.recordBranchRefused(ctx, w, failure)
}

func (s *Service) workInFlight(ctx context.Context, w store.Watch) (bool, error) {
	if s.sessionBusy(w) {
		return true, nil
	}
	if !s.gates(w) {
		return false, nil
	}
	_, open, err := s.store.ActiveProposal(ctx, w.ID)
	if err != nil || open {
		return open, err
	}
	_, pending, err := s.pendingProposal(ctx, w)
	return pending, err
}

func (s *Service) sessionBusy(w store.Watch) bool {
	return !s.quiet(w) || s.withAuthor(w)
}

func (s *Service) proposalFailed(ctx context.Context, w store.Watch) (bool, error) {
	if !s.gates(w) {
		return false, nil
	}
	_, failed, err := s.store.LatestProposal(ctx, w.ID, store.ProposalFailed)
	return failed, err
}

func (s *Service) holdForGitHub(ctx context.Context, w store.Watch, todo []store.Activity) ([]store.Activity, error) {
	if !slices.ContainsFunc(todo, isBehind) {
		return todo, nil
	}
	owns, err := s.githubOwnsBehind(ctx, w)
	if err != nil || !owns {
		return todo, err
	}
	return slices.DeleteFunc(todo, isBehind), nil
}

func isBehind(a store.Activity) bool { return a.Kind == store.ActivityBehind }

func (s *Service) triedOnGitHub(ctx context.Context, w store.Watch) (bool, error) {
	return s.store.HasActivityOfKinds(ctx, w.ID, branchUpdateRef(w.HeadSHA), store.ActivityBranchUpdated, store.ActivityBranchNotUpdated)
}

func refusalReason(failure error) string {
	if refusal, ok := errors.AsType[*ghclient.BranchRefusal](failure); ok {
		return refusal.Reason
	}
	return failure.Error()
}

func (s *Service) recordBranchRefused(ctx context.Context, w store.Watch, refusal error) error {
	reason := redact.Text(refusalReason(refusal))
	s.log.Warn("GitHub did not update the branch, the agent does it", "watch", w.ID, "pr", prLabel(w), "err", reason)
	_, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityBranchNotUpdated, Ref: branchUpdateRef(w.HeadSHA),
		Summary: fmt.Sprintf("GitHub could not update %s, so the agent does it: %s", w.HeadRef, firstLine(reason)),
		Payload: mustJSON(map[string]any{"method": w.BranchUpdate, "from": w.HeadSHA, "base": w.BaseRef, "error": reason}),
	})
	return err
}

func branchUpdateWord(w store.Watch) string {
	if w.BranchUpdate == store.BranchMerge {
		return "merged " + w.BaseRef + " into " + w.HeadRef
	}
	return "rebased " + w.HeadRef + " onto " + w.BaseRef
}

func (s *Service) knownHead(ctx context.Context, w store.Watch, sha string) bool {
	seen, err := s.store.HasActivity(ctx, w.ID, store.ActivityCommit, sha)
	if err == nil && seen {
		return true
	}
	return s.startHead(ctx, w) == sha
}

func (s *Service) startHead(ctx context.Context, w store.Watch) string {
	start, found, err := s.store.ActivityByRef(ctx, w.ID, store.ActivityWatchStarted, "start")
	if err != nil || !found {
		return ""
	}
	var p struct {
		HeadSHA string `json:"head_sha"`
	}
	if json.Unmarshal(start.Payload, &p) != nil {
		return ""
	}
	return p.HeadSHA
}
