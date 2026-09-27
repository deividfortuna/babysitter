package prwatch

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

func greenSnapshot(sha string) *snapshot.Snapshot {
	return &snapshot.Snapshot{
		PR: snapshot.PR{
			Repo: "octo/hello", Number: 3, URL: "https://github.com/octo/hello/pull/3", State: "open",
			HeadSHA: sha, HeadBranch: "fix", BaseBranch: "main", MergeableState: "clean",
		},
		Checks: snapshot.Checks{Status: checks.CISuccess, PassedCount: 2, AllTerminal: true, Items: []snapshot.Check{
			{Name: "build", Source: "check_run", Status: "completed", Conclusion: "success", URL: "https://ci/build"},
			{Name: "lint", Source: "status", Status: "success", URL: "https://ci/lint"},
		}},
	}
}

func kinds(as []store.Activity) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, string(a.Kind)+":"+a.Ref)
	}
	return out
}

func equalKinds(t *testing.T, got []store.Activity, want ...string) {
	t.Helper()
	g := kinds(got)
	if len(g) != len(want) {
		t.Fatalf("activity = %v, want %v", g, want)
	}
	for i := range want {
		if g[i] != want[i] {
			t.Fatalf("activity = %v, want %v", g, want)
		}
	}
}

func TestDiffBaselineAndQuietPoll(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	s := greenSnapshot("abc")

	items, next := Diff(State{}, s, now)
	equalKinds(t, items, "checks_green:abc")
	if next.HeadSHA != "abc" || next.GreenSHA != "abc" || next.MergeableState != "clean" ||
		next.Checks["build"] != "passed" || next.Checks["lint"] != "passed" {
		t.Fatalf("next = %+v", next)
	}

	items, again := Diff(next, s, now)
	equalKinds(t, items)
	if again.GreenSHA != "abc" {
		t.Fatalf("again = %+v", again)
	}
}

func TestDiffReviewItems(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	line := 12
	s := greenSnapshot("abc")
	s.NewReviewItems = []snapshot.ReviewItem{
		{Kind: store.KindIssueComment, ID: 1, Author: "bob", Body: "Looks good\nmore", URL: "https://c/1"},
		{Kind: store.KindReviewComment, ID: 2, Author: "bob", Body: "Return the error", Path: "main.go", Line: &line, Side: "RIGHT", CommitID: "abc", URL: "https://c/2"},
		{Kind: store.KindReview, ID: 3, Author: "carol", Body: "", State: "CHANGES_REQUESTED", URL: "https://c/3"},
	}
	prev := State{HeadSHA: "abc", GreenSHA: "abc", MergeableState: "clean", Checks: map[string]checks.State{"build": checks.Passed, "lint": checks.Passed}}
	items, _ := Diff(prev, s, now)
	equalKinds(t, items, "comment:1", "review_comment:2", "review:3")
	if items[0].Summary != "bob commented: Looks good" || items[0].Actor != "bob" || items[0].URL != "https://c/1" {
		t.Fatalf("comment = %+v", items[0])
	}
	if items[1].Summary != "bob commented on main.go:12: Return the error" {
		t.Fatalf("review comment = %+v", items[1])
	}
	if items[2].Summary != "carol changes requested" {
		t.Fatalf("review = %+v", items[2])
	}
	var payload map[string]any
	if err := json.Unmarshal(items[2].Payload, &payload); err != nil || payload["state"] != "CHANGES_REQUESTED" || payload["item_id"] != float64(3) {
		t.Fatalf("payload = %s, %v", items[2].Payload, err)
	}
	var inline map[string]any
	if err := json.Unmarshal(items[1].Payload, &inline); err != nil {
		t.Fatal(err)
	}
	if inline["path"] != "main.go" || inline["line"] != float64(12) || inline["side"] != "RIGHT" || inline["commit_id"] != "abc" {
		t.Fatalf("inline payload = %s", items[1].Payload)
	}
}

func TestTheGreenRowCountsTheChecksByName(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	s := greenSnapshot("abc")
	s.Checks.PassedCount = 3
	s.Checks.Items = append(s.Checks.Items, snapshot.Check{Name: "build", Source: "check_run", Status: "completed", Conclusion: "success", URL: "https://ci/build2"})
	items, next := Diff(State{}, s, now)
	if len(next.Checks) != 2 {
		t.Fatalf("checks = %+v", next.Checks)
	}
	green := items[len(items)-1]
	if green.Kind != store.ActivityChecksGreen || green.Summary != "all 2 checks passed on abc" {
		t.Fatalf("green row = %+v", green)
	}
	var payload map[string]any
	if err := json.Unmarshal(green.Payload, &payload); err != nil || payload["passed"] != float64(2) {
		t.Fatalf("payload = %s, %v", green.Payload, err)
	}
}

