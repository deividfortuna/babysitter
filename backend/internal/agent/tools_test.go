package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAuthorCommandsNameEveryWayTheAgentCallsBabysitter(t *testing.T) {
	t.Parallel()
	got := AuthorCommands(Launch{Hook: []string{"/opt/my tools/babysitter", "watch", "hook", "--watch", "7"}})
	for _, sub := range []string{"mode", "approve", "reject", "retry", "merge", "stop", "takeover", "handback"} {
		for _, exe := range []string{"babysitter", "/opt/my tools/babysitter", "'/opt/my tools/babysitter'"} {
			if want := exe + " watch " + sub; !slices.Contains(got, want) {
				t.Errorf("AuthorCommands() lacks %q: %q", want, got)
			}
		}
	}
	if slices.ContainsFunc(got, func(c string) bool { return strings.Contains(c, "reply") }) {
		t.Fatalf("the reply of the agent is refused: %q", got)
	}
	if bare := AuthorCommands(Launch{}); len(bare) != 8 {
		t.Fatalf("AuthorCommands() without a hook = %q, want the bare word only", bare)
	}
}

func TestAuthorPatternsTakeEverySpellingOfADecision(t *testing.T) {
	t.Parallel()
	got := AuthorPatterns()
	for _, sub := range []string{"mode", "approve", "reject", "retry", "merge", "stop", "takeover", "handback"} {
		if want := "*babysitter* watch " + sub; !slices.Contains(got, want) {
			t.Errorf("AuthorPatterns() lacks %q: %q", want, got)
		}
	}
	if slices.ContainsFunc(got, func(c string) bool { return strings.Contains(c, "reply") }) {
		t.Fatalf("the reply of the agent is refused: %q", got)
	}
}

func TestIsAuthorDecisionTakesEverySpellingOfADecision(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		"babysitter watch mode 1 auto",
		"babysitter -o json watch reject 1 --reason x",
		"babysitter --data-dir /tmp/x watch stop",
		"/usr/local/bin/babysitter --output json watch approve 1",
		"'/opt/my tools/babysitter' watch merge 1",
		"sh -c 'babysitter -o text watch retry 1'",
		"cd /repo && babysitter watch takeover 1",
		"babysitter watch\thandback 1",
		"babysitter \\\n  watch merge 1",
		"baby''sitter watch reject 1",
		`baby""sitter watch approve 1`,
		`babysitter watch "reject" 1`,
		`babysitter wat'ch' mer""ge 1`,
		`baby\sitter watch st\op 1`,
		"babysitter watch reply 7 done && babysitter watch merge 7",
		`babysitter watch reply 7 "$(babysitter watch merge 7)"`,
		"babysitter watch reply 7 `babysitter watch merge 7`",
		`babysitter -o "a watch reply" watch merge 1`,
		"babysitter --data-dir /tmp/watch watch merge 1",
		"babysitter watch --reason x reject 1",
		"echo $(babysitter watch merge 1)",
		`bash -lc "babysitter watch merge 1"`,
		"env BABYSITTER_WATCH=7 babysitter watch merge 1",
	} {
		if !IsAuthorDecision(command) {
			t.Errorf("IsAuthorDecision(%q) = false", command)
		}
	}
	for _, command := range []string{
		"babysitter watch reply 1 fixed in the last commit",
		`babysitter watch reply 7 "please watch merge when ready"`,
		"babysitter -o json watch reply 7 'I did not run babysitter watch merge'",
		`babysitter watch reply --to 42 7 "the author runs watch approve"`,
		"babysitter -o json watch status 1",
		"babysitter watch modes",
		"go test ./internal/prwatch/ -run TestWatchMerge",
		"git commit -m 'watch merge readiness'",
		"",
	} {
		if IsAuthorDecision(command) {
			t.Errorf("IsAuthorDecision(%q) = true", command)
		}
	}
}

func TestRefusesToolUseReadsTheCommandOfEveryAgent(t *testing.T) {
	t.Parallel()
	const decision = "babysitter -o json watch reject 1 --reason x"
	refused := []map[string]any{
		{"toolName": "bash", "toolArgs": map[string]any{"command": decision}},
		{"toolName": "bash", "toolArgs": `{"command":"` + decision + `"}`},
		{"tool_name": "Bash", "tool_input": map[string]any{"command": decision}},
	}
	for _, payload := range refused {
		if !RefusesToolUse(EventPreToolUse, payload) {
			t.Errorf("RefusesToolUse(%v) = false", payload)
		}
		if RefusesToolUse(EventPostToolUse, payload) {
			t.Errorf("RefusesToolUse() refuses a tool that already ran: %v", payload)
		}
	}
	allowed := []map[string]any{
		{"toolName": "bash", "toolArgs": map[string]any{"command": "babysitter watch reply 1 done"}},
		{"tool_name": "Read", "tool_input": map[string]any{"file_path": decision}},
		{"toolName": "bash", "toolArgs": "not json"},
		{},
		nil,
	}
	for _, payload := range allowed {
		if RefusesToolUse(EventPreToolUse, payload) {
			t.Errorf("RefusesToolUse(%v) = true", payload)
		}
	}
}

func TestThePrePushHookRefusesEveryPush(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not installed")
	}
	dir := filepath.Join(t.TempDir(), "hooks")
	env, err := GitEnv(Launch{HooksDir: dir})
	if err != nil {
		t.Fatalf("GitEnv() error = %v", err)
	}
	for _, e := range env {
		if strings.HasPrefix(e, "BABYSITTER_PUSH_REF") {
			t.Fatalf("the env names a branch to push: %v", env)
		}
	}
	for _, ref := range []string{"refs/heads/fix", "refs/heads/main"} {
		cmd := exec.Command("sh", filepath.Join(dir, "pre-push"), "origin", "git@github.com:octo/hello.git")
		cmd.Stdin = strings.NewReader("refs/heads/babysitter/fix abc " + ref + " def\n")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("the hook let a push to %s through", ref)
		}
		if !strings.Contains(string(out), "the daemon pushes this branch") || !strings.Contains(string(out), "let the turn end") {
			t.Fatalf("the hook says %q", out)
		}
	}
}

func TestTheCommitMsgHookMakesBabysitterACoAuthorOnce(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	env, err := GitEnv(Launch{HooksDir: filepath.Join(t.TempDir(), "hooks")})
	if err != nil {
		t.Fatalf("GitEnv() error = %v", err)
	}
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=Octo", "-c", "user.email=octo@example.com"}, args...)...)
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("init", "--quiet")
	git("commit", "--quiet", "--allow-empty", "-m", "Fix the parser", "-m", "Signed-off-by: Octo <octo@example.com>")
	git("commit", "--quiet", "--amend", "--allow-empty", "--no-edit")

	message := git("log", "-1", "--format=%B")
	if got := strings.Count(message, CoAuthorTrailer); got != 1 {
		t.Fatalf("the message has %d co-author trailers, want 1:\n%s", got, message)
	}
	trailers := git("log", "-1", "--format=%(trailers:only,unfold)")
	if !strings.Contains(trailers, "Signed-off-by: Octo") || !strings.Contains(trailers, CoAuthorTrailer) {
		t.Fatalf("trailers = %q", trailers)
	}
}

func TestATurnEndsWhenTheAgentIsIdleOrGone(t *testing.T) {
	t.Parallel()
	for _, s := range States {
		want := s == StateIdle || s == StateExited
		if s.EndsTurn() != want {
			t.Errorf("%s.EndsTurn() = %v, want %v", s, !want, want)
		}
	}
}
