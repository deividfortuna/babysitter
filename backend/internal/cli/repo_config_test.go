package cli

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func gitCheckoutOf(t *testing.T, origin string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", origin}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestRepoConfigChangesAndShowsTheConfiguration(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.Repo("acme/billing")
	db := filepath.Join(t.TempDir(), "babysitter.db")
	run := func(args ...string) (string, error) {
		t.Helper()
		return runCLI(t, g, db, args...)
	}
	if _, err := run("repo", "add", "acme/billing"); err != nil {
		t.Fatal(err)
	}

	out, err := run("repo", "config", "acme/billing")
	if err != nil || !strings.Contains(out, "My pull requests:  off") || !strings.Contains(out, "Merge scope:       patch") ||
		!strings.Contains(out, "Approval:          never") || !strings.Contains(out, "Watch defaults:    the settings of the daemon") {
		t.Fatalf("repo config = %q, %v", out, err)
	}
	if _, err := run("repo", "config", "acme/billing", "--auto-start-mine"); err == nil || !strings.Contains(err.Error(), "set the checkout") {
		t.Fatalf("a toggle without a checkout = %v", err)
	}
	web := gitCheckoutOf(t, "https://github.com/acme/web.git")
	if _, err := run("repo", "config", "acme/billing", "--checkout", web); err == nil || !strings.Contains(err.Error(), "has no remote for acme/billing") {
		t.Fatalf("a checkout of another repository = %v", err)
	}

	dir := gitCheckoutOf(t, "git@github.com:acme/billing.git")
	out, err = run("repo", "config", "acme/billing", "-o", "json", "--checkout", dir, "--auto-start-mine", "--auto-watch-dependabot",
		"--provider", "copilot", "--approvals", "branch", "--merge-method", "squash",
		"--keep-worktree", "--include-own=false",
		"--dependabot-scope", "minor", "--dependabot-approval", "green", "--dependabot-limit", "2")
	if err != nil {
		t.Fatal(err)
	}
	var cfg repoConfigOutput
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if cfg.CheckoutDir != dir || !cfg.AutoStartMine || !cfg.AutoWatchDependabot || cfg.Overrides.Provider != "copilot" ||
		cfg.Overrides.ApprovalsRequired != "branch" || cfg.Overrides.MergeMethod != "squash" ||
		!*cfg.Overrides.KeepWorktree || *cfg.Overrides.IncludeOwn || cfg.Overrides.AutoApproveRebase != nil ||
		cfg.DependabotScope != "minor" || cfg.DependabotApproval != "green" || cfg.DependabotLimit != 2 {
		t.Fatalf("config = %+v", cfg)
	}

	out, err = run("repo", "config", "acme/billing", "--approvals", "default", "--auto-start-mine=false")
	if err != nil || !strings.Contains(out, "My pull requests:  off") || !strings.Contains(out, "Dependabot:        on since") ||
		!strings.Contains(out, "provider copilot, merge method squash, include own false, keep worktree true\n") {
		t.Fatalf("repo config after a change = %q, %v", out, err)
	}
	out, err = run("repo", "list")
	if err != nil || !strings.Contains(out, "AUTO START") || !strings.Contains(out, "dependabot") {
		t.Fatalf("repo list = %q, %v", out, err)
	}
}

func TestRepoConfigProviderGivesTheNewProviderItsDefaultModel(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.Repo("acme/billing")
	db := filepath.Join(t.TempDir(), "babysitter.db")
	run := func(args ...string) (repoConfigOutput, error) {
		t.Helper()
		out, err := runCLI(t, g, db, append(args, "-o", "json")...)
		if err != nil {
			return repoConfigOutput{}, err
		}
		var cfg repoConfigOutput
		if err := json.Unmarshal([]byte(out), &cfg); err != nil {
			t.Fatalf("invalid JSON %q: %v", out, err)
		}
		return cfg, nil
	}
	if _, err := runCLI(t, g, db, "repo", "add", "acme/billing"); err != nil {
		t.Fatal(err)
	}
	if _, err := run("repo", "config", "acme/billing", "--provider", "claude", "--model", "opus"); err != nil {
		t.Fatal(err)
	}

	cfg, err := run("repo", "config", "acme/billing", "--provider", "copilot")
	if err != nil || cfg.Overrides.Provider != "copilot" || cfg.Overrides.Model != "" {
		t.Fatalf("repo config --provider copilot = %+v, %v; want copilot on its default model", cfg.Overrides, err)
	}
	cfg, err = run("repo", "config", "acme/billing", "--provider", "claude", "--model", "sonnet")
	if err != nil || cfg.Overrides.Provider != "claude" || cfg.Overrides.Model != "sonnet" {
		t.Fatalf("repo config --provider claude --model sonnet = %+v, %v", cfg.Overrides, err)
	}
}

