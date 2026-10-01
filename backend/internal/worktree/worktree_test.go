package worktree

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func repos(t *testing.T) (origin, author, other string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	author = filepath.Join(root, "author")
	other = filepath.Join(root, "other")
	git(t, root, "init", "-q", "--bare", "-b", "main", origin)
	git(t, root, "clone", "-q", origin, author)
	git(t, author, "config", "user.name", "t")
	git(t, author, "config", "user.email", "t@example.com")
	git(t, author, "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init")
	write(t, author, "a.txt", "one\n")
	git(t, author, "add", "a.txt")
	git(t, author, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "add a")
	git(t, author, "push", "-q", "origin", "main")
	git(t, author, "checkout", "-q", "-b", "fix")
	write(t, author, "b.txt", "two\n")
	git(t, author, "add", "b.txt")
	git(t, author, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "add b")
	git(t, author, "push", "-q", "-u", "origin", "fix")
	git(t, root, "clone", "-q", origin, other)
	return origin, author, other
}

func TestCreateAndRemove(t *testing.T) {
	t.Parallel()
	_, author, _ := repos(t)
	ctx := context.Background()
	g := New()
	dir := filepath.Join(t.TempDir(), "wt", "octo-hello-3")

	if err := g.Create(ctx, author, dir, "babysitter/fix", "origin/fix"); err != nil {
		t.Fatal(err)
	}
	if b := git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); b != "babysitter/fix" {
		t.Fatalf("worktree branch = %q", b)
	}
	if h := git(t, dir, "rev-parse", "HEAD"); h != git(t, author, "rev-parse", "fix") {
		t.Fatalf("worktree head = %s, want the head of fix", h)
	}
	if b := git(t, author, "rev-parse", "--abbrev-ref", "HEAD"); b != "fix" {
		t.Fatalf("author branch = %q", b)
	}

	if err := g.Remove(ctx, author, dir, "babysitter/fix"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("worktree dir after Remove: %v", err)
	}
	if out := git(t, author, "branch", "--list", "babysitter/fix"); out != "" {
		t.Fatalf("branch after Remove = %q", out)
	}
	if err := g.Remove(ctx, author, dir, "babysitter/fix"); err != nil {
		t.Fatal(err)
	}
}

func TestFetchNeedsPushedBranch(t *testing.T) {
	t.Parallel()
	_, author, _ := repos(t)
	g := New()
	err := g.Fetch(context.Background(), author, "origin/nope")
	if err == nil || !strings.Contains(err.Error(), "was the branch pushed") {
		t.Fatalf("Fetch of unpushed branch = %v", err)
	}
	if err := g.Fetch(context.Background(), author, "nope"); err == nil {
		t.Fatal("Fetch with a bare upstream expected an error")
	}
}

