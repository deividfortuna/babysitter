package prwatch

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/deividfortuna/babysitter/internal/store"
)

var ErrBadApprovals = errors.New("invalid approvals: use 0 or more, or the rule of the base branch")

type MergeRulesChange struct {
	ApprovalsRequired Approvals
	MergeMethod       *string
	MergeWhenReady    *bool
}

func (s *Service) SetMergeRules(ctx context.Context, id int64, c MergeRulesChange) (store.Watch, error) {
	if n := c.ApprovalsRequired.Count; n != nil && *n < 0 {
		return store.Watch{}, fmt.Errorf("%w: %d", ErrBadApprovals, *n)
	}
	unlock := s.locks.Lock(id)
	defer unlock()
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	if w.Status != store.WatchActive {
		return store.Watch{}, ErrWatchStopped
	}
	method, err := mergeMethodAfter(w.MergeMethod, c.MergeMethod)
	if err != nil {
		return store.Watch{}, err
	}
	approvals, err := s.changedApprovals(ctx, w, c.ApprovalsRequired)
	if err != nil {
		return store.Watch{}, err
	}
	mergeWhenReady := *cmp.Or(c.MergeWhenReady, &w.MergeWhenReady)
	needsPoll := approvals != w.ApprovalsRequired || mergeWhenReady && !w.MergeWhenReady
	w, err = s.store.SetWatchMergeRules(ctx, w.ID, store.MergeRules{
		ApprovalsRequired: approvals, MergeMethod: method, MergeWhenReady: mergeWhenReady,
	})
	if err != nil {
		return store.Watch{}, err
	}
	if needsPoll {
		s.Kick(w.ID)
	}
	return w, nil
}

func mergeMethodAfter(current string, change *string) (string, error) {
	if change == nil {
		return current, nil
	}
	method, ok := normalizeMergeMethod(*change)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrBadMergeMethod, *change)
	}
	return method, nil
}

func (s *Service) changedApprovals(ctx context.Context, w store.Watch, a Approvals) (int, error) {
	if !a.Set {
		return w.ApprovalsRequired, nil
	}
	if a.Count != nil {
		return *a.Count, nil
	}
	client, err := s.newClient(ctx)
	if err != nil {
		return 0, err
	}
	return s.branchApprovals(ctx, client, w.Key(), w.BaseRef)
}
