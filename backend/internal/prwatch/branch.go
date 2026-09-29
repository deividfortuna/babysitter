package prwatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

var (
	ErrBadBranchUpdate = errors.New("invalid branch update: use rebase or merge")
	ErrBranchUpdating  = errors.New("GitHub is updating the branch; send the message again after the next poll")

	errAcceptedNotRecorded = errors.New("GitHub accepted the update of the branch, but the daemon could not record it")
)

const (
	stallPolls        = 3
	branchUpdateTries = 3
	stallReason       = "GitHub accepted the update, but the branch did not move"
)

type BranchUpdater string

const (
	UpdaterDependabot BranchUpdater = "dependabot"
	UpdaterSession    BranchUpdater = "session"
	UpdaterGitHub     BranchUpdater = "github"
	UpdaterAgent      BranchUpdater = "agent"
)

func BranchUpdaterOf(w store.Watch) BranchUpdater {
	switch {
	case agent.IsDependabot(w.Author):
		return UpdaterDependabot
	case !hostedProvider(w.Provider):
		return UpdaterSession
	case w.UpdateOnGitHub:
		return UpdaterGitHub
	default:
		return UpdaterAgent
	}
}

func branchUpdateAfter(current store.BranchUpdate, change *store.BranchUpdate) (store.BranchUpdate, error) {
	if change == nil {
		return current, nil
	}
	if !change.Valid() {
		return "", fmt.Errorf("%w: %q", ErrBadBranchUpdate, *change)
	}
	return *change, nil
}

func behindRef(head string) string { return "behind@" + head }

func branchUpdateRef(head string) string { return "branch_update@" + head }

func branchPayload(w store.Watch) map[string]any {
	return map[string]any{"method": w.BranchUpdate, "from": w.HeadSHA, "base": w.BaseRef}
}

func (s *Service) updatesOnGitHub(w store.Watch) bool {
	return BranchUpdaterOf(w) == UpdaterGitHub
}

func (s *Service) githubStep(ctx context.Context, client *github.Client, w store.Watch, pr snapshot.PR) error {
	if pr.BehindErr != "" {
		s.log.Warn("could not compare the branch with its base, it counts as up to date", "watch", w.ID, "pr", prLabel(w), "err", redact.Text(pr.BehindErr))
	}
	if err := s.catchStalledUpdate(ctx, w); err != nil {
		return s.keepPolling(w, err)
	}
	candidate := pr.Behind() && s.updatesOnGitHub(w)
	if !candidate {
		return nil
	}
	return s.keepPolling(w, s.updateBehind(ctx, client, w, pr.NodeID))
}

func (s *Service) keepPolling(w store.Watch, err error) error {
	if err == nil || stopsThePoll(err) {
		return err
	}
	s.log.Error("update the branch on GitHub", "watch", w.ID, "pr", prLabel(w), "err", err)
	return nil
}

func (s *Service) updateBehind(ctx context.Context, client *github.Client, w store.Watch, nodeID string) error {
	behind, found, err := s.store.ActivityByRef(ctx, w.ID, store.ActivityBehind, behindRef(w.HeadSHA))
	if err != nil {
		return err
	}
	ready := found && behind.NudgedAt == nil && !s.sessionBusy(w)
	if !ready {
		return nil
	}
	tried, err := s.triedOnGitHub(ctx, w)
	if err != nil || tried {
		return err
	}
	blocker, err := s.proposalBlocker(ctx, w)
	if err != nil || blocker != "" {
		return err
	}
	resp, err := ghclient.UpdatePullBranch(ctx, client, nodeID, string(w.BranchUpdate), w.HeadSHA)
	if err := s.afterWrite(ctx, resp, err); err != nil {
		return s.branchUpdateFailed(ctx, w, err)
	}
	_, err = s.record(ctx, w, store.Activity{
		Kind: store.ActivityBranchUpdated, Ref: branchUpdateRef(w.HeadSHA),
		Summary: "GitHub accepted the request to " + branchUpdateWord(w) + " at " + textx.ShortSHA(w.HeadSHA),
		Payload: mustJSON(branchPayload(w)),
	})
	if err != nil {
		return fmt.Errorf("%w: %w", errAcceptedNotRecorded, err)
	}
	return nil
}

