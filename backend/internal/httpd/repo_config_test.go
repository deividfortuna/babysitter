package httpd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/store"
)

// jsonString quotes a path for a JSON body: a Windows path has
// backslashes that JSON escapes.
func jsonString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func checkoutOf(t *testing.T, origin string) string {
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

func TestTheRepositoryConfigurationRoutes(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	repo, err := st.AddRepo(context.Background(), "acme", "billing")
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/repos/%d/config", repo.ID)

	var cfg RepoConfig
	if rec := call(t, h, http.MethodGet, path, "", &cfg); rec.Code != http.StatusOK || cfg.AutoStartMine || cfg.DependabotScope != "patch" || cfg.DependabotApproval != "never" || cfg.DependabotLimit != 1 {
		t.Fatalf("GET config: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPatch, path, `{"autoStartMine":true}`, &cfg); rec.Code != http.StatusOK || !cfg.AutoStartMine || cfg.CheckoutDir != "" {
		t.Fatalf("a toggle without a checkout: %d %s", rec.Code, rec.Body)
	}
	web := jsonString(t, checkoutOf(t, "https://github.com/acme/web.git"))
	if rec := call(t, h, http.MethodPatch, path, `{"checkoutDir":`+web+`}`, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "has no remote for acme/billing") {
		t.Fatalf("a checkout of another repository: %d %s", rec.Code, rec.Body)
	}

	dir := checkoutOf(t, "git@github.com:acme/billing.git")
	body := `{"checkoutDir":` + jsonString(t, dir) + `,"autoStartMine":true,"includeDrafts":true,"autoWatchDependabot":true,
		"overrides":{"provider":"copilot","model":"gpt-5.3-codex","effort":"low","approvalMode":"manual","mergeMethod":"squash","approvalsRequired":null,"includeExisting":true,
			"autoApproveRebase":true,"includeOwn":false,"keepWorktree":true},
		"dependabotScope":"minor","dependabotApproval":"ask","dependabotLimit":2}`
	if rec := call(t, h, http.MethodPatch, path, body, &cfg); rec.Code != http.StatusOK {
		t.Fatalf("PATCH config: %d %s", rec.Code, rec.Body)
	}
	o := cfg.Overrides
	if cfg.CheckoutDir != dir || !cfg.AutoStartMine || cfg.AutoStartMineSince == nil || !cfg.AutoWatchDependabot || !cfg.IncludeDrafts ||
		o.Provider != "copilot" || o.Model != "gpt-5.3-codex" || o.Effort != "low" || o.ApprovalMode != "manual" || o.MergeMethod != "squash" || !o.ApprovalsRequired.Set || o.ApprovalsRequired.Value != nil ||
		!*o.IncludeExisting || !*o.AutoApproveRebase || *o.IncludeOwn || !*o.KeepWorktree || cfg.DependabotScope != "minor" || cfg.DependabotApproval != "ask" || cfg.DependabotLimit != 2 {
		t.Fatalf("config = %+v", cfg)
	}
	if rec := call(t, h, http.MethodPatch, path, `{"dependabotLimit":0}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("a limit of 0: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/repos/999/config", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown repository: %d %s", rec.Code, rec.Body)
	}
}

func TestTheQueueRouteListsTheWaitingDependabotPullRequests(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	ctx := context.Background()
	repo, err := st.AddRepo(ctx, "acme", "billing")
	if err != nil {
		t.Fatal(err)
	}
	since := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	cfg := store.DefaultRepoConfig(repo.ID)
	cfg.CheckoutDir, cfg.DependabotSince = "/code/billing", &since
	if _, err := st.SaveRepoConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	for n, level := range map[int]dependabot.Level{10: dependabot.Minor, 11: dependabot.Major} {
		created := since.Add(time.Duration(n) * time.Minute)
		pr := store.PullRequest{
			RepoID: repo.ID, Number: n, Title: "Bump", Author: "dependabot[bot]", State: store.StateOpen,
			CreatedAt: created, UpdatedAt: created, SyncedAt: created, UpdateType: level,
		}
		if err := st.UpsertPR(ctx, pr); err != nil {
			t.Fatal(err)
		}
	}
	var q RepoQueue
	if rec := call(t, h, http.MethodGet, fmt.Sprintf("/repos/%d/queue", repo.ID), "", &q); rec.Code != http.StatusOK {
		t.Fatalf("GET queue: %d %s", rec.Code, rec.Body)
	}
	if len(q.PullRequests) != 2 || q.PullRequests[0].Number != 10 || q.PullRequests[0].Position != 1 || q.PullRequests[0].UpdateType != dependabot.Minor || q.PullRequests[1].Number != 11 {
		t.Fatalf("queue = %+v, want 10 then 11", q)
	}
}

func TestMergeWhenReadyChangesOnAWatch(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	w, err := st.CreateWatch(context.Background(), store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var out Watch
	if rec := call(t, h, http.MethodPatch, fmt.Sprintf("/watches/%d", w.ID), `{"mergeWhenReady":true}`, &out); rec.Code != http.StatusOK || !out.MergeWhenReady {
		t.Fatalf("PATCH mergeWhenReady: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, fmt.Sprintf("/watches/%d/merge", w.ID), `{"approve":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("POST merge approve: %d %s", rec.Code, rec.Body)
	}
	if len(fw.merges) != 1 || !fw.merges[0].Approve {
		t.Fatalf("merges = %+v, want one with approve", fw.merges)
	}
}

func TestStartPassesMergeWhenReadyToTheWatch(t *testing.T) {
	t.Parallel()
	h, _, fw := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src","mergeWhenReady":true}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#4","sourceDir":"/src"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	starts := fw.started()
	if len(starts) != 2 || starts[0].MergeWhenReady == nil || !*starts[0].MergeWhenReady {
		t.Fatalf("starts = %+v, want mergeWhenReady true on the first", starts)
	}
	if starts[1].MergeWhenReady != nil {
		t.Fatalf("a start without the field sent mergeWhenReady %v", *starts[1].MergeWhenReady)
	}
}
