package prwatch

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

var (
	ErrNotDependabot = errors.New("only a pull request of Dependabot is approved in your name")
	ErrOutOfScope    = errors.New("the update is outside the Dependabot merge scope of the repository")
)

const policyReviewBody = "Approved by the Dependabot policy of babysitter: the build is green and the update is within the merge scope of the repository."

func (s *Service) repoConfigOf(ctx context.Context, w store.Watch) (store.RepoConfig, error) {
	repo, err := s.store.GetRepo(ctx, w.Owner, w.Name)
	if errors.Is(err, store.ErrRepoNotFound) {
		return store.DefaultRepoConfig(0), nil
	}
	if err != nil {
		return store.RepoConfig{}, err
	}
	return s.store.GetRepoConfig(ctx, repo.ID)
}

func (s *Service) inScope(ctx context.Context, w store.Watch) (store.RepoConfig, bool, error) {
	cfg, err := s.repoConfigOf(ctx, w)
	if err != nil {
		return store.RepoConfig{}, false, err
	}
	return cfg, agent.IsDependabot(w.Author) && dependabot.Within(w.UpdateType, cfg.DependabotScope), nil
}

func (s *Service) approveByHand(ctx context.Context, client *github.Client, w store.Watch) error {
	if !agent.IsDependabot(w.Author) {
		return fmt.Errorf("%w: %s#%d is by %s", ErrNotDependabot, w.Repo(), w.Number, w.Author)
	}
	snap, err := snapshot.Collect(ctx, client, s.store, target(w), s.watchOptions(w))
	if err != nil {
		return err
	}
	if w, err = s.followUpdateType(ctx, w, snap); err != nil {
		return err
	}
	cfg, ok, err := s.inScope(ctx, w)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s update, the scope is %s", ErrOutOfScope, updateWord(w.UpdateType), cfg.DependabotScope)
	}
	return s.approveOnce(ctx, client, w, snap, "approved in your name from babysitter")
}

func (s *Service) approveOnce(ctx context.Context, client *github.Client, w store.Watch, snap *snapshot.Snapshot, why string) error {
	if done, err := s.store.HasActivity(ctx, w.ID, store.ActivityApproved, "approve@"+snap.PR.HeadSHA); err != nil || done {
		return err
	}
	return s.approvePull(ctx, client, w, snap, why)
}

func (s *Service) followUpdateType(ctx context.Context, w store.Watch, snap *snapshot.Snapshot) (store.Watch, error) {
	level := cmp.Or(snap.PR.UpdateType, w.UpdateType)
	mergeWhenReady := w.MergeWhenReady
	if w.AutoReason == store.AutoDependabot && mergeWhenReady {
		cfg, err := s.repoConfigOf(ctx, w)
		if err != nil {
			return w, err
		}
		mergeWhenReady = dependabot.Within(level, cfg.DependabotScope)
	}
	unchanged := level == w.UpdateType && mergeWhenReady == w.MergeWhenReady
	if unchanged {
		return w, nil
	}
	return s.store.SetWatchUpdateType(ctx, w.ID, level, mergeWhenReady)
}

func updateWord(l dependabot.Level) string {
	if l == "" {
		return "an unknown"
	}
	return "a " + string(l)
}

func (s *Service) approvePull(ctx context.Context, client *github.Client, w store.Watch, snap *snapshot.Snapshot, why string) error {
	sha := snap.PR.HeadSHA
	review, resp, err := ghclient.ApprovePull(ctx, client, w.Owner, w.Name, w.Number, sha, policyReviewBody)
	if err := s.afterWrite(ctx, resp, err); err != nil {
		return err
	}
	_, err = s.record(ctx, w, store.Activity{
		Kind: store.ActivityApproved, Ref: "approve@" + sha, Actor: w.BotLogin,
		Summary: fmt.Sprintf("%s: %s update, build green on %s", why, w.UpdateType, shortSHA(sha)),
		URL:     review.GetHTMLURL(),
		Payload: mustJSON(map[string]any{"sha": sha, "update_type": w.UpdateType, "review_id": review.GetID()}),
	})
	return err
}

func shortSHA(sha string) string { return sha[:min(7, len(sha))] }

func (s *Service) dependabotPolicy(ctx context.Context, client *github.Client, w store.Watch, snap *snapshot.Snapshot, state agentStatus) error {
	if w.AutoReason != store.AutoDependabot || !snapshot.BuildGreen(snap) {
		return nil
	}
	cfg, ok, err := s.inScope(ctx, w)
	if err != nil || !ok {
		return err
	}
	sha := snap.PR.HeadSHA
	switch cfg.DependabotApproval {
	case store.ApproveGreen:
		return s.approveOnce(ctx, client, w, snap, "approved in your name by the Dependabot policy of babysitter")
	case store.ApproveAsk:
		if !s.waitsOnlyForReview(w, snap, state) {
			return nil
		}
		_, err := s.record(ctx, w, store.Activity{
			Kind: store.ActivityApprovalAsked, Ref: "ask@" + sha,
			Summary: fmt.Sprintf("%s waits on your review: %s update, build green. Only a review is missing.", snap.PR.Title, w.UpdateType),
			Payload: mustJSON(map[string]any{"sha": sha, "update_type": w.UpdateType}),
		})
		return err
	default:
		return nil
	}
}

func (s *Service) waitsOnlyForReview(w store.Watch, snap *snapshot.Snapshot, state agentStatus) bool {
	return snapshot.WaitsOnlyForReview(snap, w.ApprovalsRequired) &&
		state.untold == "" && state.proposal == "" && state.author == ""
}

func (s *Service) mergeWhenReady(ctx context.Context, client *github.Client, w store.Watch, snap *snapshot.Snapshot, next State, state agentStatus) (bool, error) {
	stored, err := s.store.GetWatch(ctx, w.ID)
	if err != nil || !stored.MergeWhenReady {
		return false, err
	}
	if since, _ := s.Readiness(stored, state.session.State); since == nil {
		return false, nil
	}
	m := mergeWhenReady(stored, snap)
	if tried, err := s.store.HasActivity(ctx, w.ID, store.ActivityMergeFailed, m.failedRef); err != nil || tried {
		return false, err
	}
	_, err = s.mergeNow(ctx, client, stored, snap, next, m)
	if errors.Is(err, ErrMergeRefused) {
		return false, s.recordAutoMergeFailure(ctx, stored, snap, m.failedRef, err)
	}
	if err != nil {
		s.log.Warn("merge when ready failed, the next poll tries again", "watch", w.ID, "pr", prLabel(w), "err", err)
		return false, nil
	}
	return true, nil
}

func (s *Service) recordAutoMergeFailure(ctx context.Context, w store.Watch, snap *snapshot.Snapshot, ref string, cause error) error {
	_, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityMergeFailed, Ref: ref,
		Summary: cause.Error() + ". The daemon tries again on a new head.", URL: snap.PR.URL,
		Payload: mustJSON(map[string]any{"sha": snap.PR.HeadSHA, "error": cause.Error()}),
	})
	return err
}
