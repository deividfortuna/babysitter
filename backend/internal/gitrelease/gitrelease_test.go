package gitrelease

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/gitrepo"
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

func commit(t *testing.T, dir, file, content, message string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", file)
	git(t, dir, "-c", "commit.gpgsign=false", "commit", "-q", "-m", message)
	return git(t, dir, "rev-parse", "HEAD")
}

func repos(t *testing.T) (origin, work, other string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	work = filepath.Join(root, "work")
	other = filepath.Join(root, "other")
	git(t, root, "init", "-q", "--bare", "-b", "main", origin)
	git(t, root, "clone", "-q", origin, work)
	git(t, work, "config", "user.name", "t")
	git(t, work, "config", "user.email", "t@example.com")
	commit(t, work, "a.txt", "one\n", "add a")
	git(t, work, "push", "-q", "origin", "main")
	git(t, work, "checkout", "-q", "-b", "fix")
	commit(t, work, "b.txt", "two\n", "add b")
	git(t, work, "push", "-q", "-u", "origin", "fix")
	git(t, root, "clone", "-q", "-b", "fix", origin, other)
	git(t, other, "config", "user.name", "o")
	git(t, other, "config", "user.email", "o@example.com")
	return origin, work, other
}

func TestAFastForwardGoesOutPlain(t *testing.T) {
	t.Parallel()
	origin, work, _ := repos(t)
	ctx := context.Background()
	g := New()

	head, err := g.Fetch(ctx, work, "fix")
	if err != nil || head != git(t, origin, "rev-parse", "fix") {
		t.Fatalf("Fetch() = %q, %v", head, err)
	}
	sha := commit(t, work, "c.txt", "three\n", "add c")
	if got, err := g.Head(ctx, work); err != nil || got != sha {
		t.Fatalf("Head() = %q, %v", got, err)
	}
	if ok, err := g.Contains(ctx, work, sha, head); err != nil || !ok {
		t.Fatalf("the work does not contain the head it started from: %v, %v", ok, err)
	}
	if ok, err := g.Contains(ctx, work, head, sha); err != nil || ok {
		t.Fatalf("the head contains the new commit: %v, %v", ok, err)
	}
	if err := g.Push(ctx, work, Push{SHA: sha, Branch: "fix"}); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if got := git(t, origin, "rev-parse", "fix"); got != sha {
		t.Fatalf("origin fix = %s, want %s", got, sha)
	}
}

func TestThePinnedLeaseRefusesABranchSomebodyMoved(t *testing.T) {
	t.Parallel()
	origin, work, other := repos(t)
	ctx := context.Background()
	g := New()
	head, err := g.Fetch(ctx, work, "fix")
	if err != nil {
		t.Fatal(err)
	}
	git(t, work, "commit", "-q", "--amend", "-m", "add b, reworded")
	rewrite := git(t, work, "rev-parse", "HEAD")

	theirs := commit(t, other, "d.txt", "theirs\n", "their commit")
	git(t, other, "push", "-q", "origin", "fix")
	if _, err := g.Fetch(ctx, work, "fix"); err != nil {
		t.Fatal(err)
	}

	err = g.Push(ctx, work, Push{SHA: rewrite, Branch: "fix", Lease: head})
	if !errors.Is(err, ErrLeaseRefused) {
		t.Fatalf("Push() error = %v, want ErrLeaseRefused", err)
	}
	if got := git(t, origin, "rev-parse", "fix"); got != theirs {
		t.Fatalf("origin fix = %s, want their commit %s", got, theirs)
	}

	if err := g.Push(ctx, work, Push{SHA: rewrite, Branch: "fix", Lease: theirs}); err != nil {
		t.Fatalf("Push() with the lease on the head it read error = %v", err)
	}
	if got := git(t, origin, "rev-parse", "fix"); got != rewrite {
		t.Fatalf("origin fix = %s, want %s", got, rewrite)
	}
}

