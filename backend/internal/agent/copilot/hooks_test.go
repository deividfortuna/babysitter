package copilot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent"
)

var hook = []string{"/usr/local/bin/babysitter", "watch", "hook", "--watch", "7"}

func linkedWorktree(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	main, linked := filepath.Join(root, "main"), filepath.Join(root, "linked")
	gitDir := filepath.Join(main, ".git", "worktrees", "linked")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../..\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linked, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return linked, main
}

func readHooks(t *testing.T, worktree string) hookFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(worktree, ".github", "hooks", "babysitter.json"))
	if err != nil {
		t.Fatalf("hooks file: %v", err)
	}
	var file hookFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("hooks file %s: %v", data, err)
	}
	return file
}

func TestHooksJSON(t *testing.T) {
	t.Parallel()
	data, err := hooksJSON(hook)
	if err != nil {
		t.Fatalf("hooksJSON() error = %v", err)
	}
	if !strings.HasSuffix(string(data), "}\n") {
		t.Fatalf("hooks file does not end with a line break: %q", data)
	}
	var file hookFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("hooks file %s: %v", data, err)
	}
	if file.Version != 1 || len(file.Hooks) != len(hooks) {
		t.Fatalf("hooks file = %s", data)
	}
	for _, h := range hooks {
		entries := file.Hooks[h.key]
		if len(entries) != 1 {
			t.Fatalf("%s = %+v", h.key, entries)
		}
		want := "'/usr/local/bin/babysitter' 'watch' 'hook' '--watch' '7' '" + h.event + "'"
		wantPS := "& " + want
		if e := entries[0]; e.Type != "command" || e.Bash != want || e.Powershell != wantPS || e.TimeoutSec != hookTimeout {
			t.Errorf("%s = %+v, want %q and %q", h.key, e, want, wantPS)
		}
		if !agent.ValidEvent(h.event) {
			t.Errorf("%s reports %q, which the daemon does not know", h.key, h.event)
		}
	}
	for key, event := range map[string]string{
		"sessionStart": agent.EventSessionStart, "userPromptSubmitted": agent.EventUserPromptSubmit,
		"preToolUse": agent.EventPreToolUse, "postToolUse": agent.EventPostToolUse,
		"agentStop": agent.EventStop, "sessionEnd": agent.EventSessionEnd,
	} {
		if entries := file.Hooks[key]; len(entries) != 1 || !strings.HasSuffix(entries[0].Bash, "'"+event+"'") {
			t.Errorf("%s = %+v, want the event %s", key, entries, event)
		}
	}
}

func TestInstallHooksInLinkedWorktree(t *testing.T) {
	t.Parallel()
	worktree, main := linkedWorktree(t)
	if err := installHooks(worktree, hook); err != nil {
		t.Fatalf("installHooks() error = %v", err)
	}
	if file := readHooks(t, worktree); file.Hooks["agentStop"][0].Bash != "'/usr/local/bin/babysitter' 'watch' 'hook' '--watch' '7' 'stop'" {
		t.Fatalf("agentStop = %+v", file.Hooks["agentStop"])
	}
	exclude := filepath.Join(main, ".git", "info", "exclude")
	if got, err := os.ReadFile(exclude); err != nil || string(got) != "/.github/hooks/babysitter.json\n" {
		t.Fatalf("exclude = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(main, ".git", "worktrees", "linked", "info", "exclude")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the exclude went in the git dir of the worktree, which git does not read: %v", err)
	}
}

