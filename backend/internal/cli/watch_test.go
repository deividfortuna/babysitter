package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/service"
)

// helloPR is pull request 3 of octo/hello as the watch tests see it: a
// draft with changes requested by carol, a failing check and a pending
// combined status.
func helloPR() (*ghfake.GitHub, *ghfake.PR) {
	g := ghfake.New()
	pr := g.PR("octo/hello", 3)
	pr.Draft = true
	pr.Mergeable, pr.MergeableState = nil, "blocked"
	pr.CreatedAt, pr.UpdatedAt = ghfake.At("2026-09-01T00:00:00Z"), ghfake.At("2026-09-02T00:00:00Z")
	pr.Requested, pr.Labels = []string{"bob"}, []string{"bug"}
	pr.Additions, pr.Deletions = 12, 4
	pr.Reviews = []ghfake.Review{{ID: 1, State: "CHANGES_REQUESTED", Author: "carol", SubmittedAt: ghfake.At("2026-09-02T00:00:00Z")}}
	pr.CheckRuns = []ghfake.CheckRun{{ID: 1, Status: "completed", Conclusion: "failure"}}
	pr.StatusState = "pending"
	return g, pr
}

func TestRepoSyncPRsFlow(t *testing.T) {
	api, hello := helloPR()
	db := filepath.Join(t.TempDir(), "babysitter.db")
	run := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, api, db, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out
	}

	if out := run("repo", "add", "octo/hello"); out != "Watching octo/hello\n" {
		t.Fatalf("repo add = %q", out)
	}
	if out := run("repo", "add", "https://github.com/octo/hello"); out != "Already watching octo/hello\n" {
		t.Fatalf("second repo add = %q", out)
	}
	if out := run("repo", "list"); !strings.HasPrefix(out, "REPOSITORY") || !strings.Contains(out, "octo/hello") || !strings.Contains(out, "never") {
		t.Fatalf("repo list = %q", out)
	}
	if out := run("prs"); strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Fatalf("prs before sync = %q", out)
	}

	run("sync")

	var prs []prOutput
	if err := json.Unmarshal([]byte(run("prs", "-o", "json")), &prs); err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 {
		t.Fatalf("prs = %+v", prs)
	}
	pr := prs[0]
	if pr.Repository != "octo/hello" || pr.Number != 3 || pr.Title != "Fix the thing" || !pr.Draft ||
		pr.ReviewDecision != "changes_requested" || pr.ChangesRequested != 1 || pr.CIStatus != "failure" ||
		pr.MergeableState != "blocked" || pr.Additions != 12 || pr.Deletions != 4 ||
		len(pr.RequestedReviewers) != 1 || pr.RequestedReviewers[0] != "bob" || len(pr.Labels) != 1 {
		t.Fatalf("pr = %+v", pr)
	}

	text := run("prs")
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "REPOSITORY") {
		t.Fatalf("prs text = %q", text)
	}
	for _, want := range []string{"octo/hello", "draft", "changes_requested", "failure", "blocked", "+12/-4"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("prs text row is missing %q: %q", want, lines[1])
		}
	}
	if out := run("prs", "--repo", "octo/other"); strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Fatalf("prs --repo other = %q", out)
	}
	if out := run("repo", "list", "-o", "json"); !strings.Contains(out, `"last_synced_at": "20`) {
		t.Fatalf("repo list after sync = %q", out)
	}

	api.Update(func() {
		hello.State, hello.Merged, hello.MergedAt = "closed", true, ghfake.At("2026-09-03T00:00:00Z")
	})
	run("sync")
	if out := run("prs"); strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Fatalf("prs after merge = %q", out)
	}
	if err := json.Unmarshal([]byte(run("prs", "--state", "all", "-o", "json")), &prs); err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].State != "merged" || prs[0].MergedAt == nil {
		t.Fatalf("prs all = %+v", prs)
	}

	if out := run("repo", "remove", "octo/hello"); out != "Stopped watching octo/hello\n" {
		t.Fatalf("repo remove = %q", out)
	}
	if _, err := runCLI(t, api, db, "repo", "remove", "octo/hello"); err == nil {
		t.Fatal("second repo remove expected error")
	}
	if out := run("prs", "--state", "all"); strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Fatalf("prs after remove = %q", out)
	}
}

func TestPRsRejectsBadState(t *testing.T) {
	t.Parallel()
	_, err := runCLI(t, ghfake.New(), filepath.Join(t.TempDir(), "x.db"), "prs", "--state", "bogus")
	if err == nil || !strings.Contains(err.Error(), "unknown state") {
		t.Fatalf("err = %v", err)
	}
}

