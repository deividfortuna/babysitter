package prwatch

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/store"
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

func TestDiffOfAForkReadsTheBaseFromTheBaseRepository(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.api.Repo("alice/hello")
	fx.update(func() { fx.pr.HeadRepo = "alice/hello" })
	fork := t.TempDir()
	gitIn(t, fork, []string{"init", "-q"}, []string{"remote", "add", "origin", "git@github.com:alice/hello.git"},
		[]string{"config", "user.name", "Alice"}, []string{"config", "user.email", "alice@example.com"})
	fx.co.dir = fork
	fx.rel.set(func(f *fakeRelease) {
		f.branches = map[string]string{"main": "stale"}
		f.upstream = map[string]string{"main": "m2"}
		f.history["m2"] = []string{"m1"}
		f.history["abc"] = []string{"m1"}
	})
	w, err := fx.svc.Start(context.Background(), StartRequest{Target: pr3})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	d, err := fx.svc.Diff(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if d.Base != "m1" || d.Head != "abc" {
		t.Fatalf("Diff() = %+v, want from the merge base m1 with the main of octo/hello", d)
	}
	var urls []string
	fx.rel.set(func(f *fakeRelease) { urls = slices.Clone(f.fetchURLs) })
	if len(urls) != 1 {
		t.Fatalf("fetched from %v, want the base repository once", urls)
	}
	if owner, name, err := store.ParseFullName(urls[0]); err != nil || owner+"/"+name != "octo/hello" {
		t.Fatalf("fetched the base from %s, want octo/hello", urls[0])
	}
}

func TestSiblingURLKeepsTheTransportOfOrigin(t *testing.T) {
	t.Parallel()
	for origin, want := range map[string]string{
		"https://github.com/alice/hello.git":                   "https://github.com/octo/hello.git",
		"https://x-access-token:secret@github.com/alice/hello": "https://github.com/octo/hello.git",
		"git@github.com:alice/hello.git":                       "git@github.com:octo/hello.git",
		"ssh://git@github.com/alice/hello.git/":                "ssh://git@github.com/octo/hello.git",
	} {
		if got := siblingURL(origin, "octo/hello"); got != want {
			t.Errorf("siblingURL(%q) = %q, want %q", origin, got, want)
		}
	}
}

func TestViewAndDiffWaitForTheLockOfTheWatch(t *testing.T) {
	t.Parallel()
	for name, read := range map[string]func(*Service, int64) error{
		"view": func(s *Service, id int64) error { _, err := s.View(context.Background(), id); return err },
		"diff": func(s *Service, id int64) error { _, err := s.Diff(context.Background(), id); return err },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			w := fx.start()

			unlock := fx.svc.locks.Lock(w.ID)
			done := make(chan error, 1)
			go func() { done <- read(fx.svc, w.ID) }()
			select {
			case err := <-done:
				unlock()
				t.Fatalf("%s returned %v while a stop could hold the lock of the watch", name, err)
			case <-time.After(200 * time.Millisecond):
			}
			unlock()
			if err := <-done; err != nil {
				t.Fatalf("%s after the lock error = %v", name, err)
			}
		})
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
