package prwatch

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

var ErrBadBranchUpdate = errors.New("invalid branch update: use rebase or merge")

func branchUpdateAfter(current store.BranchUpdate, change *store.BranchUpdate) (store.BranchUpdate, error) {
	if change == nil {
		return current, nil
	}
	update := store.BranchUpdate(normalized(string(*change)))
	if !update.Valid() {
		return "", fmt.Errorf("%w: %q", ErrBadBranchUpdate, *change)
	}
	return update, nil
}

func branchUpdateRef(head string) string { return "branch_update@" + head }

func (s *Service) updatesOnGitHub(w store.Watch) bool {
	return w.UpdateOnGitHub && s.hosted(w) && !agent.IsDependabot(w.Author) && w.MergeableState == store.MergeableBehind
}

func (s *Service) updateBehind(ctx context.Context, client *github.Client, w store.Watch) error {
	if !s.updatesOnGitHub(w) {
		return nil
	}
	behind, err := s.store.UnnudgedOfKind(ctx, w.ID, store.ActivityBehind)
	if err != nil || len(behind) == 0 {
		return err
	}
	tried, err := s.triedOnGitHub(ctx, w)
	if err != nil || tried {
		return err
	}
	inFlight, err := s.workInFlight(ctx, w)
	if err != nil || inFlight {
		return err
	}
	head, resp, err := ghclient.UpdatePullBranch(ctx, client, w.Owner, w.Name, w.Number, string(w.BranchUpdate), w.HeadSHA)
	if err := s.afterWrite(ctx, resp, err); err != nil {
		if errors.Is(err, ghclient.ErrPaused) {
			return err
		}
		return s.recordBranchRefused(ctx, w, err)
	}
	if _, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityBranchUpdated, Ref: branchUpdateRef(w.HeadSHA),
		Summary: "GitHub " + branchUpdateWord(w) + ", now at " + textx.ShortSHA(head),
		Payload: mustJSON(map[string]any{"method": w.BranchUpdate, "from": w.HeadSHA, "to": head, "base": w.BaseRef}),
	}); err != nil {
		return err
	}
	return s.store.MarkActivityNudged(ctx, ids(behind), s.now())
}

func (s *Service) workInFlight(ctx context.Context, w store.Watch) (bool, error) {
	if s.sessionBusy(w) {
		return true, nil
	}
	blocker, err := s.proposalBlocker(ctx, w)
	return blocker != "", err
}

func (s *Service) sessionBusy(w store.Watch) bool {
	return !s.quiet(w) || s.withAuthor(w)
}

func (s *Service) holdForGitHub(ctx context.Context, w store.Watch, todo []store.Activity) ([]store.Activity, error) {
	if !s.updatesOnGitHub(w) {
		return todo, nil
	}
	tried, err := s.triedOnGitHub(ctx, w)
	if err != nil || tried {
		return todo, err
	}
	return slices.DeleteFunc(todo, isBehind), nil
}

func isBehind(a store.Activity) bool { return a.Kind == store.ActivityBehind }

func (s *Service) triedOnGitHub(ctx context.Context, w store.Watch) (bool, error) {
	ref := branchUpdateRef(w.HeadSHA)
	updated, err := s.store.HasActivity(ctx, w.ID, store.ActivityBranchUpdated, ref)
	if err != nil || updated {
		return updated, err
	}
	return s.store.HasActivity(ctx, w.ID, store.ActivityBranchNotUpdated, ref)
}

func (s *Service) recordBranchRefused(ctx context.Context, w store.Watch, refusal error) error {
	reason := redact.Text(refusal.Error())
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

func (s *Service) followsRemote(ctx context.Context, w store.Watch, work string) bool {
	return !s.pushes(w) || s.updatedOnGitHub(ctx, w, work)
}

func (s *Service) updatedOnGitHub(ctx context.Context, w store.Watch, work string) bool {
	updated, err := s.store.HasActivity(ctx, w.ID, store.ActivityBranchUpdated, branchUpdateRef(work))
	return err == nil && updated
}
