package prwatch

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/gitrepo"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

var ErrNoSnapshot = errors.New("the daemon has not read the pull request since it started: try again after the next poll")

var ErrShallowCheckout = errors.New("the checkout is shallow, so it does not have the commit where the head left the base")

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

func (s *Service) Diff(ctx context.Context, id int64) (PullDiff, error) {
	unlock := s.locks.Lock(id)
	defer unlock()
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return PullDiff{}, err
	}
	if w.Status != store.WatchActive {
		return PullDiff{}, ErrWatchStopped
	}
	dir := checkoutOf(w)
	base, err := s.fetchBase(ctx, dir, w)
	if err != nil {
		return PullDiff{}, fmt.Errorf("fetch the base branch %s: %w", w.BaseRef, err)
	}
	head, err := s.rel.Fetch(ctx, dir, w.HeadRef)
	if err != nil {
		return PullDiff{}, fmt.Errorf("fetch the head branch %s: %w", w.HeadRef, err)
	}
	from, err := s.rel.MergeBase(ctx, dir, base, head)
	if err != nil {
		return PullDiff{}, s.noMergeBase(ctx, dir, err)
	}
	d := PullDiff{Base: from, Head: head}
	if d.Diff, d.Truncated, err = s.rel.Diff(ctx, dir, from, head, maxDiff); err != nil {
		return PullDiff{}, err
	}
	return d, nil
}

func (s *Service) noMergeBase(ctx context.Context, dir string, err error) error {
	if shallow, _ := s.rel.Shallow(ctx, dir); shallow {
		return fmt.Errorf("%w: run git fetch --unshallow in %s", ErrShallowCheckout, dir)
	}
	return err
}

func (s *Service) fetchBase(ctx context.Context, dir string, w store.Watch) (string, error) {
	origin, err := gitrepo.RemoteURL(ctx, w.SourceDir, "origin")
	if err != nil {
		return "", err
	}
	owner, name, err := store.ParseFullName(origin)
	if err != nil {
		return "", err
	}
	baseRepo := w.Owner + "/" + w.Name
	if strings.EqualFold(owner+"/"+name, baseRepo) {
		return s.rel.Fetch(ctx, dir, w.BaseRef)
	}
	return s.rel.FetchFrom(ctx, dir, siblingURL(origin, baseRepo), w.BaseRef)
}

func siblingURL(origin, repo string) string {
	if u, err := url.Parse(origin); err == nil && u.Scheme != "" && u.Scheme != "ssh" {
		u.User = nil
		origin = u.String()
	}
	path := strings.TrimSuffix(strings.TrimSuffix(origin, "/"), ".git")
	ownerStart := strings.LastIndexAny(path[:strings.LastIndex(path, "/")], "/:") + 1
	return path[:ownerStart] + repo + ".git"
}

func checkoutOf(w store.Watch) string {
	if w.WorktreeDir != "" {
		return w.WorktreeDir
	}
	return w.SourceDir
}
