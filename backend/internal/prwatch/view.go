package prwatch

import (
	"context"
	"errors"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

var ErrNoSnapshot = errors.New("the daemon has not read the pull request since it started: try again after the next poll")

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

func (s *Service) publishFirst(ctx context.Context, id int64, snap *snapshot.Snapshot) {
	unlock := s.locks.Lock(id)
	defer unlock()
	w, err := s.store.GetWatch(ctx, id)
	if err != nil || w.Status != store.WatchActive {
		return
	}
	s.snapshots.getOrMake(id, func() *snapshot.Snapshot {
		view := *snap
		return &view
	})
}

func (s *Service) View(ctx context.Context, id int64) (*snapshot.Snapshot, error) {
	unlock := s.locks.Lock(id)
	defer unlock()
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

func (s *Service) Diff(ctx context.Context, id int64) (string, error) {
	unlock := s.locks.Lock(id)
	defer unlock()
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return "", err
	}
	if w.Status != store.WatchActive {
		return "", ErrWatchStopped
	}
	if s.guard.Paused() {
		return "", ghclient.ErrPaused
	}
	client, err := s.newClient(ctx)
	if err != nil {
		return "", err
	}
	diff, resp, err := ghclient.PullDiff(ctx, client, w.Owner, w.Name, w.Number)
	if err := s.guard.After(ctx, resp, err); err != nil {
		return "", err
	}
	return diff, nil
}