func TestRepoAddRejectsBadName(t *testing.T) {
	t.Parallel()
	_, err := runCLI(t, ghfake.New(), filepath.Join(t.TempDir(), "x.db"), "repo", "add", "nope")
	if err == nil || !strings.Contains(err.Error(), "want owner/name") {
		t.Fatalf("err = %v", err)
	}
}

func TestServeRejectsBadInterval(t *testing.T) {
	t.Parallel()
	_, err := runCLI(t, ghfake.New(), filepath.Join(t.TempDir(), "x.db"), "serve", "--interval", "0")
	if err == nil || !strings.Contains(err.Error(), "interval") {
		t.Fatalf("err = %v", err)
	}
}

func TestDBPathFromEnv(t *testing.T) {
	t.Setenv("BABYSITTER_DB", "/tmp/env.db")
	o := &options{}
	if p, _ := o.dbPath(); p != "/tmp/env.db" {
		t.Fatalf("dbPath() = %q", p)
	}
	o.db = "/tmp/flag.db"
	if p, _ := o.dbPath(); p != "/tmp/flag.db" {
		t.Fatalf("dbPath() with flag = %q", p)
	}
}

type fakeManager struct {
	calls []string
	cfg   service.Config
	st    service.Status
}

func (m *fakeManager) Install(ctx context.Context, cfg service.Config) error {
	m.calls = append(m.calls, "install")
	m.cfg = cfg
	m.st.Installed = true
	return nil
}

func (m *fakeManager) Uninstall(context.Context) error {
	m.calls = append(m.calls, "uninstall")
	return nil
}

func (m *fakeManager) Start(context.Context) error { m.calls = append(m.calls, "start"); return nil }
func (m *fakeManager) Stop(context.Context) error  { m.calls = append(m.calls, "stop"); return nil }
func (m *fakeManager) Status(context.Context) (service.Status, error) {
	m.calls = append(m.calls, "status")
	return m.st, nil
}

func runService(t *testing.T, m service.Manager, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd(WithServiceManager(m))
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&strings.Builder{})
	root.SetArgs(append([]string{"service"}, args...))
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func TestServiceCommands(t *testing.T) {
	m := &fakeManager{st: service.Status{Path: "/x/unit", Running: true, PID: 9}}

	out, err := runService(t, m, "install", "--interval", "2m", "--db", "/tmp/d.db")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Installed the service at /x/unit") {
		t.Fatalf("install out = %q", out)
	}
	want := []string{"serve", "--interval", "2m0s", "--db", "/tmp/d.db"}
	if strings.Join(m.cfg.Args, " ") != strings.Join(want, " ") || !filepath.IsAbs(m.cfg.Executable) {
		t.Fatalf("cfg = %+v", m.cfg)
	}

	for _, c := range []string{"start", "stop", "uninstall"} {
		if _, err := runService(t, m, c); err != nil {
			t.Fatalf("%s: %v", c, err)
		}
	}
	out, err = runService(t, m, "status", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var st service.Status
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("status json %q: %v", out, err)
	}
	if !st.Installed || !st.Running || st.PID != 9 {
		t.Fatalf("status = %+v", st)
	}
	out, _ = runService(t, m, "status")
	if !strings.Contains(out, "Running:   yes") || !strings.Contains(out, "PID:       9") {
		t.Fatalf("status text = %q", out)
	}
	if got := strings.Join(m.calls, ","); got != "install,status,start,stop,uninstall,status,status" {
		t.Fatalf("calls = %s", got)
	}
}

func TestWorktreeLineNamesTheBranchThatStays(t *testing.T) {
	t.Parallel()
	w := httpd.Watch{
		SourceDir: "/home/alice/hello", WorktreeDir: "/data/worktrees/octo-hello-3",
		Summary: &httpd.WatchSummary{WorktreeRemoved: true, WorkBranchLeft: "babysitter/fix"},
	}
	got := worktreeLine(w)
	if !strings.Contains(got, "worktree removed from "+w.WorktreeDir) {
		t.Fatalf("worktreeLine() = %q", got)
	}
	if !strings.Contains(got, "babysitter/fix") || !strings.Contains(got, w.SourceDir) {
		t.Fatalf("worktreeLine() says nothing about the branch that stays: %q", got)
	}
	w.Summary.WorkBranchLeft = ""
	if got := worktreeLine(w); got != "worktree removed from "+w.WorktreeDir {
		t.Fatalf("worktreeLine() of a full removal = %q", got)
	}
	w.Summary = nil
	if got := worktreeLine(w); got != "worktree left at "+w.WorktreeDir {
		t.Fatalf("worktreeLine() of a kept worktree = %q", got)
	}
}
