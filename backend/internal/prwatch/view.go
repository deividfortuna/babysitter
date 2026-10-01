package prwatch

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

var ErrNoSnapshot = errors.New("the daemon has not read the pull request since it started: try again after the next poll")

type PullDiff struct {
	Base      string
	Head      string
	Diff      string
	Truncated bool
}

func (s *Service) collect(ctx context.Context, client *github.Client, w store.Watch) (*snapshot.Snapshot, error) {
	snap, err := snapshot.Collect(ctx, client, s.store, target(w), s.watchOptions(w))
	if err != nil {
		return nil, err
	}
	s.publish(w.ID, snap)
	return snap, nil
}

func (s *Service) publish(id int64, snap *snapshot.Snapshot) {
	view := *snap
	s.snapshots.set(id, &view)
}

func (s *Service) View(ctx context.Context, id int64) (*snapshot.Snapshot, error) {
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.Status != store.WatchActive {
		return nil, ErrWatchStopped
	}
	snap := s.snapshots.get(w.ID)
	if snap == nil {
		return nil, ErrNoSnapshot
	}
	return snap, nil
}

func (s *Service) Diff(ctx context.Context, id int64) (PullDiff, error) {
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return PullDiff{}, err
	}
	if w.Status != store.WatchActive {
		return PullDiff{}, ErrWatchStopped
	}
	dir := checkoutOf(w)
	base, err := s.rel.Fetch(ctx, dir, w.BaseRef)
	if err != nil {
		return PullDiff{}, fmt.Errorf("fetch the base branch %s: %w", w.BaseRef, err)
	}
	head, err := s.rel.Fetch(ctx, dir, w.HeadRef)
	if err != nil {
		return PullDiff{}, fmt.Errorf("fetch the head branch %s: %w", w.HeadRef, err)
	}
	from, err := s.rel.MergeBase(ctx, dir, base, head)
	if err != nil {
		return PullDiff{}, err
	}
	d := PullDiff{Base: from, Head: head}
	if d.Diff, d.Truncated, err = s.rel.Diff(ctx, dir, from, head, maxDiff); err != nil {
		return PullDiff{}, err
	}
	return d, nil
}

func checkoutOf(w store.Watch) string {
	if w.WorktreeDir != "" {
		return w.WorktreeDir
	}
	return w.SourceDir
}
