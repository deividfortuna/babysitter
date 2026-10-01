package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent"
)

func launch(t *testing.T) agent.Launch {
	t.Helper()
	return agent.Launch{
		WorktreeDir: t.TempDir(), Model: "", SessionID: "0d5f4b5e-6f1e-4f8e-9c9a-2d4b7e1c3a10", Name: "PR #3",
		Hook: []string{"/usr/local/bin/babysitter", "watch", "hook", "--watch", "7"}, HooksDir: filepath.Join(t.TempDir(), "hooks"),
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
	c := New("", "")
	c.ConfigPath = filepath.Join(t.TempDir(), ".claude.json")
	argv, _, err := c.Command(launch(t))
	if err != nil {
		t.Fatal(err)
	}
	denied, _ := flag(argv, "--disallowedTools")
	rules := strings.Split(denied, ",")
	for _, sub := range []string{"mode", "approve", "reject", "retry", "merge", "stop"} {
		for _, exe := range []string{"babysitter", "/usr/local/bin/babysitter"} {
			for _, rule := range []string{"Bash(" + exe + " watch " + sub + ":*)", "Bash(" + exe + " watch " + sub + " *)"} {
				if !slices.Contains(rules, rule) {
					t.Errorf("denied tools lack %s", rule)
				}
			}
		}
	}
	if strings.Contains(denied, "watch reply") {
		t.Fatalf("the reply of the agent is refused: %q", denied)
	}
}

func TestTheAgentCannotDecideForTheAuthorWithAnotherSpelling(t *testing.T) {
	t.Parallel()
	c := New("", "")
	c.ConfigPath = filepath.Join(t.TempDir(), ".claude.json")
	argv, _, err := c.Command(launch(t))
	if err != nil {
		t.Fatal(err)
	}
	denied, _ := flag(argv, "--disallowedTools")
	rules := strings.Split(denied, ",")
	refused := func(line string) bool {
		return slices.ContainsFunc(rules, func(rule string) bool { return bashRuleMatches(rule, line) })
	}
	if !refused("babysitter watch mode 1 auto") {
		t.Fatal("the rules do not refuse the spelling the live check saw refused")
	}
	for _, line := range []string{
		"babysitter -o text watch mode 1 auto",
		"babysitter --data-dir /tmp/x watch mode 1 auto",
		"babysitter -o json watch merge 1",
		"/usr/local/bin/babysitter --output json watch approve 1",
		"sh -c 'babysitter watch mode 1 auto'",
		"babysitter -o json watch stop",
	} {
		if !refused(line) {
			t.Errorf("no denied tool refuses %q", line)
		}
	}
}

func TestNoDeniedToolMixesAWildcardWithThePrefixSyntax(t *testing.T) {
	t.Parallel()
	c := New("", "")
	c.ConfigPath = filepath.Join(t.TempDir(), ".claude.json")
	argv, _, err := c.Command(launch(t))
	if err != nil {
		t.Fatal(err)
	}
	denied, _ := flag(argv, "--disallowedTools")
	for rule := range strings.SplitSeq(denied, ",") {
		if pattern, ok := strings.CutSuffix(rule, ":*)"); ok && strings.Contains(pattern, "*") {
			t.Errorf("claude matches %s as a literal prefix, so it refuses nothing", rule)
		}
	}
}

func bashRuleMatches(rule, command string) bool {
	pattern, ok := strings.CutPrefix(rule, "Bash(")
	if !ok {
		return false
	}
	pattern = strings.TrimSuffix(pattern, ")")
	if prefix, ok := strings.CutSuffix(pattern, ":*"); ok {
		return strings.HasPrefix(command, prefix)
	}
	glob := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
	return regexp.MustCompile(glob).MatchString(command)
}

func TestCommandWithoutScreenReader(t *testing.T) {
	t.Parallel()
	c := New("", "")
	c.ConfigPath = filepath.Join(t.TempDir(), ".claude.json")
	l := launch(t)
	l.ScreenReader = false
	argv, _, err := c.Command(l)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if slices.Contains(argv, "--ax-screen-reader") {
		t.Fatalf("the screen reader mode is on when the setting is off: %v", argv)
	}
}

func TestCommand(t *testing.T) {
	t.Parallel()
	c := New("", "sonnet")
	c.ConfigPath = filepath.Join(t.TempDir(), ".claude.json")
	l := launch(t)
	argv, env, err := c.Command(l)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if argv[0] != "claude" {
		t.Fatalf("argv = %v", argv)
	}
	if v, ok := flag(argv, "--session-id"); !ok || v != l.SessionID {
		t.Fatalf("session id flag = %q, %v in %v", v, ok, argv)
	}
	if _, ok := flag(argv, "--resume"); ok {
		t.Fatalf("a new session resumes: %v", argv)
	}
	if v, _ := flag(argv, "--permission-mode"); v != "dontAsk" {
		t.Fatalf("permission mode = %q", v)
	}
	if v, _ := flag(argv, "--setting-sources"); v != "user" {
		t.Fatalf("setting sources = %q, the worktree holds the branch under review", v)
	}
	if !slices.Contains(argv, "--ax-screen-reader") {
		t.Fatalf("the output is drawn for a terminal nobody reads: %v", argv)
	}
	if v, _ := flag(argv, "--model"); v != "sonnet" {
		t.Fatalf("model = %q", v)
	}
	if v, _ := flag(argv, "--name"); v != "PR #3" {
		t.Fatalf("name = %q", v)
	}
	if v, _ := flag(argv, "--append-system-prompt"); !strings.Contains(v, "Standing rules") {
		t.Fatalf("system prompt = %q", v)
	}
	denied, _ := flag(argv, "--disallowedTools")
	for _, rule := range []string{
		"Bash(git push --force *)", "Bash(gh pr merge:*)",
		"Bash(gh api * -f *)", "Bash(gh api * -F *)", "Bash(gh api * --field *)", "Bash(gh api * --raw-field *)", "Bash(gh api * --input *)",
	} {
		if !strings.Contains(denied, rule) {
			t.Errorf("denied tools lack %s: %q", rule, denied)
		}
	}
	if v, _ := flag(argv, "--settings"); v != hookSettings(l.Hook) {
		t.Fatalf("settings = %q", v)
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "GIT_CONFIG_KEY_0=core.hooksPath") || !strings.Contains(joined, "GIT_CONFIG_VALUE_0="+l.HooksDir) || strings.Contains(joined, "BABYSITTER_PUSH_REF") {
		t.Fatalf("env = %v", env)
	}
	hook, err := os.ReadFile(filepath.Join(l.HooksDir, "pre-push"))
	if err != nil || !strings.Contains(string(hook), "the daemon pushes this branch") {
		t.Fatalf("pre-push hook = %q, %v", hook, err)
	}
	data, err := os.ReadFile(c.ConfigPath)
	// Claude Code keys the project with forward slashes, on Windows too.
	project, _ := json.Marshal(filepath.ToSlash(l.WorktreeDir))
	if err != nil || !strings.Contains(string(data), `"hasTrustDialogAccepted": true`) || !strings.Contains(string(data), string(project)+": {") {
		t.Fatalf("trust config = %s, %v", data, err)
	}
	if !c.Signals() || c.Prelude() != "" {
		t.Fatal("claude has hooks and takes the rules in the system prompt")
	}

	l.Resume = true
	l.Model = "opus"
	l.Effort = "xhigh"
	argv, _, err = c.Command(l)
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if v, _ := flag(argv, "--effort"); v != "xhigh" {
		t.Fatalf("effort of the launch = %q in %v", v, argv)
	}
	if v, _ := flag(argv, "--resume"); v != l.SessionID {
		t.Fatalf("resume flag = %q in %v", v, argv)
	}
	if _, ok := flag(argv, "--session-id"); ok {
		t.Fatalf("a resumed session takes a new id: %v", argv)
	}
	if v, _ := flag(argv, "--model"); v != "opus" {
		t.Fatalf("model of the launch = %q", v)
	}
	if _, _, err := c.Command(agent.Launch{}); err == nil {
		t.Fatal("Command() without a worktree succeeded")
	}
}

func TestAcceptTrustKeepsTheRestOfTheConfig(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","projects":{"/other":{"hasTrustDialogAccepted":true,"allowedTools":["Bash"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := acceptTrust(path, "/work/tree"); err != nil {
		t.Fatalf("acceptTrust() error = %v", err)
	}
	data, _ := os.ReadFile(path)
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("config %s: %v", data, err)
	}
	projects := config["projects"].(map[string]any)
	if config["theme"] != "dark" || len(projects) != 2 || projects["/work/tree"].(map[string]any)["hasTrustDialogAccepted"] != true {
		t.Fatalf("config = %s", data)
	}
	if other := projects["/other"].(map[string]any); len(other["allowedTools"].([]any)) != 1 {
		t.Fatalf("other project = %v", other)
	}
	before, _ := os.Stat(path)
	if err := acceptTrust(path, "/work/tree"); err != nil {
		t.Fatalf("second acceptTrust() error = %v", err)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("a trusted directory rewrote the config")
	}
}

func TestAcceptTrustKeepsEveryDirectoryOfConcurrentStarts(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".claude.json")
	dirs := []string{"/work/one", "/work/two", "/work/three", "/work/four", "/work/five"}
	var wg sync.WaitGroup
	for _, dir := range dirs {
		wg.Go(func() {
			if err := acceptTrust(path, dir); err != nil {
				t.Errorf("acceptTrust(%s) error = %v", dir, err)
			}
		})
	}
	wg.Wait()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Projects map[string]struct {
			Accepted bool `json:"hasTrustDialogAccepted"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("config %s: %v", data, err)
	}
	for _, dir := range dirs {
		if !config.Projects[dir].Accepted {
			t.Fatalf("%s lost its trust: %s", dir, data)
		}
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