func TestMissingListsTheCommitsOfTheHeadTheWorkLacks(t *testing.T) {
	t.Parallel()
	_, work, other := repos(t)
	ctx := context.Background()
	g := New()
	start, err := g.Fetch(ctx, work, "fix")
	if err != nil {
		t.Fatal(err)
	}
	theirs := commit(t, other, "d.txt", "theirs\n", "their commit")
	git(t, other, "push", "-q", "origin", "fix")
	commit(t, work, "c.txt", "mine\n", "my commit")
	remote, err := g.Fetch(ctx, work, "fix")
	if err != nil {
		t.Fatal(err)
	}
	base, err := g.MergeBase(ctx, work, "HEAD", remote)
	if err != nil || base != start {
		t.Fatalf("MergeBase() = %q, %v, want %s", base, err, start)
	}
	head, _ := g.Head(ctx, work)
	missing, err := g.Missing(ctx, work, head, remote, base)
	if err != nil || !slices.Equal(missing, []string{theirs}) {
		t.Fatalf("Missing() = %v, %v, want [%s]", missing, err, theirs)
	}

	if err := g.Rebase(ctx, work, remote); err != nil {
		t.Fatalf("Rebase() error = %v", err)
	}
	head, _ = g.Head(ctx, work)
	if missing, err := g.Missing(ctx, work, head, remote, base); err != nil || len(missing) != 0 {
		t.Fatalf("Missing() after the rebase = %v, %v", missing, err)
	}
	if ok, _ := g.Contains(ctx, work, head, remote); !ok {
		t.Fatal("the rebased work does not contain the remote head")
	}
}

func TestFastForwardMovesOnlyABranchBehind(t *testing.T) {
	t.Parallel()
	_, work, other := repos(t)
	ctx := context.Background()
	g := New()
	theirs := commit(t, other, "d.txt", "theirs\n", "their commit")
	git(t, other, "push", "-q", "origin", "fix")
	remote, err := g.Fetch(ctx, work, "fix")
	if err != nil || remote != theirs {
		t.Fatalf("Fetch() = %q, %v", remote, err)
	}
	if err := g.FastForward(ctx, work, remote); err != nil {
		t.Fatalf("FastForward() error = %v", err)
	}
	if head, _ := g.Head(ctx, work); head != theirs {
		t.Fatalf("head = %s, want %s", head, theirs)
	}

	commit(t, work, "c.txt", "mine\n", "my commit")
	commit(t, other, "e.txt", "more\n", "their second commit")
	git(t, other, "push", "-q", "origin", "fix")
	remote, _ = g.Fetch(ctx, work, "fix")
	if err := g.FastForward(ctx, work, remote); err == nil {
		t.Fatal("FastForward() of a diverged branch succeeded")
	}
}

func TestResetFollowsADivergedBranchAndKeepsLocalChanges(t *testing.T) {
	t.Parallel()
	_, work, other := repos(t)
	ctx := context.Background()
	g := New()
	commit(t, work, "c.txt", "mine\n", "my commit")
	theirs := commit(t, other, "d.txt", "theirs\n", "their commit")
	git(t, other, "push", "-q", "origin", "fix")
	remote, err := g.Fetch(ctx, work, "fix")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "notes.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := g.Reset(ctx, work, remote); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if head, _ := g.Head(ctx, work); head != theirs {
		t.Fatalf("head = %s, want %s", head, theirs)
	}
	if status := git(t, work, "status", "--porcelain"); status != "?? notes.txt" {
		t.Fatalf("status = %q, want the local file kept", status)
	}
}

func TestDirtyListsTheChangesThatAreNotCommitted(t *testing.T) {
	t.Parallel()
	_, work, _ := repos(t)
	ctx := context.Background()
	g := New()
	if files, err := g.Dirty(ctx, work); err != nil || len(files) != 0 {
		t.Fatalf("Dirty() of a clean worktree = %q, %v", files, err)
	}
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "notes.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := g.Dirty(ctx, work)
	if err != nil || !slices.Equal(files, []string{" M a.txt", "?? notes.txt"}) {
		t.Fatalf("Dirty() = %q, %v", files, err)
	}
}

