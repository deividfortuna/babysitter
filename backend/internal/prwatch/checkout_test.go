package prwatch

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"sync"
	"testing"

	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

type fakeCheckouts struct {
	mu    sync.Mutex
	dir   string
	err   error
	repos []string
}

func (f *fakeCheckouts) Ensure(_ context.Context, repo string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.repos = append(f.repos, repo)
	return f.dir, f.err
}

func (f *fakeCheckouts) ensured() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.repos)
}

func gitIn(t *testing.T, dir string, args ...[]string) {
	t.Helper()
	for _, a := range args {
		cmd := exec.Command("git", a...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
}

var pr3 = snapshot.Target{Owner: "octo", Name: "hello", Number: 3}

func TestStartWithoutACheckoutUsesTheManagedOne(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)

	w, err := fx.svc.Start(context.Background(), StartRequest{Target: pr3})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := fx.co.ensured(); !slices.Equal(got, []string{"octo/hello"}) {
		t.Fatalf("checkouts made = %v", got)
	}
	if w.SourceDir != fx.dir {
		t.Fatalf("source dir = %q, want the managed checkout %q", w.SourceDir, fx.dir)
	}
	if !slices.Equal(fx.git.sources, []string{fx.dir}) || !slices.Equal(fx.git.fetched(), []string{"origin/fix"}) {
		t.Fatalf("worktree sources = %v, fetches = %v", fx.git.sources, fx.git.fetched())
	}
}

func TestStartWithACheckoutMakesNoManagedOne(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.start()
	if got := fx.co.ensured(); len(got) != 0 {
		t.Fatalf("checkouts made = %v", got)
	}
}

func TestStartWithoutACheckoutClonesTheHeadRepository(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.api.Repo("alice/hello")
	fx.update(func() { fx.pr.HeadRepo = "alice/hello" })
	fork := t.TempDir()
	gitIn(t, fork, []string{"init", "-q"}, []string{"remote", "add", "origin", "https://github.com/alice/hello.git"},
		[]string{"config", "user.name", "Alice"}, []string{"config", "user.email", "alice@example.com"})
	fx.co.dir = fork

	w, err := fx.svc.Start(context.Background(), StartRequest{Target: pr3})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := fx.co.ensured(); !slices.Equal(got, []string{"alice/hello"}) {
		t.Fatalf("checkouts made = %v", got)
	}
	if w.SourceDir != fork {
		t.Fatalf("source dir = %q, want the clone of the fork", w.SourceDir)
	}
}

func TestStartWithoutACheckoutRejections(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		req  StartRequest
		want error
	}{
		"no repository": {StartRequest{Target: snapshot.Target{Number: 3}}, snapshot.ErrIncompleteTarget},
		"no number":     {StartRequest{Target: snapshot.Target{Owner: "octo", Name: "hello"}}, snapshot.ErrIncompleteTarget},
		"self provider": {StartRequest{Target: pr3, Provider: ProviderSelf}, ErrNoCheckout},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			if _, err := fx.svc.Start(context.Background(), tc.req); !errors.Is(err, tc.want) {
				t.Fatalf("Start() error = %v, want %v", err, tc.want)
			}
			if got := fx.co.ensured(); len(got) != 0 {
				t.Fatalf("checkouts made = %v", got)
			}
		})
	}
}

func TestStartWithoutACheckoutStopsWhenTheCloneFails(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.co.err = errors.New("clone https://github.com/octo/hello.git: exit 128")

	if _, err := fx.svc.Start(context.Background(), StartRequest{Target: pr3}); !errors.Is(err, fx.co.err) {
		t.Fatalf("Start() error = %v", err)
	}
	if len(fx.git.createdDirs()) != 0 {
		t.Fatalf("a failed clone made a worktree: %v", fx.git.createdDirs())
	}
	if _, err := fx.st.FindActiveWatch(context.Background(), store.WatchKey{Owner: "octo", Name: "hello", Number: 3}); !errors.Is(err, store.ErrWatchNotFound) {
		t.Fatalf("a failed clone left a watch: %v", err)
	}
}

func TestStartWithoutACheckoutMakesNoneForAWatchedPullRequest(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.start()

	if _, err := fx.svc.Start(context.Background(), StartRequest{Target: pr3}); !errors.Is(err, store.ErrWatchExists) {
		t.Fatalf("Start() error = %v, want ErrWatchExists", err)
	}
	if got := fx.co.ensured(); len(got) != 0 {
		t.Fatalf("checkouts made = %v", got)
	}
}
