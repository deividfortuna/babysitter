package prwatch

import (
	"cmp"
	"context"

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
	req.IncludeExisting = cmp.Or(req.IncludeExisting, &set.IncludeExisting)
	req.IncludeOwn = cmp.Or(req.IncludeOwn, &set.IncludeOwn)
	req.MergeMethod = cmp.Or(req.MergeMethod, &set.MergeMethod)
	req.ApprovalMode = cmp.Or(req.ApprovalMode, &set.ApprovalMode)
	req.AutoApproveRebase = cmp.Or(req.AutoApproveRebase, &set.AutoApproveRebase)
	if !req.ApprovalsRequired.Set {
		req.ApprovalsRequired = Approvals{Set: true, Count: set.ApprovalsRequired}
	}
	return req, nil
}

func (s *Service) keepsWorktree(ctx context.Context, w store.Watch, o StopOptions) bool {
	return s.withAuthor(w) || s.keepWorktree(ctx, o)
}

func (s *Service) keepWorktree(ctx context.Context, o StopOptions) bool {
	if o.KeepWorktree != nil {
		return *o.KeepWorktree
	}
	set, err := s.store.Settings(ctx)
	if err != nil {
		s.log.Warn("read the settings for the worktree of a stopped watch, removing it", "err", err)
		return false
	}
	return set.KeepWorktree
}