func TestHasMergesSeesAMergeAfterTheHead(t *testing.T) {
	t.Parallel()
	_, work, _ := repos(t)
	ctx := context.Background()
	g := New()
	head, _ := g.Head(ctx, work)
	mine := commit(t, work, "c.txt", "mine\n", "my commit")
	if merges, err := g.HasMerges(ctx, work, head, mine); err != nil || merges {
		t.Fatalf("HasMerges() of plain commits = %v, %v", merges, err)
	}

	git(t, work, "checkout", "-q", "main")
	commit(t, work, "m.txt", "main\n", "main moves")
	git(t, work, "checkout", "-q", "fix")
	git(t, work, "-c", "commit.gpgsign=false", "merge", "-q", "--no-ff", "--no-edit", "main")
	merged, _ := g.Head(ctx, work)
	if merges, err := g.HasMerges(ctx, work, head, merged); err != nil || !merges {
		t.Fatalf("HasMerges() after a merge of main = %v, %v", merges, err)
	}
}

func TestARebaseThatConflictsIsAborted(t *testing.T) {
	t.Parallel()
	_, work, other := repos(t)
	ctx := context.Background()
	g := New()
	commit(t, other, "b.txt", "theirs\n", "their b")
	git(t, other, "push", "-q", "origin", "fix")
	mine := commit(t, work, "b.txt", "mine\n", "my b")
	remote, err := g.Fetch(ctx, work, "fix")
	if err != nil {
		t.Fatal(err)
	}

	err = g.Rebase(ctx, work, remote)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Rebase() error = %v, want ErrConflict", err)
	}
	var conflict *ConflictError
	if !errors.As(err, &conflict) || !slices.Equal(conflict.Files, []string{"b.txt"}) || !strings.Contains(err.Error(), "b.txt") {
		t.Fatalf("Rebase() error = %v, want the file that conflicts", err)
	}
	if head, _ := g.Head(ctx, work); head != mine {
		t.Fatalf("head = %s, want %s after the abort", head, mine)
	}
	if status := git(t, work, "status", "--porcelain"); status != "" {
		t.Fatalf("the worktree is not clean after the abort: %q", status)
	}
}

func TestAMergeKeepsTheCommitsOfBothSides(t *testing.T) {
	t.Parallel()
	_, work, other := repos(t)
	ctx := context.Background()
	g := New()
	commit(t, other, "c.txt", "theirs\n", "their c")
	git(t, other, "push", "-q", "origin", "fix")
	mine := commit(t, work, "d.txt", "mine\n", "my d")
	remote, err := g.Fetch(ctx, work, "fix")
	if err != nil {
		t.Fatal(err)
	}

	if err := g.Merge(ctx, work, remote); err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	head, _ := g.Head(ctx, work)
	for _, want := range []string{mine, remote} {
		if kept, err := g.Contains(ctx, work, head, want); err != nil || !kept {
			t.Fatalf("the merge %s lost %s: %v, %v", head, want, kept, err)
		}
	}
}

func diverged(t *testing.T) (work, mine, theirs string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	work = t.TempDir()
	git(t, work, "init", "-q", "-b", "fix")
	git(t, work, "config", "user.name", "t")
	git(t, work, "config", "user.email", "t@example.com")
	commit(t, work, "a.txt", "one\n", "add a")
	git(t, work, "checkout", "-q", "-b", "moved")
	theirs = commit(t, work, "c.txt", "theirs\n", "their c")
	git(t, work, "checkout", "-q", "fix")
	mine = commit(t, work, "d.txt", "mine\n", "my d")
	return work, mine, theirs
}

func mergedBoth(t *testing.T, g *Runner, work, mine, theirs string) {
	t.Helper()
	head, _ := g.Head(context.Background(), work)
	for _, want := range []string{mine, theirs} {
		if kept, err := g.Contains(context.Background(), work, head, want); err != nil || !kept {
			t.Fatalf("the merge %s lost %s: %v, %v", head, want, kept, err)
		}
	}
}