func TestFetchFailsInsteadOfAskingForCredentials(t *testing.T) {
	_, author, _ := repos(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	git(t, author, "remote", "set-url", "origin", srv.URL+"/octo/hello.git")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_ASKPASS", "")
	t.Setenv("SSH_ASKPASS", "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := New().Fetch(ctx, author, "origin/fix")
	if err == nil || !strings.Contains(err.Error(), "terminal prompts disabled") {
		t.Fatalf("Fetch() error = %v, want git to refuse the credential prompt", err)
	}
}

func TestCreateNeedsTheUpstreamFetched(t *testing.T) {
	t.Parallel()
	_, author, _ := repos(t)
	if err := New().Create(context.Background(), author, filepath.Join(t.TempDir(), "wt"), "babysitter/nope", "origin/nope"); err == nil {
		t.Fatal("Create of a branch origin never had expected an error")
	}
}

func TestCreateStartsAtTheCommitTheLastFetchBrought(t *testing.T) {
	t.Parallel()
	_, author, other := repos(t)
	ctx := context.Background()
	g := New()
	git(t, other, "checkout", "-q", "fix")
	write(t, other, "c.txt", "three\n")
	git(t, other, "add", "c.txt")
	git(t, other, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "add c")
	git(t, other, "push", "-q", "origin", "fix")
	pushed := git(t, other, "rev-parse", "HEAD")

	stale := filepath.Join(t.TempDir(), "stale")
	if err := g.Create(ctx, author, stale, "babysitter/stale", "origin/fix"); err != nil {
		t.Fatal(err)
	}
	if h := git(t, stale, "rev-parse", "HEAD"); h == pushed {
		t.Fatal("Create fetched on its own, and the fetch belongs to Fetch")
	}

	if err := g.Fetch(ctx, author, "origin/fix"); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(t.TempDir(), "fresh")
	if err := g.Create(ctx, author, fresh, "babysitter/fresh", "origin/fix"); err != nil {
		t.Fatal(err)
	}
	if h := git(t, fresh, "rev-parse", "HEAD"); h != pushed {
		t.Fatalf("worktree head = %s, want %s that the fetch brought", h, pushed)
	}
}

func TestRemoveDeletesTheDirectoryWhenTheCheckoutIsGone(t *testing.T) {
	t.Parallel()
	_, author, _ := repos(t)
	ctx := context.Background()
	g := New()
	dir := filepath.Join(t.TempDir(), "wt")
	if err := g.Create(ctx, author, dir, "babysitter/fix", "origin/fix"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(author); err != nil {
		t.Fatal(err)
	}
	if err := g.Remove(ctx, author, dir, "babysitter/fix"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("worktree dir after Remove: %v", err)
	}
}

func TestRemoveLeavesADirectoryThatIsNotAWorktree(t *testing.T) {
	t.Parallel()
	_, author, _ := repos(t)
	dir := filepath.Join(t.TempDir(), "notaworktree")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	err := New().Remove(context.Background(), author, dir, "")
	if err == nil || !strings.Contains(err.Error(), "not a git worktree") {
		t.Fatalf("Remove() error = %v, want a refusal", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the directory was deleted: %v", err)
	}
}

func TestRemoveLeavesADirectoryWithAGitFileThatIsNotAWorktree(t *testing.T) {
	t.Parallel()
	_, author, _ := repos(t)
	dir := filepath.Join(t.TempDir(), "notaworktree")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("notes about git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("the work of the author\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := New().Remove(context.Background(), author, dir, "")
	if err == nil || !strings.Contains(err.Error(), "not a git worktree") {
		t.Fatalf("Remove() error = %v, want a refusal", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatalf("the directory was deleted: %v", err)
	}
}

func TestRemoveReportsAStatThatFails(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits that make a stat fail")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads every directory")
	}
	_, author, _ := repos(t)
	ctx := context.Background()
	g := New()
	parent := filepath.Join(t.TempDir(), "locked")
	dir := filepath.Join(parent, "wt")
	if err := g.Create(ctx, author, dir, "babysitter/fix", "origin/fix"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o750) })

	if err := g.Remove(ctx, author, dir, "babysitter/fix"); err == nil {
		t.Fatal("Remove() = nil, want the stat error; the worktree is still on disk")
	}
}

func TestRemoveReportsABranchThatStays(t *testing.T) {
	t.Parallel()
	_, author, _ := repos(t)
	ctx := context.Background()
	g := New()
	dir := filepath.Join(t.TempDir(), "wt")
	if err := g.Create(ctx, author, dir, "babysitter/fix", "origin/fix"); err != nil {
		t.Fatal(err)
	}
	git(t, author, "checkout", "-q", "-b", "babysitter/held")

	if err := g.Remove(ctx, author, dir, "babysitter/held"); err == nil {
		t.Fatal("Remove() = nil, want the branch error; babysitter/held is checked out and stays")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("worktree dir after Remove: %v", err)
	}
}

func TestRemoveIgnoresABranchThatIsAlreadyGone(t *testing.T) {
	t.Parallel()
	_, author, _ := repos(t)
	ctx := context.Background()
	g := New()
	dir := filepath.Join(t.TempDir(), "wt")

	if err := g.Remove(ctx, author, dir, "babysitter/never-made"); err != nil {
		t.Fatalf("Remove() of a watch that was never started = %v", err)
	}
}
