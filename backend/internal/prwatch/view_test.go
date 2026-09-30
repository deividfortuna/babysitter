package prwatch

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
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

func TestDiffGoesFromTheMergeBaseToThePushedHead(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.rel.set(func(f *fakeRelease) {
		f.branches = map[string]string{"main": "m2"}
		f.history["m2"] = []string{"m1"}
		f.history["abc"] = []string{"m1"}
	})
	w := fx.start()

	d, err := fx.svc.Diff(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if d.Base != "m1" || d.Head != "abc" || !strings.Contains(d.Diff, "-m1\n+abc") {
		t.Fatalf("Diff() = %+v, want from the merge base m1 to the head abc", d)
	}
}

func TestDiffOfASelfWatchReadsItsCheckout(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startSelf()

	if _, err := fx.svc.Diff(context.Background(), w.ID); err != nil {
		t.Fatalf("Diff() of a self watch error = %v", err)
	}
	var dirs []string
	fx.rel.set(func(f *fakeRelease) { dirs = slices.Clone(f.fetchDirs) })
	if len(dirs) != 2 || dirs[0] != fx.dir || dirs[1] != fx.dir {
		t.Fatalf("fetched in %v, want the base and the head in the checkout %s", dirs, fx.dir)
	}
	if _, err := fx.svc.View(context.Background(), w.ID); err != nil {
		t.Fatalf("View() of a self watch error = %v", err)
	}
}
