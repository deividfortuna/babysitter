package checks

import (
	"testing"

	"github.com/google/go-github/v91/github"
)

func run(status, conclusion string) *github.CheckRun {
	return &github.CheckRun{Status: new(status), Conclusion: new(conclusion)}
}

func combined(state string, statuses ...string) *github.CombinedStatus {
	c := &github.CombinedStatus{State: new(state)}
	for _, s := range statuses {
		c.Statuses = append(c.Statuses, &github.RepoStatus{State: new(s)})
	}
	return c
}

func TestOverall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		runs     []*github.CheckRun
		combined *github.CombinedStatus
		want     CIStatus
	}{
		{name: "nothing", want: CINone},
		{name: "combined pending with zero statuses", combined: combined("pending"), want: CINone},
		{name: "success", runs: []*github.CheckRun{run("completed", "success")}, want: CISuccess},
		{name: "skipped and neutral are success", runs: []*github.CheckRun{run("completed", "skipped"), run("completed", "neutral")}, want: CISuccess},
		{name: "one queued is pending", runs: []*github.CheckRun{run("completed", "success"), run("queued", "")}, want: CIPending},
		{name: "in progress is pending", runs: []*github.CheckRun{run("in_progress", "")}, want: CIPending},
		{name: "one failure wins", runs: []*github.CheckRun{run("completed", "success"), run("in_progress", ""), run("completed", "failure")}, want: CIFailure},
		{name: "timed out is failure", runs: []*github.CheckRun{run("completed", "timed_out")}, want: CIFailure},
		{name: "action required is pending", runs: []*github.CheckRun{run("completed", "success"), run("completed", "action_required")}, want: CIPending},
		{name: "deployment review is pending", runs: []*github.CheckRun{run("completed", "success"), run("waiting", "")}, want: CIPending},
		{name: "stale is not a failure", runs: []*github.CheckRun{run("completed", "stale")}, want: CISuccess},
		{name: "legacy status success", combined: combined("success", "success"), want: CISuccess},
		{name: "legacy status error", runs: []*github.CheckRun{run("completed", "success")}, combined: combined("failure", "success", "error"), want: CIFailure},
		{name: "legacy status pending", combined: combined("pending", "pending"), want: CIPending},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Overall(tc.runs, tc.combined); got != tc.want {
				t.Fatalf("CIStatus() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClassifyCheckRun(t *testing.T) {
	t.Parallel()
	cases := map[State]struct {
		status, conclusion string
	}{
		Pending: {"queued", ""},
		Failed:  {"completed", "cancelled"},
		Passed:  {"completed", "success"},
		Skipped: {"completed", "neutral"},
	}
	for want, c := range cases {
		if got := ClassifyCheckRun(run(c.status, c.conclusion)); got != want {
			t.Errorf("ClassifyCheckRun(%s, %s) = %s, want %s", c.status, c.conclusion, got, want)
		}
	}
	for state, want := range map[string]State{"pending": Pending, "error": Failed, "success": Passed, "other": Skipped} {
		if got := ClassifyStatus(&github.RepoStatus{State: new(state)}); got != want {
			t.Errorf("ClassifyStatus(%s) = %s, want %s", state, got, want)
		}
	}
}

func TestSummarize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name              string
		states            map[string]State
		headSHA, greenSHA string
		want              string
	}{
		{name: "no checks", want: "none"},
		{name: "failing names them, sorted", states: map[string]State{"lint": Failed, "build": Failed, "test": Passed}, want: "failing: build, lint"},
		{name: "failure wins over pending", states: map[string]State{"build": Failed, "test": Pending}, want: "failing: build"},
		{name: "pending counts", states: map[string]State{"build": Pending, "test": Pending, "lint": Passed}, want: "pending (2)"},
		{name: "green on the head", states: map[string]State{"build": Passed}, headSHA: "abc", greenSHA: "abc", want: "green"},
		{name: "passed on an earlier commit", states: map[string]State{"build": Passed}, headSHA: "def", greenSHA: "abc", want: "passed"},
		{name: "passed without a green commit", states: map[string]State{"build": Passed}, headSHA: "abc", want: "passed"},
		{name: "skipped only is passed", states: map[string]State{"build": Skipped}, want: "passed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Summarize(tc.states, tc.headSHA, tc.greenSHA); got != tc.want {
				t.Errorf("Summarize(%v, %q, %q) = %q, want %q", tc.states, tc.headSHA, tc.greenSHA, got, tc.want)
			}
		})
	}
}