func TestAMergeSkipsTheHooksOfTheRepository(t *testing.T) {
	hooks := t.TempDir()
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hooks)
	work, mine, theirs := diverged(t)
	for _, name := range []string{"pre-merge-commit", "prepare-commit-msg", "commit-msg"} {
		if err := os.WriteFile(filepath.Join(hooks, name), []byte("#!/bin/sh\nexit 1\n"), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	g := New()

	if err := g.Merge(context.Background(), work, theirs); err != nil {
		t.Fatalf("Merge() error = %v, want the hooks skipped", err)
	}
	mergedBoth(t, g, work, mine, theirs)
}

func TestAMergeMakesAMergeCommitWhenTheCheckoutOnlyFastForwards(t *testing.T) {
	t.Parallel()
	work, mine, theirs := diverged(t)
	git(t, work, "config", "merge.ff", "only")
	g := New()

	if err := g.Merge(context.Background(), work, theirs); err != nil {
		t.Fatalf("Merge() error = %v, want a merge commit whatever merge.ff says", err)
	}
	mergedBoth(t, g, work, mine, theirs)
}

func TestAMergeThatConflictsIsAborted(t *testing.T) {
	t.Parallel()
	_, work, other := repos(t)
	ctx := context.Background()
	g := New()
	commit(t, other, "b.txt", "theirs\n", "their b")
	git(t, other, "push", "-q", "origin", "fix")
	mine := commit(t, work, "b.txt", "mine\n", "my b")
	remote, err := g.Fetch(ctx, work, "fix")
	if err != nil {
		t.Fatal(err)
	}

	err = g.Merge(ctx, work, remote)
	var conflict *ConflictError
	if !errors.As(err, &conflict) || !slices.Equal(conflict.Files, []string{"b.txt"}) {
		t.Fatalf("Merge() error = %v, want the file that conflicts", err)
	}
	if head, _ := g.Head(ctx, work); head != mine {
		t.Fatalf("head = %s, want %s after the abort", head, mine)
	}
	if status := git(t, work, "status", "--porcelain"); status != "" {
		t.Fatalf("the worktree is not clean after the abort: %q", status)
	}
}

func TestARebaseThatNeverStartedSaysWhy(t *testing.T) {
	t.Parallel()
	_, work, other := repos(t)
	ctx := context.Background()
	g := New()
	commit(t, other, "c.txt", "theirs\n", "their c")
	git(t, other, "push", "-q", "origin", "fix")
	commit(t, work, "d.txt", "mine\n", "my d")
	remote, err := g.Fetch(ctx, work, "fix")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "b.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = g.Rebase(ctx, work, remote)
	if err == nil || errors.Is(err, ErrConflict) || strings.Contains(err.Error(), "abort") {
		t.Fatalf("Rebase() error = %v, want the refusal of git alone", err)
	}
	if !strings.Contains(err.Error(), "unstaged changes") {
		t.Fatalf("Rebase() error = %v, want it to say the worktree has changes", err)
	}
}

func TestTheAuthorReadsTheCommitsTheFilesAndTheDiff(t *testing.T) {
	t.Parallel()
	_, work, _ := repos(t)
	ctx := context.Background()
	g := New()
	head, _ := g.Head(ctx, work)
	first := commit(t, work, "c.txt", "three\n", "Add c")
	if err := os.WriteFile(filepath.Join(work, "b.txt"), []byte("two, changed\nand more\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, work, "rm", "-q", "a.txt")
	git(t, work, "add", "b.txt")
	git(t, work, "-c", "commit.gpgsign=false", "commit", "-q", "-m", "Change b\n\nand remove a")
	second := git(t, work, "rev-parse", "HEAD")

	commits, err := g.Log(ctx, work, head, second)
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	want := []Commit{{SHA: first, Subject: "Add c"}, {SHA: second, Subject: "Change b"}}
	if !slices.Equal(commits, want) {
		t.Fatalf("Log() = %+v, want %+v", commits, want)
	}
	files, err := g.Files(ctx, work, head, second)
	if err != nil {
		t.Fatalf("Files() error = %v", err)
	}
	wantFiles := []File{
		{Path: "a.txt", Status: "D", Added: 0, Deleted: 1},
		{Path: "b.txt", Status: "M", Added: 2, Deleted: 1},
		{Path: "c.txt", Status: "A", Added: 1, Deleted: 0},
	}
	if !slices.Equal(files, wantFiles) {
		t.Fatalf("Files() = %+v, want %+v", files, wantFiles)
	}
	diff, truncated, err := g.Diff(ctx, work, head, second, 1<<20)
	if err != nil || truncated {
		t.Fatalf("Diff() = %v, %v", truncated, err)
	}
	for _, line := range []string{"diff --git a/b.txt b/b.txt", "-two", "+two, changed", "+three", "deleted file mode"} {
		if !strings.Contains(diff, line) {
			t.Errorf("the diff lacks %q:\n%s", line, diff)
		}
	}
}

func TestTheDiffIsCutBetweenWholeFiles(t *testing.T) {
	t.Parallel()
	_, work, _ := repos(t)
	ctx := context.Background()
	g := New()
	head, _ := g.Head(ctx, work)
	commit(t, work, "c.txt", "three\n", "Add c")
	last := commit(t, work, "d.txt", "four\n", "Add d")
	whole, _, err := g.Diff(ctx, work, head, last, 1<<20)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	second := strings.Index(whole, "diff --git a/d.txt")

	cut, truncated, err := g.Diff(ctx, work, head, last, second+10)
	if err != nil || !truncated || cut != whole[:second] {
		t.Fatalf("Diff() capped inside the second file = %q, %v, %v, want the first file only", cut, truncated, err)
	}
	none, truncated, err := g.Diff(ctx, work, head, last, 10)
	if err != nil || !truncated || none != "" {
		t.Fatalf("Diff() capped inside the first file = %q, %v, %v, want nothing", none, truncated, err)
	}
}

func TestTheFilesKeepTheirNamesAndSayWhichAreBinary(t *testing.T) {
	t.Parallel()
	_, work, _ := repos(t)
	ctx := context.Background()
	g := New()
	head, _ := g.Head(ctx, work)
	commit(t, work, "café [1].txt", "one\ntwo\n", "Add an accented name")
	last := commit(t, work, "logo.bin", "\x00\x01\x02", "Add a binary")

	files, err := g.Files(ctx, work, head, last)
	if err != nil {
		t.Fatalf("Files() error = %v", err)
	}
	want := []File{
		{Path: "café [1].txt", Status: "A", Added: 2},
		{Path: "logo.bin", Status: "A", Binary: true},
	}
	if !slices.Equal(files, want) {
		t.Fatalf("Files() = %+v, want %+v", files, want)
	}
	one, truncated, err := g.Diff(ctx, work, head, last, 1<<20, "café [1].txt")
	if err != nil || truncated {
		t.Fatalf("Diff() of one file = %v, %v", truncated, err)
	}
	if strings.Count(one, "diff --git ") != 1 || !strings.Contains(one, "+two") {
		t.Fatalf("Diff() of one file =\n%s", one)
	}
}

func TestTheDiffIgnoresTheDiffSettingsOfTheRepository(t *testing.T) {
	t.Parallel()
	_, work, _ := repos(t)
	ctx := context.Background()
	g := New()
	head, _ := g.Head(ctx, work)
	git(t, work, "config", "diff.noprefix", "true")
	git(t, work, "config", "diff.mnemonicPrefix", "true")
	git(t, work, "config", "diff.shout.textconv", "tr a-z A-Z <")
	git(t, work, "config", "color.diff", "always")
	if err := os.WriteFile(filepath.Join(work, ".git", "info", "attributes"), []byte("*.txt diff=shout\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	last := commit(t, work, "c.txt", "quiet\n", "Add c")

	diff, _, err := g.Diff(ctx, work, head, last, 1<<20)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	for _, line := range []string{"diff --git a/c.txt b/c.txt", "+++ b/c.txt", "+quiet"} {
		if !strings.Contains(diff, line) {
			t.Errorf("the diff lacks %q:\n%s", line, diff)
		}
	}
	if strings.Contains(diff, "QUIET") || strings.Contains(diff, "\x1b[") {
		t.Errorf("the diff follows the settings of the repository:\n%s", diff)
	}
}

func TestDiscardPutsTheWorkBranchBack(t *testing.T) {
	t.Parallel()
	_, work, _ := repos(t)
	ctx := context.Background()
	g := New()
	head, _ := g.Head(ctx, work)
	commit(t, work, "c.txt", "three\n", "add c")
	if err := g.Discard(ctx, work, head); err != nil {
		t.Fatalf("Discard() error = %v", err)
	}
	if got, _ := g.Head(ctx, work); got != head {
		t.Fatalf("head = %s, want %s", got, head)
	}
	if _, err := os.Stat(filepath.Join(work, "c.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the file of the discarded commit is still there: %v", err)
	}
}

func TestGitNeverPromptsAndPushesWithoutTheHooks(t *testing.T) {
	t.Parallel()
	var envs [][]string
	var calls [][]string
	g := &Runner{Run: func(_ context.Context, dir, _ string, env []string, name string, args ...string) (string, error) {
		envs = append(envs, env)
		calls = append(calls, append([]string{name}, args...))
		return "", nil
	}}
	if err := g.Push(context.Background(), "/wt", Push{SHA: "abc", Branch: "fix", Lease: "def"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(envs[0], "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("env = %v", envs[0])
	}
	want := []string{"git", "push", "--no-verify", "--force-with-lease=refs/heads/fix:def", "origin", "abc:refs/heads/fix"}
	if !slices.Equal(calls[0], want) {
		t.Fatalf("push = %v, want %v", calls[0], want)
	}
}

func TestEveryGitCommandCarriesTheAuthEnv(t *testing.T) {
	t.Parallel()
	envs := map[string][]string{}
	g := &Runner{
		Run: func(_ context.Context, _, _ string, env []string, _ string, args ...string) (string, error) {
			envs[args[0]] = env
			return "", nil
		},
		Auth: func(context.Context) ([]string, error) { return []string{"GIT_CONFIG_COUNT=1"}, nil },
	}
	ctx := context.Background()
	if _, err := g.Fetch(ctx, "/wt", "fix"); err != nil {
		t.Fatal(err)
	}
	if err := g.Push(ctx, "/wt", Push{SHA: "abc", Branch: "fix"}); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"fetch", "push", "rev-parse"} {
		if !slices.Contains(envs[cmd], "GIT_CONFIG_COUNT=1") || !slices.Contains(envs[cmd], "GIT_TERMINAL_PROMPT=0") {
			t.Errorf("env of %s = %v, want the auth env and no prompt: a partial clone fetches objects on any command", cmd, envs[cmd])
		}
	}
}

func TestPushStopsWhenTheAuthEnvFails(t *testing.T) {
	t.Parallel()
	expired := errors.New("the GitHub App sign in expired")
	ran := false
	g := &Runner{
		Run: func(context.Context, string, string, []string, string, ...string) (string, error) {
			ran = true
			return "", nil
		},
		Auth: func(context.Context) ([]string, error) { return nil, expired },
	}

	err := g.Push(context.Background(), "/wt", Push{SHA: "abc", Branch: "fix"})

	if !errors.Is(err, expired) {
		t.Fatalf("Push() error = %v, want %v", err, expired)
	}
	if ran {
		t.Fatal("git ran without the token of the app, with the credentials of the user")
	}
}

func TestLocalCommandsRunWhenTheAuthEnvFails(t *testing.T) {
	t.Parallel()
	expired := errors.New("the GitHub App sign in expired")
	var env []string
	g := &Runner{
		Run: func(_ context.Context, _, _ string, e []string, _ string, _ ...string) (string, error) {
			env = e
			return "abc", nil
		},
		Auth: func(context.Context) ([]string, error) { return nil, expired },
	}

	head, err := g.Head(context.Background(), "/wt")

	if err != nil || head != "abc" {
		t.Fatalf("Head() = %q, %v; want the local command to run without the token", head, err)
	}
	want := append(slices.Clone(gitrepo.NoPromptEnv), "GIT_NO_LAZY_FETCH=1")
	if !slices.Equal(env, want) {
		t.Fatalf("env = %v, want %v, so git fetches no missing object without the token", env, want)
	}
}

func TestTheParentOfAMergeIsItsFirstParent(t *testing.T) {
	t.Parallel()
	_, work, _ := repos(t)
	ctx := context.Background()
	g := New()

	first := git(t, work, "rev-parse", "HEAD")
	git(t, work, "checkout", "-q", "-b", "side", "main")
	side := commit(t, work, "s.txt", "side\n", "add s")
	git(t, work, "checkout", "-q", "fix")
	git(t, work, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	merge := git(t, work, "rev-parse", "HEAD")

	if got, err := g.Parent(ctx, work, merge); err != nil || got != first {
		t.Fatalf("Parent(merge) = %q, %v, want %q", got, err, first)
	}
	if got, err := g.Parent(ctx, work, side); err != nil || got == side || got == "" {
		t.Fatalf("Parent(side) = %q, %v", got, err)
	}
}
