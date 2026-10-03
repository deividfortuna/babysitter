package gitrepo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

func TestALocalCommandDoesNotFetchWithoutTheAppToken(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := context.Background()
	source, remote, clone := t.TempDir(), t.TempDir(), t.TempDir()
	git(t, source, "init", "-q", "-b", "main")
	git(t, source, "-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init")
	if err := os.WriteFile(filepath.Join(source, "file"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, source, "add", "file")
	git(t, source, "-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "file")
	git(t, remote, "clone", "-q", "--bare", source, ".")
	git(t, remote, "config", "uploadpack.allowFilter", "true")
	git(t, clone, "clone", "-q", "--filter=blob:none", "--no-checkout", "file://"+remote, ".")

	noToken := AuthEnv(func(context.Context) ([]string, error) { return nil, errors.New("the GitHub App sign in expired") })
	env, err := noToken.Env(ctx, "cat-file")
	if err != nil {
		t.Fatalf("Env of a local command: %v", err)
	}

	local := exec.Command("git", "rev-parse", "HEAD")
	local.Dir, local.Env = clone, append(os.Environ(), env...)
	if out, err := local.CombinedOutput(); err != nil {
		t.Fatalf("git rev-parse without the token: %v\n%s", err, out)
	}
	lazy := exec.Command("git", "cat-file", "-p", "HEAD:file")
	lazy.Dir, lazy.Env = clone, append(os.Environ(), env...)
	if out, err := lazy.CombinedOutput(); err == nil {
		t.Fatalf("git cat-file fetched the missing blob without the token: %q", out)
	}
}

func TestCheckAppRemote(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		url  string
		want error
	}{
		{"https://github.com/octo/hello.git", nil},
		{"https://GitHub.com/octo/hello.git", nil},
		{"https://github.com./octo/hello.git", nil},
		{"https://github.com:443/octo/hello.git", nil},
		{"https://github.com:8443/octo/hello.git", ErrNotHTTPS},
		{"https://www.github.com/octo/hello.git", ErrNotHTTPS},
		{"https://ssh.github.com/octo/hello.git", ErrNotHTTPS},
		{"git@GitHub.com:octo/hello.git", ErrNotHTTPS},
		{"github.com:octo/hello.git", ErrNotHTTPS},
		{"ssh://git@GITHUB.COM/octo/hello.git", ErrNotHTTPS},
		{"ssh://git@ssh.github.com:443/octo/hello.git", ErrNotHTTPS},
		{"http://github.com/octo/hello.git", ErrNotHTTPS},
		{"git@github.com.:octo/hello.git", ErrNotHTTPS},
		{"ssh://git@GitHub.com./octo/hello.git", ErrNotHTTPS},
		{"https://me:ghp_personal@github.com/octo/hello.git", ErrTokenInURL},
		{"https://me@github.com/octo/hello.git", nil},
		{"https://me:secret@gitlab.com/octo/hello.git", nil},
		{"git@gitlab.com:octo/hello.git", nil},
		{"github-work:octo/hello.git", nil},
		{"/tmp/origin.git", nil},
	} {
		if err := CheckAppRemote(tc.url); !errors.Is(err, tc.want) {
			t.Errorf("CheckAppRemote(%q) = %v, want %v", tc.url, err, tc.want)
		}
	}
}

func TestCheckAppRemoteKeepsTheTokenOutOfTheError(t *testing.T) {
	t.Parallel()
	for _, url := range []string{"https://me:ghp_personal@github.com/octo/hello.git", "http://me:ghp_personal@github.com/octo/hello.git"} {
		err := CheckAppRemote(url)
		if err == nil || strings.Contains(err.Error(), "ghp_personal") {
			t.Errorf("CheckAppRemote(%q) = %v, want an error without the token", url, err)
		}
	}
}
