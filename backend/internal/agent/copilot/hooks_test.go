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

func readHooks(t *testing.T, dir string) hookFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "hooks.json"))
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

func TestInstallPluginWritesTheManifestAndTheHooks(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "agent-plugins", "7")
	if err := installPlugin(dir, hook); err != nil {
		t.Fatalf("installPlugin() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest pluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Name != pluginName || manifest.Description == "" {
		t.Fatalf("plugin.json = %s, %v", data, err)
	}
	if got := readHooks(t, dir).Hooks["preToolUse"]; len(got) != 1 || got[0].Bash != "'/usr/local/bin/babysitter' 'watch' 'hook' '--watch' '7' 'pre-tool-use'" {
		t.Fatalf("preToolUse = %+v", got)
	}

	if err := installPlugin(dir, []string{"/opt/babysitter", "watch", "hook", "--watch", "7"}); err != nil {
		t.Fatalf("second installPlugin() error = %v", err)
	}
	if got := readHooks(t, dir).Hooks["agentStop"]; len(got) != 1 || got[0].Bash != "'/opt/babysitter' 'watch' 'hook' '--watch' '7' 'stop'" {
		t.Fatalf("a second install kept the old command: %+v", got)
	}
}

func TestRemoveWorktreeHooks(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	path := filepath.Join(worktree, filepath.FromSlash(worktreeHooksFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeWorktreeHooks(worktree); err != nil {
		t.Fatalf("removeWorktreeHooks() error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the hooks stayed in the worktree: %v", err)
	}
	if err := removeWorktreeHooks(worktree); err != nil {
		t.Fatalf("removeWorktreeHooks() without a file error = %v", err)
	}
}

func TestRemoveWorktreeHooksFollowsNoLinkOutOfTheWorktree(t *testing.T) {
	t.Parallel()
	worktree, outside := t.TempDir(), t.TempDir()
	mine := filepath.Join(outside, "hooks", "babysitter.json")
	if err := os.MkdirAll(filepath.Dir(mine), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mine, []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(worktree, ".github")); err != nil {
		t.Skipf("this system makes no symbolic links: %v", err)
	}
	if err := removeWorktreeHooks(worktree); err == nil {
		t.Fatal("removeWorktreeHooks() went through the link")
	}
	if data, err := os.ReadFile(mine); err != nil || string(data) != "mine\n" {
		t.Fatalf("the file outside the worktree = %q, %v", data, err)
	}
}
