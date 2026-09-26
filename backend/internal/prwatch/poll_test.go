package prwatch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

const secretToken = "ghp_abcdefghijklmnopqrstuvwxyz1234"

func TestAFailedPollStoresTheErrorWithoutTheToken(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.gone("Bad credentials for token " + secretToken)
	fx.advance(time.Minute)
	if err := fx.svc.Poll(context.Background(), w.ID); err == nil {
		t.Fatal("Poll() error = nil, want the failure of the snapshot")
	}
	got, err := fx.st.GetWatch(context.Background(), w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastError == "" {
		t.Fatal("the failed poll recorded no error")
	}
	if strings.Contains(got.LastError, secretToken) {
		t.Fatalf("last error carries the token: %q", got.LastError)
	}
}

func TestAJobLogReachesTheAgentWithoutItsToken(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.failBuild(stampedLog(
		"##[group]Run go test ./...",
		"##[endgroup]",
		"--- FAIL: TestThing",
		"    the request used "+secretToken,
		"##[error]Process completed with exit code 1.",
	))
	w := fx.start()
	msgs := fx.host.last().messages()
	last := msgs[len(msgs)-1]
	if !strings.Contains(last, "--- FAIL: TestThing") {
		t.Fatalf("the log did not reach the agent:\n%s", last)
	}
	if strings.Contains(last, secretToken) {
		t.Fatalf("the log carried the token to the agent:\n%s", last)
	}
	for _, a := range fx.activity(w) {
		if strings.Contains(string(a.Payload), secretToken) {
			t.Fatalf("the token is in the activity of the watch: %s", a.Payload)
		}
	}
}

// failedChecks returns n failed check runs outside any suite and the n
// failed jobs of the CI run behind them.
func failedChecks(n int) ([]ghfake.CheckRun, []*ghfake.Job) {
	var cs []ghfake.CheckRun
	var js []*ghfake.Job
	for i := 1; i <= n; i++ {
		cs = append(cs, ghfake.CheckRun{ID: int64(i), Name: fmt.Sprintf("job-%d", i), Status: "completed", Conclusion: "failure"})
		js = append(js, failedJob(int64(100+i), fmt.Sprintf("job-%d", i)))
	}
	return cs, js
}

func TestAPollReadsTheLogsOfTheFailedJobsAtOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	m := newMeeting("/repos/octo/hello/actions/jobs/9/logs", "/repos/octo/hello/actions/jobs/10/logs")
	fx.update(func() {
		fx.pr.CheckRuns = []ghfake.CheckRun{
			{ID: 1, Name: "build", Status: "completed", Conclusion: "failure"},
			{ID: 2, Name: "lint", Status: "completed", Conclusion: "failure"},
		}
	})
	fx.setJobs(stampedLog("##[group]Run go test ./...", "##[endgroup]", "##[error]Process completed with exit code 1."),
		failedJob(9, "build"), failedJob(10, "lint"))
	fx.api.Observe(func(a ghfake.Action) {
		if a.Route == ghfake.RouteJobLogs {
			m.arrive(a.Path)
		}
	})

	fx.poll(w)

	if missed := m.missed(); len(missed) != 0 {
		t.Fatalf("the logs were read one after another, missed: %v", missed)
	}
	msgs := fx.host.last().messages()
	if last := msgs[len(msgs)-1]; strings.Count(last, "Process completed with exit code 1.") != 2 {
		t.Fatalf("the message does not carry both logs:\n%s", last)
	}
}

func TestAPollStopsReadingTheLogsWhenTheRateLimitPauses(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.svc.guard = (&ghclient.RateGuard{Floor: 10}).Pausing(func() time.Time { return fx.clock() })
	w := fx.start()
	var logs atomic.Int64
	checkRuns, jobs := failedChecks(logReads + 2)
	var first []string
	for i := 1; i <= logReads; i++ {
		first = append(first, fmt.Sprintf("/repos/octo/hello/actions/jobs/%d/logs", 100+i))
	}
	onTheirWay := newMeeting(first...)
	fx.update(func() { fx.pr.CheckRuns = checkRuns })
	fx.setJobs(stampedLog("##[group]Run go test ./...", "##[endgroup]", "##[error]Process completed with exit code 1."), jobs...)
	fx.api.Observe(func(a ghfake.Action) {
		if a.Route != ghfake.RouteJobLogs {
			return
		}
		logs.Add(1)
		onTheirWay.arrive(a.Path)
		fx.api.SetRate(&ghfake.Rate{Limit: 5000, Remaining: 3, Reset: fx.clock().Add(time.Hour)})
	})

	fx.advance(time.Minute)
	err := fx.svc.Poll(context.Background(), w.ID)

	if n := logs.Load(); n != logReads {
		t.Fatalf("the poll read %d job logs, want the %d already on their way when the pause began", n, logReads)
	}
	if !errors.Is(err, ghclient.ErrPaused) {
		t.Fatalf("Poll() error = %v, want the pause of the rate limit", err)
	}
	if msgs := fx.host.last().messages(); len(msgs) != 1 {
		t.Fatalf("the paused poll told the agent about the checks: %q", msgs[1:])
	}
}

func TestTheIgnoredAuthorStaysNewForALaterWatch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	fx.svc = New(Deps{
		Log:           testutil.Logger(fx.t),
		Store:         fx.st,
		NewClient:     func(context.Context) (*github.Client, error) { return fx.client, nil },
		Git:           fx.git,
		Notifications: fx.notes,
		Host:          fx.host,
		DataDir:       fx.data,
	}, WithClock(func() time.Time { return fx.clock() }), WithInterval(time.Minute))
	w := fx.start()
	equal(t, fx.kinds(w), []string{"watch_started"})

	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "alice", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "note to self", URL: "https://c/11"}}
	})
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started"})

	seen, err := fx.st.SeenReviewItems(ctx, w.Key())
	if err != nil {
		t.Fatal(err)
	}
	if seen[store.SeenItem{Kind: store.KindIssueComment, ID: 11}] {
		t.Error("the comment the watch left out was recorded as seen")
	}

	fx.poll(w)
	stored, err := fx.st.GetWatch(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range stored.ReadyBlockers {
		if strings.Contains(b, "review item") {
			t.Errorf("the comment the watch left out blocks the merge: %q", b)
		}
	}

	if _, err := fx.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	req := fx.startRequest()
	req.IncludeOwn, req.IncludeExisting = new(true), new(true)
	again, err := fx.svc.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fx.update(func() {
		fx.pr.IssueComments = append(fx.pr.IssueComments, ghfake.Comment{ID: 12, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:02:00Z"), Body: "one more", URL: "https://c/12"})
	})
	fx.poll(again)
	equal(t, fx.kinds(again), []string{"watch_started", "comment", "comment"})
}

func TestStartWithIncludeExistingRecoversWhatAnEarlierWatchMarked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := newFixture(t)
	w := fx.start()
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "look here", URL: "https://c/11"}}
	})
	fx.poll(w)
	if got := fx.kinds(w); !slices.Contains(got, "comment") {
		t.Fatalf("kinds = %v, want the comment on the first watch", got)
	}

	if _, err := fx.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	req := fx.startRequest()
	req.IncludeExisting = new(true)
	again, err := fx.svc.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fx.poll(again)
	if got := fx.kinds(again); !slices.Contains(got, "comment") {
		t.Fatalf("kinds = %v, want the comment handed over again", got)
	}
}
