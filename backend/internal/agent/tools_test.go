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
	for _, want := range []string{"babysitter auth login", "babysitter auth logout"} {
		if !slices.Contains(got, want) {
			t.Errorf("AuthorCommands() lacks %q: %q", want, got)
		}
	}
	if bare := AuthorCommands(Launch{}); len(bare) != len(authorDecisions) {
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
	if !slices.Contains(got, "*babysitter* auth logout") {
		t.Errorf("AuthorPatterns() lacks the sign out of the app: %q", got)
	}
	if slices.ContainsFunc(got, func(c string) bool { return strings.Contains(c, "reply") }) {
		t.Fatalf("the reply of the agent is refused: %q", got)
	}
}

func TestThePrePushHookRefusesEveryPush(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not installed")
	}
	dir := filepath.Join(t.TempDir(), "hooks")
	env, err := SessionEnv(Launch{HooksDir: dir})
	if err != nil {
		t.Fatalf("SessionEnv() error = %v", err)
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
	env, err := SessionEnv(Launch{HooksDir: filepath.Join(t.TempDir(), "hooks")})
	if err != nil {
		t.Fatalf("SessionEnv() error = %v", err)
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

func TestATurnEndsWhenTheAgentStopsOrIsGone(t *testing.T) {
	t.Parallel()
	for _, s := range States {
		want := s == StateIdle || s == StateWaiting || s == StateExited
		if s.EndsTurn() != want {
			t.Errorf("%s.EndsTurn() = %v, want %v", s, !want, want)
		}
	}
}
