package prwatch

import (
	"cmp"
	"context"
	"errors"

	"github.com/deividfortuna/babysitter/internal/store"
)

type Approvals struct {
	Set   bool
	Count *int
}

func (s *Service) withDefaults(ctx context.Context, req StartRequest) (StartRequest, error) {
	set, err := s.store.Settings(ctx)
	if err != nil {
		return req, err
	}
	repo, err := s.repoOverrides(ctx, req)
	if err != nil {
		return req, err
	}
	chosen := agentOf(req, repo, set)
	req.Provider, req.Model, req.Effort = chosen.provider, chosen.model, chosen.effort
	req.IncludeExisting = cmp.Or(req.IncludeExisting, repo.IncludeExisting, &set.IncludeExisting)
	req.IncludeOwn = cmp.Or(req.IncludeOwn, repo.IncludeOwn, &set.IncludeOwn)
	req.MergeMethod = cmp.Or(req.MergeMethod, setOrNil(repo.MergeMethod), &set.MergeMethod)
	req.ApprovalMode = cmp.Or(req.ApprovalMode, setOrNil(repo.ApprovalMode), &set.ApprovalMode)
	req.AutoApproveRebase = cmp.Or(req.AutoApproveRebase, repo.AutoApproveRebase, &set.AutoApproveRebase)
	req.KeepWorktree = cmp.Or(req.KeepWorktree, repo.KeepWorktree, &set.KeepWorktree)
	req.BranchUpdate = cmp.Or(req.BranchUpdate, setOrNil(repo.BranchUpdate), &set.BranchUpdate)
	req.UpdateOnGitHub = cmp.Or(req.UpdateOnGitHub, repo.UpdateOnGitHub, &set.UpdateOnGitHub)
	req.ApprovalsRequired = approvalsOf(req.ApprovalsRequired, repo, set)
	return req, nil
}

func (s *Service) repoOverrides(ctx context.Context, req StartRequest) (store.WatchOverrides, error) {
	repo, err := s.store.GetRepo(ctx, req.Target.Owner, req.Target.Name)
	if errors.Is(err, store.ErrRepoNotFound) {
		return store.WatchOverrides{}, nil
	}
	if err != nil {
		return store.WatchOverrides{}, err
	}
	cfg, err := s.store.GetRepoConfig(ctx, repo.ID)
	if err != nil {
		return store.WatchOverrides{}, err
	}
	return cfg.Overrides, nil
}

type agentChoice struct {
	provider string
	model    string
	effort   string
}

func agentOf(req StartRequest, repo store.WatchOverrides, set store.Settings) agentChoice {
	asked := agentChoice{provider: req.Provider, model: req.Model, effort: req.Effort}
	switch {
	case req.Provider != "":
		return asked
	case repo.Provider != "":
		return asked.over(agentChoice{provider: repo.Provider, model: repo.Model, effort: repo.Effort})
	default:
		return asked.over(agentChoice{provider: set.Provider, model: set.Model, effort: set.Effort})
	}
}

func (a agentChoice) over(layer agentChoice) agentChoice {
	if a.model != "" {
		return agentChoice{provider: layer.provider, model: a.model, effort: a.effort}
	}
	return agentChoice{provider: layer.provider, model: layer.model, effort: cmp.Or(a.effort, layer.effort)}
}

func approvalsOf(asked Approvals, repo store.WatchOverrides, set store.Settings) Approvals {
	switch {
	case asked.Set:
		return asked
	case repo.ApprovalsSet:
		return Approvals{Set: true, Count: repo.Approvals}
	default:
		return Approvals{Set: true, Count: set.ApprovalsRequired}
	}
}

func setOrNil[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}

func (s *Service) keepsWorktree(w store.Watch, o StopOptions) bool {
	return s.withAuthor(w) || *cmp.Or(o.KeepWorktree, &w.KeepWorktree)
}
