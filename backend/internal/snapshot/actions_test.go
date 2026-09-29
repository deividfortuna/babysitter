package snapshot

import (
	"strings"
	"testing"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/checks"
)

func TestRecommend(t *testing.T) {
	t.Parallel()
	green := Checks{Status: "success", PassedCount: 2, AllTerminal: true}
	failed := Checks{Status: "failure", PassedCount: 1, FailedCount: 1, AllTerminal: true}
	ready := PR{Mergeable: new(true), MergeableState: "clean", ReviewDecision: "approved", Approvals: 1}
	budget := RetryState{CurrentSHARetriesUsed: 0, MaxFlakyRetries: 3}
	item := []ReviewItem{{Kind: "review", ID: 1}}
	run := []Run{{RunID: 1}}
	waiting := []Run{{RunID: 2, Conclusion: "action_required"}}
	actionCheck := Checks{Status: "pending", PassedCount: 1, PendingCount: 1, Items: []Check{{Name: "deploy", Conclusion: "action_required"}}}
	waitingCheck := Checks{Status: "pending", PassedCount: 1, PendingCount: 1, Items: []Check{{Name: "deploy", Status: "waiting"}}}
	deployWaits := []Run{{RunID: 3, Status: "waiting"}}

	cases := []struct {
		name string
		s    Snapshot
		want string
	}{
		{"merged", Snapshot{PR: PR{Merged: true, Closed: true}, Checks: green}, "stop_pr_closed"},
		{"closed with comment", Snapshot{PR: PR{Closed: true}, NewReviewItems: item}, "process_review_comment,stop_pr_closed"},
		{"ready", Snapshot{PR: ready, Checks: green, RetryState: budget}, "ready_to_merge"},
		{"ready but comment", Snapshot{PR: ready, Checks: green, NewReviewItems: item, RetryState: budget}, "process_review_comment"},
		{"ready but review required", Snapshot{PR: PR{Mergeable: new(true), MergeableState: "blocked", ReviewDecision: "review_required"}, Checks: green}, "idle"},
		{"ready but unknown mergeable", Snapshot{PR: PR{MergeableState: "unknown", ReviewDecision: "approved"}, Checks: green}, "idle"},
		{"ready but behind", Snapshot{PR: PR{Mergeable: new(true), MergeableState: "behind", ReviewDecision: "approved"}, Checks: green}, "idle"},
		{"ready but no checks", Snapshot{PR: ready, Checks: Checks{Status: "none", AllTerminal: true}, RetryState: budget}, "idle"},
		{"ready but run awaits approval", Snapshot{PR: ready, Checks: green, AwaitingApproval: waiting, RetryState: budget}, "stop_action_required"},
		{"check awaits a person", Snapshot{PR: ready, Checks: actionCheck, RetryState: budget}, "stop_action_required"},
		{"check awaits a deployment review", Snapshot{PR: ready, Checks: waitingCheck, RetryState: budget}, "stop_action_required"},
		{"run awaits a deployment review", Snapshot{PR: ready, Checks: green, AwaitingApproval: deployWaits, RetryState: budget}, "stop_action_required"},
		{"failure and approval", Snapshot{PR: ready, Checks: failed, FailedRuns: run, AwaitingApproval: waiting, RetryState: budget}, "diagnose_ci_failure,retry_failed_checks,stop_action_required"},
		{"pending", Snapshot{PR: ready, Checks: Checks{PendingCount: 1}}, "idle"},
		{"failed, budget left", Snapshot{PR: ready, Checks: failed, FailedRuns: run, RetryState: budget}, "diagnose_ci_failure,retry_failed_checks"},
		{"failed, no runs", Snapshot{PR: ready, Checks: failed, RetryState: budget}, "diagnose_ci_failure"},
		{"failed, no runs, no budget", Snapshot{PR: ready, Checks: failed, RetryState: RetryState{0, 0}}, "diagnose_ci_failure"},
		{"failed run without a check", Snapshot{PR: ready, Checks: green, FailedRuns: run, RetryState: budget}, "diagnose_ci_failure,retry_failed_checks"},
		{"failed, still pending", Snapshot{PR: ready, Checks: Checks{FailedCount: 1, PendingCount: 1}, FailedRuns: run, RetryState: budget}, "diagnose_ci_failure"},
		{"failed job only", Snapshot{PR: ready, Checks: Checks{PendingCount: 1}, FailedJobs: []FailedJob{{JobID: 1}}, RetryState: budget}, "diagnose_ci_failure"},
		{"exhausted", Snapshot{PR: ready, Checks: failed, FailedRuns: run, RetryState: RetryState{3, 3}}, "diagnose_ci_failure,stop_exhausted_retries"},
		{"no budget at all", Snapshot{PR: ready, Checks: failed, FailedRuns: run, RetryState: RetryState{0, 0}}, "diagnose_ci_failure,stop_exhausted_retries"},
		{"comment and failure", Snapshot{PR: ready, Checks: failed, FailedRuns: run, NewReviewItems: item, RetryState: budget}, "process_review_comment,diagnose_ci_failure,retry_failed_checks"},
	}
	for _, c := range cases {
		if got := strings.Join(recommend(&c.s), ","); got != c.want {
			t.Errorf("%s: actions = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestSummarizeChecks(t *testing.T) {
	t.Parallel()
	runs := []*github.CheckRun{
		{Name: new("build"), Status: new("completed"), Conclusion: new("success"), CheckSuite: &github.CheckSuite{ID: new(int64(501))}},
		{Name: new("test"), Status: new("completed"), Conclusion: new("timed_out")},
		{Name: new("lint"), Status: new("in_progress")},
		{Name: new("docs"), Status: new("completed"), Conclusion: new("skipped")},
	}
	combined := &github.CombinedStatus{Statuses: []*github.RepoStatus{
		{Context: new("ci/legacy"), State: new("error")},
		{Context: new("ci/other"), State: new("success")},
	}}
	c := summarizeChecks(runs, combined)
	if c.Status != "failure" || c.PassedCount != 2 || c.FailedCount != 2 || c.PendingCount != 1 || c.SkippedCount != 1 || c.AllTerminal {
		t.Fatalf("checks = %+v", c)
	}
	if len(c.Items) != 6 || c.Items[4].Source != "status" || c.Items[4].Name != "ci/legacy" || c.Items[0].CheckSuiteID != 501 {
		t.Fatalf("items = %+v", c.Items)
	}
	empty := summarizeChecks(nil, nil)
	if empty.Status != "none" || !empty.AllTerminal || len(empty.Items) != 0 {
		t.Fatalf("empty = %+v", empty)
	}
	waiting := summarizeChecks([]*github.CheckRun{
		{Name: new("deploy"), Status: new("completed"), Conclusion: new("action_required")},
		{Name: new("old"), Status: new("completed"), Conclusion: new("stale")},
	}, nil)
	if waiting.Status != "pending" || waiting.PendingCount != 1 || waiting.SkippedCount != 1 || waiting.FailedCount != 0 || waiting.AllTerminal {
		t.Fatalf("waiting = %+v", waiting)
	}
}

func TestNeedsApproval(t *testing.T) {
	t.Parallel()
	run := func(status checks.RunStatus, conclusion checks.Conclusion) *github.WorkflowRun {
		return &github.WorkflowRun{Status: new(string(status)), Conclusion: new(string(conclusion))}
	}
	cases := []struct {
		name string
		run  *github.WorkflowRun
		want bool
	}{
		{"conclusion", run(checks.StatusCompleted, checks.ConclusionActionRequired), true},
		{"status without a conclusion", run(checks.StatusActionRequired, ""), true},
		{"deployment review", run(checks.StatusWaiting, ""), false},
		{"queued", run(checks.RunStatus("queued"), ""), false},
		{"completed", run(checks.StatusCompleted, checks.ConclusionSuccess), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsApproval(tc.run); got != tc.want {
				t.Fatalf("needsApproval() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBlockers(t *testing.T) {
	t.Parallel()
	green := Checks{Status: "success", PassedCount: 2, AllTerminal: true}
	good := PR{Mergeable: new(true), MergeableState: "clean", ReviewDecision: "approved", Approvals: 1}
	with := func(f func(pr *PR)) PR {
		pr := good
		f(&pr)
		return pr
	}
	cases := []struct {
		name      string
		s         Snapshot
		approvals int
		want      string
	}{
		{"good", Snapshot{PR: good, Checks: green}, 1, ""},
		{"two approvals wanted", Snapshot{PR: good, Checks: green}, 2, "1 of 2 approvals"},
		{"two approvals given", Snapshot{PR: with(func(pr *PR) { pr.Approvals = 2 }), Checks: green}, 2, ""},
		{"merged", Snapshot{PR: with(func(pr *PR) { pr.Merged = true }), Checks: green}, 1, "already merged"},
		{"closed", Snapshot{PR: with(func(pr *PR) { pr.Closed = true }), Checks: green}, 1, "closed"},
		{"draft", Snapshot{PR: with(func(pr *PR) { pr.Draft = true }), Checks: green}, 1, "still a draft"},
		{"check failed", Snapshot{PR: good, Checks: Checks{Status: "failure", FailedCount: 1, PassedCount: 1, AllTerminal: true}}, 1, "1 check failed"},
		{"checks pending", Snapshot{PR: good, Checks: Checks{Status: "pending", PendingCount: 2, PassedCount: 1}}, 1, "2 checks pending"},
		{"no checks", Snapshot{PR: good, Checks: Checks{Status: "none", AllTerminal: true}}, 1, "no checks ran"},
		{"failed run", Snapshot{PR: good, Checks: green, FailedRuns: []Run{{RunID: 1}}}, 1, "1 workflow run failed"},
		{"failed run behind a failed check", Snapshot{PR: good, Checks: Checks{Status: "failure", FailedCount: 1, AllTerminal: true, Items: []Check{{Name: "build", CheckSuiteID: 5}}}, FailedRuns: []Run{{RunID: 1, CheckSuiteID: 5, WorkflowName: "CI"}}}, 1, "1 check failed"},
		{"failed run named as a check", Snapshot{PR: good, Checks: Checks{Status: "failure", FailedCount: 1, AllTerminal: true, Items: []Check{{Name: "CI"}}}, FailedRuns: []Run{{RunID: 1, WorkflowName: "CI"}}}, 1, "1 check failed"},
		{"run waits", Snapshot{PR: good, Checks: green, AwaitingApproval: []Run{{RunID: 1}}}, 1, "1 workflow run waits for a person"},
		{"mergeable unknown", Snapshot{PR: with(func(pr *PR) { pr.Mergeable = nil; pr.MergeableState = "unknown" }), Checks: green}, 1, "GitHub has not computed the mergeable state yet"},
		{"not mergeable", Snapshot{PR: with(func(pr *PR) { pr.Mergeable = new(false); pr.MergeableState = "dirty" }), Checks: green}, 1, "GitHub reports it cannot merge"},
		{"behind", Snapshot{PR: with(func(pr *PR) { pr.MergeableState = "behind" }), Checks: green}, 1, "GitHub reports behind"},
		{"blocked", Snapshot{PR: with(func(pr *PR) { pr.MergeableState = "blocked" }), Checks: green}, 1, "GitHub reports blocked"},
		{"blocked and behind", Snapshot{PR: with(func(pr *PR) { pr.MergeableState, pr.BehindBy, pr.BaseBranch = "blocked", 3, "main" }), Checks: green}, 1, "GitHub reports blocked, and the branch is 3 commits behind main"},
		{"blocked and not compared", Snapshot{PR: with(func(pr *PR) { pr.MergeableState, pr.BehindErr, pr.BaseBranch = "blocked", "404 Not Found", "main" }), Checks: green}, 1, "GitHub reports blocked, and the compare with main failed: 404 Not Found"},
		{"changes requested", Snapshot{PR: with(func(pr *PR) { pr.ChangesRequested = 1 }), Checks: green}, 1, "1 reviewer requested changes"},
		{"no approval", Snapshot{PR: with(func(pr *PR) { pr.Approvals = 0 }), Checks: green}, 1, "no approval yet"},
		{"reviewer asked", Snapshot{PR: with(func(pr *PR) { pr.RequestedReviewers = []string{"alice", "bob"} }), Checks: green}, 1, "waiting for a review from alice, bob"},
		{"threads", Snapshot{PR: good, Checks: green, Threads: Threads{Unresolved: 3, Unanswered: 3}}, 1, "3 review threads unresolved"},
		{"threads answered", Snapshot{PR: good, Checks: green, Threads: Threads{Unresolved: 3}}, 1, "3 review threads unresolved"},
		{"threads unreadable", Snapshot{PR: good, Checks: green, Threads: Threads{Err: "no access"}}, 1, "review threads unreadable: no access"},
		{"new item", Snapshot{PR: good, Checks: green, NewReviewItems: []ReviewItem{{ID: 1}}}, 1, "1 new review item not looked at yet"},
		{"several", Snapshot{PR: with(func(pr *PR) { pr.Draft = true; pr.Approvals = 0 }), Checks: green}, 1, "still a draft; no approval yet"},
	}
	for _, c := range cases {
		if got := strings.Join(Blockers(&c.s, c.approvals), "; "); got != c.want {
			t.Errorf("%s: blockers = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWaitsOnlyForReview(t *testing.T) {
	t.Parallel()
	green := Checks{Status: "success", PassedCount: 2, AllTerminal: true}
	good := PR{Mergeable: new(true), MergeableState: "clean", Approvals: 1}
	with := func(f func(pr *PR)) PR {
		pr := good
		f(&pr)
		return pr
	}
	cases := []struct {
		name string
		s    Snapshot
		want bool
	}{
		{"ready to merge", Snapshot{PR: good, Checks: green}, false},
		{"changes requested", Snapshot{PR: with(func(pr *PR) { pr.ChangesRequested = 1 }), Checks: green}, true},
		{"no approval", Snapshot{PR: with(func(pr *PR) { pr.Approvals = 0 }), Checks: green}, true},
		{"request pending", Snapshot{PR: with(func(pr *PR) { pr.RequestedReviewers = []string{"bob"} }), Checks: green}, true},
		{"merged", Snapshot{PR: with(func(pr *PR) { pr.Merged = true; pr.Approvals = 0 }), Checks: green}, false},
		{"closed", Snapshot{PR: with(func(pr *PR) { pr.Closed = true; pr.Approvals = 0 }), Checks: green}, false},
		{"draft", Snapshot{PR: with(func(pr *PR) { pr.Draft = true; pr.Approvals = 0 }), Checks: green}, false},
		{"check failed", Snapshot{PR: with(func(pr *PR) { pr.Approvals = 0 }), Checks: Checks{Status: "failure", FailedCount: 1, AllTerminal: true}}, false},
		{"behind", Snapshot{PR: with(func(pr *PR) { pr.Approvals = 0; pr.MergeableState = "behind" }), Checks: green}, false},
		{"threads", Snapshot{PR: with(func(pr *PR) { pr.Approvals = 0 }), Checks: green, Threads: Threads{Unresolved: 3, Unanswered: 3}}, false},
		{"threads answered", Snapshot{PR: with(func(pr *PR) { pr.Approvals = 0 }), Checks: green, Threads: Threads{Unresolved: 3}}, true},
		{"threads answered on an approved pull request", Snapshot{PR: good, Checks: green, Threads: Threads{Unresolved: 2}}, true},
		{"one thread not answered", Snapshot{PR: with(func(pr *PR) { pr.Approvals = 0 }), Checks: green, Threads: Threads{Unresolved: 3, Unanswered: 1}}, false},
		{"new item", Snapshot{PR: with(func(pr *PR) { pr.Approvals = 0 }), Checks: green, NewReviewItems: []ReviewItem{{ID: 1}}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := WaitsOnlyForReview(&c.s, 1); got != c.want {
				t.Fatalf("WaitsOnlyForReview() = %v, want %v; blockers %q", got, c.want, Blockers(&c.s, 1))
			}
		})
	}
}

func TestWaitsOnlyForReviewOfAnsweredThreadsOnABranchWithNoRule(t *testing.T) {
	t.Parallel()
	s := Snapshot{
		PR:      PR{Mergeable: new(true), MergeableState: "clean", ReviewersBehindHead: []string{"alice"}},
		Checks:  Checks{Status: "success", PassedCount: 2, AllTerminal: true},
		Threads: Threads{Unresolved: 1, Reviewers: []string{"bob"}},
	}
	if !WaitsOnlyForReview(&s, 0) {
		t.Fatalf("WaitsOnlyForReview() = false, want true; blockers %q", Blockers(&s, 0))
	}
	if got := strings.Join(Rereviewers(&s, 0), ","); got != "bob" {
		t.Fatalf("Rereviewers() = %q, want bob alone", got)
	}
}

func TestRereviewers(t *testing.T) {
	t.Parallel()
	short := PR{Approvals: 0, ReviewersBehindHead: []string{"alice", "copilot-pull-request-reviewer[bot]"}}
	approved := PR{Approvals: 1, ReviewersBehindHead: []string{"alice"}}
	cases := []struct {
		name string
		s    Snapshot
		want string
	}{
		{"short of approvals, no thread answered", Snapshot{PR: short}, "alice,copilot-pull-request-reviewer[bot]"},
		{"short of approvals, threads answered", Snapshot{PR: short, Threads: Threads{Reviewers: []string{"Copilot-Pull-Request-Reviewer[bot]", "carol"}}}, "alice,carol,copilot-pull-request-reviewer[bot]"},
		{"approved, threads answered", Snapshot{PR: approved, Threads: Threads{Reviewers: []string{"bob"}}}, "bob"},
		{"approved, the approver answered in a thread", Snapshot{PR: approved, Threads: Threads{Reviewers: []string{"alice"}}}, "alice"},
		{"approved, no thread answered", Snapshot{PR: approved}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := strings.Join(Rereviewers(&c.s, 1), ","); got != c.want {
				t.Fatalf("Rereviewers() = %q, want %q", got, c.want)
			}
		})
	}
}