func TestRepoConfigHelpNamesTheOverridesAsFlagsOfTheCommand(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "babysitter.db")

	out, err := runCLI(t, ghfake.New(), db, "repo", "config", "--help")
	if err != nil {
		t.Fatalf("repo config --help error = %v", err)
	}
	if strings.Contains(out, "watch start flags") || !strings.Contains(out, "The override flags") {
		t.Fatalf("help = %q, want the overrides named as flags of repo config, not of watch start", out)
	}
}

func TestRepoQueueListsTheWaitingUpdatesOldestFirst(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.Repo("acme/billing")
	later := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	for n, title := range map[int]string{
		10: "Bump lib from 1.2.0 to 1.3.0",
		11: "Bump react from 18.3.1 to 19.0.0",
		12: "Bump lodash from 4.17.20 to 4.17.21",
	} {
		pr := g.PR("acme/billing", n)
		pr.Author, pr.Title = "dependabot[bot]", title
		pr.CreatedAt = later.Add(time.Duration(n) * time.Minute)
	}
	db := filepath.Join(t.TempDir(), "babysitter.db")
	dir := gitCheckoutOf(t, "git@github.com:acme/billing.git")
	for _, args := range [][]string{
		{"repo", "add", "acme/billing"},
		{"repo", "config", "acme/billing", "--checkout", dir, "--auto-watch-dependabot"},
		{"sync"},
	} {
		if _, err := runCLI(t, g, db, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	out, err := runCLI(t, g, db, "repo", "queue", "acme/billing")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[1], "1  10  minor") || !strings.HasPrefix(lines[2], "2  11  major") || !strings.HasPrefix(lines[3], "3  12  patch") {
		t.Fatalf("repo queue = %q", out)
	}
	if _, err := runCLI(t, g, db, "repo", "config", "acme/billing", "--auto-watch-dependabot=false"); err != nil {
		t.Fatal(err)
	}
	if out, err := runCLI(t, g, db, "repo", "queue", "acme/billing"); err != nil || out != "No Dependabot pull request waits.\n" {
		t.Fatalf("repo queue after the toggle went off = %q, %v", out, err)
	}
}

func TestServeStartsNoWatch(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.Repo("acme/billing")
	pr := g.PR("acme/billing", 4)
	pr.CreatedAt = time.Now().Add(time.Hour).UTC()
	db := filepath.Join(t.TempDir(), "babysitter.db")
	dir := gitCheckoutOf(t, "git@github.com:acme/billing.git")
	for _, args := range [][]string{
		{"repo", "add", "acme/billing"},
		{"repo", "config", "acme/billing", "--checkout", dir, "--auto-start-mine"},
	} {
		if _, err := runCLI(t, g, db, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	srv := g.Serve(t)
	ctx, cancel := context.WithCancel(context.Background())
	root := NewRootCmd(WithClientFactory(func(string, time.Duration) (*github.Client, error) { return srv.NewClient() }))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--token", "x", "--db", db, "serve", "--interval", "10s"})
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()

	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	testutil.Eventually(t, func() bool {
		repo, err := st.GetRepo(context.Background(), "acme", "billing")
		return err == nil && repo.LastSyncedAt != nil
	}, "serve syncs acme/billing")
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve = %v", err)
	}
	watches, err := st.ListWatches(context.Background(), store.ListWatchesOptions{})
	if err != nil || len(watches) != 0 {
		t.Fatalf("watches = %d, %v, want none", len(watches), err)
	}
	repo, _ := st.GetRepo(context.Background(), "acme", "billing")
	if claimed, err := st.AutoStartClaimed(context.Background(), repo.ID, 4, time.Time{}); err != nil || claimed {
		t.Fatalf("claimed = %v, %v, want no claim", claimed, err)
	}
}
