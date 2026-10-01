package prwatch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func TestViewKeepsTheLastSnapshotOfTheWatch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.Labels, fx.pr.Body = []string{"bug"}, "Fixes the retry loop." })
	w := fx.start()

	snap, err := fx.svc.View(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("View() after the start error = %v", err)
	}
	if strings.Join(snap.PR.Labels, ",") != "bug" || snap.PR.Body != "Fixes the retry loop." {
		t.Fatalf("View() after the start = labels %v, body %q", snap.PR.Labels, snap.PR.Body)
	}

	fx.update(func() { fx.pr.Labels = []string{"bug", "ready"} })
	fx.poll(w)
	if snap, err = fx.svc.View(context.Background(), w.ID); err != nil {
		t.Fatal(err)
	}
	if strings.Join(snap.PR.Labels, ",") != "bug,ready" {
		t.Fatalf("View() after a poll = labels %v, want bug,ready", snap.PR.Labels)
	}

	if _, err := fx.newService().View(context.Background(), w.ID); !errors.Is(err, ErrNoSnapshot) {
		t.Fatalf("View() of a service that has not polled yet error = %v, want ErrNoSnapshot", err)
	}

	if _, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.View(context.Background(), w.ID); !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("View() of a stopped watch error = %v, want ErrWatchStopped", err)
	}
}

func TestStartKeepsTheSnapshotOfAPollThatPublishedFirst(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.svc.snapshots.set(1, &snapshot.Snapshot{PR: snapshot.PR{Title: "read by the poll"}})
	w := fx.start()

	snap, err := fx.svc.View(context.Background(), w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.PR.Title != "read by the poll" {
		t.Fatalf("View() title = %q, want the newer snapshot of the poll", snap.PR.Title)
	}
}

func TestViewIsNotChangedByTheRestOfThePoll(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.answered()
	w := fx.start()
	fx.agentIdle(w)

	fx.poll(w)
	if asked := fx.asked(); len(asked) != 1 {
		t.Fatalf("asked = %v, want the poll to ask bob for a new review", asked)
	}
	snap, err := fx.svc.View(context.Background(), w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.PR.RequestedReviewers) != 0 {
		t.Fatalf("View() requested reviewers = %v, want none as GitHub returned them", snap.PR.RequestedReviewers)
	}
}

func TestViewWaitsForTheLockOfTheWatch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()

	unlock := fx.svc.locks.Lock(w.ID)
	done := make(chan error, 1)
	go func() {
		_, err := fx.svc.View(context.Background(), w.ID)
		done <- err
	}()
	testutil.Eventually(t, func() bool { return fx.lockUsers(w.ID) >= 2 }, "View to wait on the lock of the watch")
	if _, err := fx.svc.stop(context.Background(), w.ID, store.StopUser, "", StopOptions{}); err != nil {
		unlock()
		t.Fatal(err)
	}
	unlock()
	if err := <-done; !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("View() during a stop error = %v, want ErrWatchStopped", err)
	}
}

func TestDiffReadsTheDiffOfThePullRequestFromGitHub(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.Diff = "diff --git a/x.go b/x.go\n+new\n" })
	w := fx.start()
	fx.api.Reset()

	diff, err := fx.svc.Diff(context.Background(), w.ID)
	if err != nil || diff != "diff --git a/x.go b/x.go\n+new\n" {
		t.Fatalf("Diff() = %q, %v", diff, err)
	}
	calls := fx.api.Calls(ghfake.RoutePull)
	if len(calls) != 1 || !strings.Contains(calls[0].Accept, "diff") {
		t.Fatalf("pull reads = %+v, want one read of the diff media type", calls)
	}

	if _, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Diff(context.Background(), w.ID); !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("Diff() of a stopped watch error = %v, want ErrWatchStopped", err)
	}
}

func TestDiffWaitsForAStopThatHoldsTheLockOfTheWatch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.Diff = "diff --git a/x.go b/x.go\n+new\n" })
	w := fx.start()

	unlock := fx.svc.locks.Lock(w.ID)
	done := make(chan error, 1)
	go func() {
		_, err := fx.svc.Diff(context.Background(), w.ID)
		done <- err
	}()
	testutil.Eventually(t, func() bool { return fx.lockUsers(w.ID) >= 2 }, "Diff to wait on the lock of the watch")
	if _, err := fx.svc.stop(context.Background(), w.ID, store.StopUser, "", StopOptions{}); err != nil {
		unlock()
		t.Fatal(err)
	}
	unlock()
	if err := <-done; !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("Diff() during a stop error = %v, want ErrWatchStopped", err)
	}
}

func TestDiffOfASelfWatch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.Diff = "diff --git a/x.go b/x.go\n+new\n" })
	w := fx.startSelf()

	if diff, err := fx.svc.Diff(context.Background(), w.ID); err != nil || diff == "" {
		t.Fatalf("Diff() of a self watch = %q, %v", diff, err)
	}
	if _, err := fx.svc.View(context.Background(), w.ID); err != nil {
		t.Fatalf("View() of a self watch error = %v", err)
	}
}