func TestDiffChecksAndCommits(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	prev := State{HeadSHA: "abc", GreenSHA: "abc", MergeableState: "clean", Checks: map[string]checks.State{"build": checks.Passed, "lint": checks.Passed}}

	s := greenSnapshot("abc")
	s.Checks.Status = checks.CIFailure
	s.Checks.PassedCount, s.Checks.FailedCount = 1, 1
	s.Checks.Items[0].Conclusion = "failure"
	items, next := Diff(prev, s, now)
	equalKinds(t, items, "check_failed:build@abc")
	if next.Checks["build"] != "failed" || next.GreenSHA != "abc" {
		t.Fatalf("next = %+v", next)
	}
	items, _ = Diff(next, s, now)
	equalKinds(t, items)

	s = greenSnapshot("abc")
	items, next = Diff(next, s, now)
	equalKinds(t, items, "check_recovered:build@abc")

	s = greenSnapshot("def")
	s.Checks = snapshot.Checks{Status: checks.CIPending, PendingCount: 1, Items: []snapshot.Check{
		{Name: "build", Source: "check_run", Status: "in_progress"},
	}}
	items, next = Diff(next, s, now)
	equalKinds(t, items, "commit:def")
	var payload map[string]any
	if err := json.Unmarshal(items[0].Payload, &payload); err != nil || payload["previous"] != "abc" {
		t.Fatalf("commit payload = %s, %v", items[0].Payload, err)
	}
	if next.GreenSHA != "" || next.Checks["build"] != "pending" || len(next.Checks) != 1 {
		t.Fatalf("next after commit = %+v", next)
	}
	s = greenSnapshot("def")
	items, next = Diff(next, s, now)
	equalKinds(t, items, "checks_green:def")
	if next.GreenSHA != "def" {
		t.Fatalf("next after green = %+v", next)
	}

	s.Checks.Items[0].Conclusion = "failure"
	s.Checks.Status, s.Checks.FailedCount = checks.CIFailure, 1
	items, _ = Diff(next, s, now)
	equalKinds(t, items, "check_failed:build@def")

	s = greenSnapshot("def")
	s.Checks.Status = checks.CIFailure
	s.FailedRuns = []snapshot.Run{{RunID: 9, WorkflowName: "Release", Status: "completed", Conclusion: "startup_failure", HTMLURL: "https://run/9"}}
	items, next2 := Diff(next, s, now)
	equalKinds(t, items, "check_failed:Release@def")
	if next2.Checks["Release"] != "failed" {
		t.Fatalf("next = %+v", next2)
	}
	items, _ = Diff(next2, s, now)
	equalKinds(t, items)

	s = greenSnapshot("def")
	s.Checks.Items[0].Conclusion, s.Checks.Items[0].CheckSuiteID = "failure", 501
	s.Checks.Status, s.Checks.FailedCount = checks.CIFailure, 1
	s.FailedRuns = []snapshot.Run{{RunID: 9, CheckSuiteID: 501, WorkflowName: "CI", Status: "completed", Conclusion: "failure", HTMLURL: "https://run/9"}}
	items, next3 := Diff(next, s, now)
	equalKinds(t, items, "check_failed:build@def")
	if _, ok := next3.Checks["CI"]; ok {
		t.Fatalf("the run counts beside its check: %+v", next3.Checks)
	}
}

func TestDiffMergeabilityAndEnd(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	prev := State{HeadSHA: "abc", GreenSHA: "abc", MergeableState: "clean", Checks: map[string]checks.State{"build": checks.Passed, "lint": checks.Passed}}

	s := greenSnapshot("abc")
	s.PR.MergeableState = "unknown"
	items, next := Diff(prev, s, now)
	equalKinds(t, items)
	if next.MergeableState != "clean" {
		t.Fatalf("unknown overwrote the state: %+v", next)
	}

	s.PR.MergeableState = "behind"
	items, next = Diff(next, s, now)
	equalKinds(t, items, "behind:behind@abc")
	items, next = Diff(next, s, now)
	equalKinds(t, items)

	s.PR.MergeableState = "dirty"
	items, next = Diff(next, s, now)
	equalKinds(t, items, "conflict:conflict@abc")
	if items[0].Summary != "fix conflicts with main" {
		t.Fatalf("conflict = %+v", items[0])
	}

	s.PR.MergeableState = "clean"
	items, next = Diff(next, s, now)
	equalKinds(t, items)

	s.PR.Merged, s.PR.State = true, store.StateMerged
	items, _ = Diff(next, s, now)
	equalKinds(t, items, "merged:merged")

	s = greenSnapshot("abc")
	s.PR.Closed, s.PR.State = true, store.StateClosed
	items, _ = Diff(next, s, now)
	equalKinds(t, items, "closed:closed")
}

func TestClassifyCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		c    snapshot.Check
		want checks.State
	}{
		{snapshot.Check{Source: checks.SourceCheckRun, Status: "completed", Conclusion: "success"}, checks.Passed},
		{snapshot.Check{Source: checks.SourceCheckRun, Status: "completed", Conclusion: "failure"}, checks.Failed},
		{snapshot.Check{Source: checks.SourceCheckRun, Status: "completed", Conclusion: "timed_out"}, checks.Failed},
		{snapshot.Check{Source: checks.SourceCheckRun, Status: "completed", Conclusion: "action_required"}, checks.Pending},
		{snapshot.Check{Source: checks.SourceCheckRun, Status: "in_progress"}, checks.Pending},
		{snapshot.Check{Source: checks.SourceCheckRun, Status: "completed", Conclusion: "skipped"}, checks.Skipped},
		{snapshot.Check{Source: checks.SourceCheckRun, Status: "completed", Conclusion: "stale"}, checks.Skipped},
		{snapshot.Check{Source: checks.SourceStatus, Status: "success"}, checks.Passed},
		{snapshot.Check{Source: checks.SourceStatus, Status: "error"}, checks.Failed},
		{snapshot.Check{Source: checks.SourceStatus, Status: "pending"}, checks.Pending},
	}
	for _, tc := range cases {
		if got := classifyCheck(tc.c); got != tc.want {
			t.Errorf("classifyCheck(%+v) = %q, want %q", tc.c, got, tc.want)
		}
	}
}

func TestCurrentItems(t *testing.T) {
	t.Parallel()
	items := []store.Activity{
		{ID: 1, Kind: store.ActivityCheckFailed, Payload: json.RawMessage(`{"sha":"old"}`)},
		{ID: 2, Kind: store.ActivityCheckFailed, Payload: json.RawMessage(`{"sha":"new"}`)},
		{ID: 3, Kind: store.ActivityComment, Payload: json.RawMessage(`{}`)},
		{ID: 4, Kind: store.ActivityCheckFailed, Payload: json.RawMessage(`{}`)},
	}
	got, stale := currentItems(items, "new")
	if len(got) != 3 || got[0].ID != 2 || got[1].ID != 3 || got[2].ID != 4 || len(stale) != 1 || stale[0].ID != 1 {
		t.Fatalf("currentItems = %+v, stale %+v", got, stale)
	}
	if got, stale := currentItems(items, ""); len(got) != 4 || len(stale) != 0 {
		t.Fatalf("currentItems without head = %d, %d", len(got), len(stale))
	}
}

func TestSummaryOfANonASCIICommentIsValidUTF8(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("a", 116) + strings.Repeat("é", 20)
	got := firstLine(body)
	if !utf8.ValidString(got) {
		t.Fatalf("firstLine() = %q, which is not valid UTF-8", got)
	}
}

func TestAFailedRunTakesTheJobOfItsOwnRun(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	s := greenSnapshot("abc")
	s.Checks.Status = checks.CIFailure
	s.FailedRuns = []snapshot.Run{{RunID: 90, WorkflowName: "CI", Status: "completed", Conclusion: "failure", HTMLURL: "https://run/90"}}
	s.FailedJobs = []snapshot.FailedJob{
		{RunID: 77, JobID: 9, JobName: "CI", LogsEndpoint: "repos/octo/hello/actions/jobs/9/logs"},
		{RunID: 90, JobID: 30, JobName: "CI", LogsEndpoint: "repos/octo/hello/actions/jobs/30/logs"},
	}

	items, _ := Diff(State{}, s, now)
	equalKinds(t, items, "check_failed:CI@abc")
	var p struct {
		RunID        int64  `json:"run_id"`
		JobID        int64  `json:"job_id"`
		LogsEndpoint string `json:"logs_endpoint"`
	}
	if err := json.Unmarshal(items[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.RunID != 90 || p.JobID != 30 || p.LogsEndpoint != "repos/octo/hello/actions/jobs/30/logs" {
		t.Fatalf("the check of run 90 took job %d of run %d (%s), want job 30 of run 90", p.JobID, p.RunID, p.LogsEndpoint)
	}
}
