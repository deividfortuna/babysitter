package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func checkouts(t *testing.T, origin string) *Checkouts {
	t.Helper()
	c := NewCheckouts(filepath.Join(t.TempDir(), "checkouts"))
	c.url = func(string) string { return "file://" + origin }
	return c
}

func TestEnsureClonesOnceAndMakesWorktrees(t *testing.T) {
	t.Parallel()
	origin, author, _ := repos(t)
	ctx := context.Background()
	c := checkouts(t, origin)

	dir, err := c.Ensure(ctx, "Octo/Hello")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(c.root, "octo", "hello"); dir != want {
		t.Fatalf("dir = %s, want %s", dir, want)
	}
	if got := git(t, dir, "remote", "get-url", "origin"); got != "file://"+origin {
		t.Fatalf("origin = %q", got)
	}
	if got := git(t, dir, "config", "--local", "--get-all", "credential.https://github.com.helper"); got != "!gh auth git-credential" {
		t.Fatalf("credential helper = %q", got)
	}
	marker := filepath.Join(dir, ".git", "babysitter-test")
	write(t, filepath.Join(dir, ".git"), "babysitter-test", "kept")

	again, err := c.Ensure(ctx, "octo/hello")
	if err != nil || again != dir {
		t.Fatalf("second Ensure() = %s, %v", again, err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("second Ensure() cloned again: %v", err)
	}

	g := New()
	if err := g.Fetch(ctx, dir, "origin/fix"); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	if err := g.Create(ctx, dir, wt, "babysitter/fix", "origin/fix"); err != nil {
		t.Fatal(err)
	}
	if h := git(t, wt, "rev-parse", "HEAD"); h != git(t, author, "rev-parse", "fix") {
		t.Fatalf("worktree head = %s, want the head of fix", h)
	}
	if _, err := os.Stat(filepath.Join(wt, "b.txt")); err != nil {
		t.Fatalf("worktree has no files: %v", err)
	}
	if err := g.Remove(ctx, dir, wt, "babysitter/fix"); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRestoresTheOriginThatAWorktreeChanged(t *testing.T) {
	t.Parallel()
	origin, _, other := repos(t)
	ctx := context.Background()
	c := checkouts(t, origin)
	dir, err := c.Ensure(ctx, "octo/hello")
	if err != nil {
		t.Fatal(err)
	}
	g := New()
	if err := g.Fetch(ctx, dir, "origin/fix"); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	if err := g.Create(ctx, dir, wt, "babysitter/fix", "origin/fix"); err != nil {
		t.Fatal(err)
	}
	git(t, wt, "remote", "set-url", "origin", other)
	git(t, wt, "remote", "set-url", "--add", "--push", "origin", other)

	if _, err := c.Ensure(ctx, "octo/hello"); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "config", "--get-all", "remote.origin.url"); got != "file://"+origin {
		t.Fatalf("origin url = %q, want file://%s", got, origin)
	}
	if got := git(t, dir, "config", "--get", "--default", "", "remote.origin.pushurl"); got != "" {
		t.Fatalf("origin pushurl = %q, want none", got)
	}
}

func TestEnsureClonesOnceForConcurrentCalls(t *testing.T) {
	t.Parallel()
	origin, _, _ := repos(t)
	c := checkouts(t, origin)

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Go(func() { _, errs[i] = c.Ensure(context.Background(), "octo/hello") })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(c.root, "octo"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "hello" {
		t.Fatalf("octo holds %v, want only hello", entries)
	}
}

func TestEnsureLeavesNothingWhenTheCloneFails(t *testing.T) {
	t.Parallel()
	c := checkouts(t, filepath.Join(t.TempDir(), "missing.git"))

	if _, err := c.Ensure(context.Background(), "octo/hello"); err == nil {
		t.Fatal("Ensure() of a missing origin expected an error")
	}
	entries, err := os.ReadDir(filepath.Join(c.root, "octo"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("octo holds %v after a failed clone", entries)
	}
}

func TestEnsureRejectsNamesOutsideTheRoot(t *testing.T) {
	t.Parallel()
	c := NewCheckouts(t.TempDir())
	for _, repo := range []string{"", "octo", "octo/", "/hello", "../hello", "octo/..", "octo/a/b", `octo\hello`} {
		if _, err := c.Ensure(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "owner/name") {
			t.Errorf("Ensure(%q) error = %v", repo, err)
		}
	}
}

func TestEnsureKeepsAFolderThatIsNotACheckout(t *testing.T) {
	t.Parallel()
	origin, _, _ := repos(t)
	c := checkouts(t, origin)
	dir := filepath.Join(c.root, "octo", "hello")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "notes.txt", "mine\n")

	if _, err := c.Ensure(context.Background(), "octo/hello"); err == nil || !strings.Contains(err.Error(), "not a git checkout") {
		t.Fatalf("Ensure() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatalf("Ensure() changed the folder: %v", err)
	}
}