func stopsThePoll(err error) bool {
	return errors.Is(err, ghclient.ErrPaused) || errors.Is(err, errAcceptedNotRecorded)
}

func (s *Service) catchStalledUpdate(ctx context.Context, w store.Watch) error {
	sent, found, err := s.store.ActivityByRef(ctx, w.ID, store.ActivityBranchUpdated, branchUpdateRef(w.HeadSHA))
	if err != nil || !found {
		return err
	}
	stalled := s.now().Sub(sent.At) >= stallPolls*s.Interval()
	if !stalled {
		return nil
	}
	refused, err := s.refusedOnGitHub(ctx, w)
	if err != nil || refused {
		return err
	}
	return s.recordBranchRefused(ctx, w, stallReason)
}

func (s *Service) branchUpdateFailed(ctx context.Context, w store.Watch, failure error) error {
	refusal, refused := errors.AsType[*ghclient.BranchRefusal](failure)
	if refused {
		return s.recordBranchRefused(ctx, w, refusal.Reason)
	}
	pause := rateLimitOf(failure)
	if s.branchTries.count(w.ID, branchUpdateRef(w.HeadSHA)) < branchUpdateTries {
		s.log.Warn("GitHub did not answer the update of the branch, the next poll tries again", "watch", w.ID, "pr", prLabel(w), "err", redact.Text(failure.Error()))
		return pause
	}
	if err := s.recordBranchRefused(ctx, w, failure.Error()); err != nil {
		return err
	}
	return pause
}

func (s *Service) sessionBusy(w store.Watch) bool {
	return !s.quiet(w) || s.withAuthor(w)
}

func (s *Service) githubOwnsBehind(ctx context.Context, w store.Watch) (bool, error) {
	if !s.updatesOnGitHub(w) {
		return false, nil
	}
	refused, err := s.refusedOnGitHub(ctx, w)
	if err != nil || refused {
		return false, err
	}
	failed, err := s.proposalFailed(ctx, w)
	return !failed, err
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

func (s *Service) waitsForGitHub(ctx context.Context, w store.Watch) bool {
	accepted, err := s.store.HasActivity(ctx, w.ID, store.ActivityBranchUpdated, branchUpdateRef(w.HeadSHA))
	if err != nil || !accepted {
		return err != nil
	}
	refused, err := s.refusedOnGitHub(ctx, w)
	return err != nil || !refused
}

func (s *Service) refusedOnGitHub(ctx context.Context, w store.Watch) (bool, error) {
	return s.store.HasActivity(ctx, w.ID, store.ActivityBranchNotUpdated, branchUpdateRef(w.HeadSHA))
}

func (s *Service) recordBranchRefused(ctx context.Context, w store.Watch, reason string) error {
	reason = redact.Text(reason)
	s.log.Warn("GitHub did not update the branch, the agent does it", "watch", w.ID, "pr", prLabel(w), "err", reason)
	payload := branchPayload(w)
	payload["error"] = reason
	_, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityBranchNotUpdated, Ref: branchUpdateRef(w.HeadSHA),
		Summary: fmt.Sprintf("GitHub could not update %s, so the agent does it: %s", w.HeadRef, firstLine(reason)),
		Payload: mustJSON(payload),
	})
	return err
}

func branchUpdateWord(w store.Watch) string {
	if w.BranchUpdate == store.BranchMerge {
		return "merge " + w.BaseRef + " into " + w.HeadRef
	}
	return "rebase " + w.HeadRef + " onto " + w.BaseRef
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