func TestInstallHooksFollowsNoLinkOutOfTheWorktree(t *testing.T) {
	t.Parallel()
	cases := []string{".github", filepath.Join(".github", "hooks"), filepath.Join(".github", "hooks", "babysitter.json")}
	for _, link := range cases {
		t.Run(link, func(t *testing.T) {
			t.Parallel()
			worktree, _ := linkedWorktree(t)
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.MkdirAll(outside, 0o750); err != nil {
				t.Fatal(err)
			}
			target := outside
			if filepath.Base(link) == "babysitter.json" {
				target = filepath.Join(outside, "config.json")
				if err := os.WriteFile(target, []byte("mine\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(worktree, ".github", "hooks"), 0o750); err != nil {
					t.Fatal(err)
				}
			}
			at := filepath.Join(worktree, link)
			if err := os.MkdirAll(filepath.Dir(at), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, at); err != nil {
				t.Skipf("this system makes no symbolic links: %v", err)
			}
			if err := installHooks(worktree, hook); err == nil {
				t.Fatalf("installHooks() wrote through the link at %s", link)
			}
			if data, err := os.ReadFile(filepath.Join(outside, "config.json")); err == nil && string(data) != "mine\n" {
				t.Fatalf("the file outside the worktree = %q", data)
			}
			if _, err := os.Stat(filepath.Join(outside, "babysitter.json")); err == nil {
				t.Fatal("the hooks went outside the worktree")
			}
		})
	}
}

func TestInstallHooksRewritesTheFile(t *testing.T) {
	t.Parallel()
	worktree, main := linkedWorktree(t)
	if err := installHooks(worktree, hook); err != nil {
		t.Fatalf("installHooks() error = %v", err)
	}
	exclude := filepath.Join(main, ".git", "info", "exclude")
	if err := os.WriteFile(exclude, []byte("/build\n/.github/hooks/babysitter.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installHooks(worktree, []string{"/opt/babysitter", "watch", "hook", "--watch", "7"}); err != nil {
		t.Fatalf("second installHooks() error = %v", err)
	}
	stop := readHooks(t, worktree).Hooks["agentStop"]
	if len(stop) != 1 || stop[0].Bash != "'/opt/babysitter' 'watch' 'hook' '--watch' '7' 'stop'" {
		t.Fatalf("a second install kept the old command: %+v", stop)
	}
	if got, _ := os.ReadFile(exclude); string(got) != "/build\n/.github/hooks/babysitter.json\n" {
		t.Fatalf("a second install changed the exclude file: %q", got)
	}
}

func TestInstallHooksWithoutACommandRemovesTheFile(t *testing.T) {
	t.Parallel()
	worktree, _ := linkedWorktree(t)
	if err := installHooks(worktree, hook); err != nil {
		t.Fatalf("installHooks() error = %v", err)
	}
	if err := installHooks(worktree, nil); err != nil {
		t.Fatalf("installHooks() without a command error = %v", err)
	}
	path := filepath.Join(worktree, ".github", "hooks", "babysitter.json")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the hooks file stayed: %v", err)
	}
	if err := installHooks(worktree, nil); err != nil {
		t.Fatalf("installHooks() without a file error = %v", err)
	}
}

func TestInstallHooksOutsideGit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := installHooks(dir, hook); err != nil {
		t.Fatalf("installHooks() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github", "hooks", "babysitter.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := installHooks(dir, hook); err != nil {
		t.Fatalf("installHooks() in a checkout error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude")); err != nil || string(got) != "/.github/hooks/babysitter.json\n" {
		t.Fatalf("exclude = %q, %v", got, err)
	}
}

func TestExcludeFromGit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exclude := filepath.Join(dir, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exclude, []byte("# comment\n/build"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := excludeFromGit(dir, "/x"); err != nil {
		t.Fatalf("excludeFromGit() error = %v", err)
	}
	if got, _ := os.ReadFile(exclude); string(got) != "# comment\n/build\n/x\n" {
		t.Fatalf("exclude = %q", got)
	}
	if err := excludeFromGit(dir, "/x"); err != nil {
		t.Fatalf("second excludeFromGit() error = %v", err)
	}
	if got, _ := os.ReadFile(exclude); string(got) != "# comment\n/build\n/x\n" {
		t.Fatalf("a pattern was written twice: %q", got)
	}
	if err := excludeFromGit(dir, "/xy"); err != nil {
		t.Fatalf("excludeFromGit() of a longer pattern error = %v", err)
	}
	if got, _ := os.ReadFile(exclude); !strings.HasSuffix(string(got), "/x\n/xy\n") {
		t.Fatalf("a pattern with a common prefix was taken for a duplicate: %q", got)
	}
}

func TestGitCommonDir(t *testing.T) {
	t.Parallel()
	if got, err := gitCommonDir(t.TempDir()); err != nil || got != "" {
		t.Fatalf("outside git = %q, %v", got, err)
	}

	checkout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(checkout, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if got, err := gitCommonDir(checkout); err != nil || got != filepath.Join(checkout, ".git") {
		t.Fatalf("checkout = %q, %v", got, err)
	}

	linked, main := linkedWorktree(t)
	if got, err := gitCommonDir(linked); err != nil || got != filepath.Join(main, ".git") {
		t.Fatalf("linked worktree = %q, %v", got, err)
	}

	relative := t.TempDir()
	gitDir := filepath.Join(relative, "repo", ".git", "worktrees", "wt")
	if err := os.MkdirAll(gitDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte(filepath.Join(relative, "repo", ".git")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(relative, "wt")
	if err := os.MkdirAll(wt, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: ../repo/.git/worktrees/wt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := gitCommonDir(wt); err != nil || got != filepath.Join(relative, "repo", ".git") {
		t.Fatalf("relative gitdir with an absolute commondir = %q, %v", got, err)
	}

	alone := t.TempDir()
	if err := os.WriteFile(filepath.Join(alone, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(gitDir, "commondir")); err != nil {
		t.Fatal(err)
	}
	if got, err := gitCommonDir(alone); err != nil || got != gitDir {
		t.Fatalf("gitdir without a commondir = %q, %v", got, err)
	}

	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, ".git"), []byte("not a pointer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := gitCommonDir(broken); err == nil {
		t.Fatal("a .git file without a gitdir line resolved")
	}
	if err := installHooks(broken, hook); err == nil {
		t.Fatal("installHooks() on a broken .git file succeeded")
	}
}
