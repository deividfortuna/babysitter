package copilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent"
)

func launch(t *testing.T) agent.Launch {
	t.Helper()
	return agent.Launch{
		WorktreeDir: t.TempDir(), Model: "", SessionID: "0d5f4b5e-6f1e-4f8e-9c9a-2d4b7e1c3a10", Name: "PR #3",
		Hook: []string{"/usr/local/bin/babysitter", "watch", "hook", "--watch", "7"}, HooksDir: filepath.Join(t.TempDir(), "hooks"),
		PluginDir:    filepath.Join(t.TempDir(), "agent-plugins", "7"),
		ScreenReader: true,
	}
}

func flag(argv []string, name string) (string, bool) {
	for i, a := range argv {
		if a == name && i+1 < len(argv) {
			return argv[i+1], true
		}
	}
	return "", false
}

func TestTheAgentCannotDecideForTheAuthor(t *testing.T) {
	t.Parallel()
	argv, _, err := New("", "").Command(launch(t))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(argv, " ")
	for _, sub := range []string{"mode", "approve", "reject", "retry", "merge", "stop"} {
		for _, exe := range []string{"babysitter", "/usr/local/bin/babysitter"} {
			if rule := "shell(" + exe + " watch " + sub + ":*)"; !slices.Contains(argv, rule) {
				t.Errorf("argv lacks the rule %s", rule)
			}
		}
	}
	if strings.Contains(joined, "watch reply") {
		t.Fatalf("the reply of the agent is refused: %v", argv)
	}
}

func TestTheHooksRefuseADecisionOfTheAuthorWithAnotherSpelling(t *testing.T) {
	t.Parallel()
	l := launch(t)
	argv, _, err := New("", "").Command(l)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := flag(argv, "--plugin-dir"); v != l.PluginDir {
		t.Fatalf("plugin dir = %q in %v", v, argv)
	}
	if got := readHooks(t, l.PluginDir).Hooks["preToolUse"]; len(got) != 1 || !strings.HasSuffix(got[0].Bash, "'"+agent.EventPreToolUse+"'") {
		t.Fatalf("preToolUse = %+v, want the hook that refuses the decisions of the author", got)
	}
	if slices.ContainsFunc(argv, func(a string) bool { return strings.HasPrefix(a, "shell(*") }) {
		t.Fatalf("argv has a rule that copilot matches as a literal prefix: %v", argv)
	}
}

func TestTheHooksStayOutOfTheWorktree(t *testing.T) {
	t.Parallel()
	l := launch(t)
	path := filepath.Join(l.WorktreeDir, filepath.FromSlash(worktreeHooksFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := New("", "").Command(l); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the agent can edit the hooks in the worktree: %v", err)
	}
}

func TestCommandWithoutAHookLoadsNoPlugin(t *testing.T) {
	t.Parallel()
	l := launch(t)
	l.Hook = nil
	argv, _, err := New("", "").Command(l)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(argv, "--plugin-dir") {
		t.Fatalf("a session without a hook loads the plugin: %v", argv)
	}
	l = launch(t)
	l.PluginDir = ""
	if _, _, err := New("", "").Command(l); err == nil {
		t.Fatal("Command() with a hook and no plugin dir succeeded")
	}
}

func TestCommandPassesTheReasoningEffortOfTheWatch(t *testing.T) {
	t.Parallel()
	c := New("", "")
	l := launch(t)
	l.Effort = "xhigh"
	argv, _, err := c.Command(l)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if v, _ := flag(argv, "--reasoning-effort"); v != "xhigh" {
		t.Fatalf("reasoning effort = %q in %v", v, argv)
	}
}

func TestCommandWithoutScreenReader(t *testing.T) {
	t.Parallel()
	c := New("", "")
	l := launch(t)
	l.ScreenReader = false
	argv, _, err := c.Command(l)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if slices.Contains(argv, "--screen-reader") {
		t.Fatalf("the screen reader mode is on when the setting is off: %v", argv)
	}
}

func TestCommand(t *testing.T) {
	t.Parallel()
	c := New("", "")
	l := launch(t)
	argv, env, err := c.Command(l)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if argv[0] != "copilot" {
		t.Fatalf("argv = %v", argv)
	}
	if v, _ := flag(argv, "--session-id"); v != l.SessionID {
		t.Fatalf("session id = %q", v)
	}
	if _, ok := flag(argv, "--model"); ok {
		t.Fatalf("a model without a choice: %v", argv)
	}
	if _, ok := flag(argv, "--reasoning-effort"); ok {
		t.Fatalf("an effort without a choice: %v", argv)
	}
	if !slices.Contains(argv, "--screen-reader") {
		t.Fatalf("the output is drawn for a terminal nobody reads: %v", argv)
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "--allow-all-tools") || !strings.Contains(joined, "--deny-tool shell(gh pr merge:*)") {
		t.Fatalf("argv = %v", argv)
	}
	for _, rule := range []string{"shell(gh api * -f:*)", "shell(gh api * -F:*)", "shell(gh api * --field:*)", "shell(gh api * --raw-field:*)", "shell(gh api * --input:*)"} {
		if !strings.Contains(joined, "--deny-tool "+rule) {
			t.Errorf("argv lacks the rule %s: %v", rule, argv)
		}
	}
	if slices.Contains(argv, "--allow-all-paths") {
		t.Fatalf("the file tools reach outside the worktree: %v", argv)
	}
	if got := strings.Join(env, "\n"); strings.Contains(got, "BABYSITTER_PUSH_REF") || !strings.Contains(got, "COPILOT_ALLOW_ALL=true") {
		t.Fatalf("env = %v", env)
	}
	if !c.Signals() || c.Prelude() != agent.SystemPrompt() {
		t.Fatal("copilot has hooks and takes the rules in the first message")
	}
	if _, _, err := c.Command(agent.Launch{}); err == nil {
		t.Fatal("Command() without a worktree succeeded")
	}
}

func TestDoctor(t *testing.T) {
	t.Parallel()
	ok := func(context.Context, string, string, []string, string, ...string) (string, error) {
		return "1.0.0", nil
	}
	if err := (&Runner{Exec: ok}).Doctor(context.Background()); err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	silent := func(context.Context, string, string, []string, string, ...string) (string, error) { return "", nil }
	if err := (&Runner{Exec: silent}).Doctor(context.Background()); err == nil {
		t.Fatal("Doctor() of a silent command succeeded")
	}
}

func TestCommandOfAResumeCarriesNoName(t *testing.T) {
	t.Parallel()
	c := New("", "")
	l := launch(t)
	l.Resume = true
	argv, _, err := c.Command(l)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if v, _ := flag(argv, "--session-id"); v != l.SessionID {
		t.Fatalf("session id = %q", v)
	}
	if slices.Contains(argv, "--name") {
		t.Fatalf("a resume names the session, which Copilot CLI refuses: %v", argv)
	}
}

func TestCommandOfANewSessionNamesIt(t *testing.T) {
	t.Parallel()
	c := New("", "")
	l := launch(t)
	argv, _, err := c.Command(l)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if v, _ := flag(argv, "--name"); v != l.Name {
		t.Fatalf("name = %q, want %q", v, l.Name)
	}
}
