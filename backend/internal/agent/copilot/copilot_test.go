package copilot

import (
	"context"
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

func TestTheAgentCannotDecideForTheAuthorWithAnotherSpelling(t *testing.T) {
	t.Parallel()
	argv, _, err := New("", "").Command(launch(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"mode", "approve", "reject", "retry", "merge", "stop"} {
		if rule := "shell(*babysitter* watch " + sub + ":*)"; !slices.Contains(argv, rule) {
			t.Errorf("argv lacks the rule %s", rule)
		}
	}
}

func TestCommandPassesTheReasoningEffortOfTheWatch(t *testing.T) {
	t.Parallel()
	c := New("", "")
	l := launch(t)
	l.WorktreeDir, _ = linkedWorktree(t)
	l.Effort = "xhigh"
	argv, _, err := c.Command(l)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if v, _ := flag(argv, "--reasoning-effort"); v != "xhigh" {
		t.Fatalf("reasoning effort = %q in %v", v, argv)
	}
}

func TestCommand(t *testing.T) {
	t.Parallel()
	c := New("", "")
	l := launch(t)
	l.WorktreeDir, _ = linkedWorktree(t)
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
	l.WorktreeDir, _ = linkedWorktree(t)
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
	l.WorktreeDir, _ = linkedWorktree(t)
	argv, _, err := c.Command(l)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if v, _ := flag(argv, "--name"); v != l.Name {
		t.Fatalf("name = %q, want %q", v, l.Name)
	}
}
