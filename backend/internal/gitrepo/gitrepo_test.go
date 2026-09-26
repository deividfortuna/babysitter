package gitrepo

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/store"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestBranchAndOrigin(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := context.Background()
	dir := t.TempDir()

	if _, err := RemoteURL(ctx, dir, "origin"); err == nil || !strings.Contains(err.Error(), "git remote get-url origin") {
		t.Fatalf("RemoteURL outside a repository: %v", err)
	}

	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "remote", "add", "origin", "git@github.com:octo/hello.git")
	git(t, dir, "-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init")
	git(t, dir, "checkout", "-q", "-b", "fix")

	if b, err := CurrentBranch(ctx, dir); err != nil || b != "fix" {
		t.Fatalf("CurrentBranch = %q, %v", b, err)
	}
	u, err := RemoteURL(ctx, dir, "origin")
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if owner, name, err := store.ParseFullName(u); err != nil || owner != "octo" || name != "hello" {
		t.Fatalf("RemoteURL = %q, parsed as %s/%s, %v", u, owner, name, err)
	}
	if _, err := RemoteURL(ctx, dir, "upstream"); err == nil || !strings.Contains(err.Error(), "upstream") {
		t.Fatalf("RemoteURL of a missing remote err = %v", err)
	}
	git(t, dir, "remote", "add", "upstream", "https://github.com/up/hello.git")
	u, err = RemoteURL(ctx, dir, "upstream")
	if err != nil {
		t.Fatalf("RemoteURL upstream: %v", err)
	}
	if owner, name, err := store.ParseFullName(u); err != nil || owner != "up" || name != "hello" {
		t.Fatalf("RemoteURL upstream = %q, parsed as %s/%s, %v", u, owner, name, err)
	}

	git(t, dir, "checkout", "-q", "--detach")
	if _, err := CurrentBranch(ctx, dir); !errors.Is(err, ErrDetachedHead) {
		t.Fatalf("detached CurrentBranch err = %v", err)
	}
}

func TestConfigValue(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := context.Background()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.name", "Alice Author")

	if v, err := ConfigValue(ctx, dir, "user.name"); err != nil || v != "Alice Author" {
		t.Fatalf("ConfigValue(user.name) = %q, %v", v, err)
	}
	if v, err := ConfigValue(ctx, dir, "babysitter.nothing"); err != nil || v != "" {
		t.Fatalf("ConfigValue(missing) = %q, %v", v, err)
	}
}
