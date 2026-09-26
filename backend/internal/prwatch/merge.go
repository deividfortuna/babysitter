package prwatch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

var ErrNotReady = errors.New("the pull request is not ready to merge")

var ErrMergeRefused = errors.New("GitHub refused the merge")

var ErrBadMergeMethod = errors.New("invalid merge method")

type MergeOptions struct {
	Method string
}

func normalizeMergeMethod(m string) (string, bool) {
	m = normalized(m)
	if m == "" {
		return "", true
	}
	return m, slices.Contains(ghclient.MergeMethodsKnown, m)
}

func mergeRefusal(blockers []string) error {
	if len(blockers) == 0 {
		blockers = []string{"the readiness has not stood a whole interval yet"}
	}
	return fmt.Errorf("%w: %s", ErrNotReady, strings.Join(blockers, "; "))
}

func (s *Service) Merge(ctx context.Context, id int64, o MergeOptions) (store.Watch, error) {
	method, ok := normalizeMergeMethod(o.Method)
	if !ok {
		return store.Watch{}, fmt.Errorf("%w: %q", ErrBadMergeMethod, o.Method)
	}
	unlock := s.locks.lock(id)
	defer unlock()

	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	if w.Status != store.WatchActive {
		return store.Watch{}, ErrWatchStopped
	}
	client, err := s.newClient(ctx)
	if err != nil {
		return store.Watch{}, err
	}
	snap, err := snapshot.Collect(ctx, client, s.store, target(w), s.watchOptions(w))
	if err != nil {
		return store.Watch{}, err
	}
	p, err := s.refresh(ctx, w, snap, s.now())
	if err != nil {
		return store.Watch{}, err
	}
	w, next := p.watch, p.next
	if reason, ok := p.ended(); ok {
		return s.stop(ctx, w.ID, reason, "", StopOptions{})
	}
	pending, err := s.untoldOf(ctx, w)
	if err != nil {
		return store.Watch{}, err
	}
	state, err := s.agentStatus(ctx, w, pending)
	if err != nil {
		return store.Watch{}, err
	}
	if err := s.assess(ctx, w, snap, state, p.newHead()); err != nil {
		return store.Watch{}, err
	}
	stored, err := s.store.GetWatch(ctx, w.ID)
	if err != nil {
		return store.Watch{}, err
	}
	if since, blockers := s.Readiness(stored, state.session.State); since == nil {
		return store.Watch{}, mergeRefusal(blockers)
	}
	if method, err = s.mergeMethod(ctx, client, w, cmp.Or(method, w.MergeMethod)); err != nil {
		return store.Watch{}, err
	}
	_, resp, err := ghclient.MergePull(ctx, client, w.Owner, w.Name, w.Number, method, snap.PR.HeadSHA)
	if err := s.afterWrite(ctx, resp, err); err != nil {
		return store.Watch{}, s.mergeFailed(ctx, w, snap, err)
	}
	if _, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityMerged, Ref: "merged", Actor: w.BotLogin,
		Summary: "merged by babysitter (" + method + ")", URL: snap.PR.URL,
		Payload: mustJSON(map[string]any{"sha": snap.PR.HeadSHA, "method": method}),
	}); err != nil {
		return store.Watch{}, err
	}
	if err := s.store.UpdateWatchState(ctx, w.ID, store.WatchState{
		Title: snap.PR.Title, BaseRef: snap.PR.BaseBranch,
		HeadSHA: next.HeadSHA, PRState: store.StateMerged, MergeableState: next.MergeableState,
		CheckStates: next.Checks, GreenSHA: next.GreenSHA, PolledAt: s.now(),
	}); err != nil {
		return store.Watch{}, err
	}
	return s.stop(ctx, w.ID, store.StopMerged, method, StopOptions{})
}

func (s *Service) mergeMethod(ctx context.Context, client *github.Client, w store.Watch, method string) (string, error) {
	repo, err := ghclient.GetRepo(ctx, client, w.Owner, w.Name)
	if err != nil {
		return "", err
	}
	allowed := ghclient.MergeMethods(repo)
	switch {
	case len(allowed) == 0:
		return "", fmt.Errorf("%w: %s allows no merge method", ErrMergeRefused, w.Repo())
	case method == "":
		return allowed[0], nil
	case !slices.Contains(allowed, method):
		return "", fmt.Errorf("%w: %s does not allow %s, only %s", ErrMergeRefused, w.Repo(), method, strings.Join(allowed, ", "))
	}
	return method, nil
}

func (s *Service) mergeFailed(ctx context.Context, w store.Watch, snap *snapshot.Snapshot, err error) error {
	if !ghclient.IsRefused(err) {
		return err
	}
	clean := redact.Err(err)
	if _, rerr := s.record(ctx, w, store.Activity{
		Kind: store.ActivityMergeFailed, Ref: fmt.Sprintf("merge@%s@%d", snap.PR.HeadSHA, s.now().Unix()),
		Summary: "GitHub refused the merge: " + clean.Error(), URL: snap.PR.URL,
		Payload: mustJSON(map[string]any{"sha": snap.PR.HeadSHA, "error": clean.Error()}),
	}); rerr != nil {
		return errors.Join(clean, rerr)
	}
	return fmt.Errorf("%w: %s", ErrMergeRefused, clean.Error())
}
