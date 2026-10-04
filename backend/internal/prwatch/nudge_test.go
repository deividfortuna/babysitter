package prwatch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

func TestAFailedCheckTakesTheJobOfItsNameInItsRun(t *testing.T) {
	t.Parallel()
	build77 := snapshot.FailedJob{RunID: 77, JobID: 9, JobName: "build", LogsEndpoint: "repos/octo/hello/actions/jobs/9/logs"}
	test77 := snapshot.FailedJob{RunID: 77, JobID: 12, JobName: "test", LogsEndpoint: "repos/octo/hello/actions/jobs/12/logs"}
	lint88 := snapshot.FailedJob{RunID: 88, JobID: 21, JobName: "lint", LogsEndpoint: "repos/octo/hello/actions/jobs/21/logs"}
	build90 := snapshot.FailedJob{RunID: 90, JobID: 30, JobName: "build", LogsEndpoint: "repos/octo/hello/actions/jobs/30/logs"}
	cases := []struct {
		name  string
		jobs  []snapshot.FailedJob
		check string
		runID int64
		want  snapshot.FailedJob
		found bool
	}{
		{name: "the first job of the run has its name", jobs: []snapshot.FailedJob{build77, test77}, check: "build", runID: 77, want: build77, found: true},
		{name: "a later job of the run has its name", jobs: []snapshot.FailedJob{build77, test77}, check: "test", runID: 77, want: test77, found: true},
		{name: "no job of the run has its name", jobs: []snapshot.FailedJob{lint88}, check: "CI", runID: 88, want: lint88, found: true},
		{name: "another run has a job of the same name", jobs: []snapshot.FailedJob{build77, build90}, check: "build", runID: 90, want: build90, found: true},
		{name: "no job of its run and none of its name", jobs: []snapshot.FailedJob{build77}, check: "lint", runID: 88},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, ok := jobOf(c.jobs, c.check, c.runID)
			if got != c.want || ok != c.found {
				t.Fatalf("jobOf(%q, run %d) = job %d of run %d, %v, want job %d of run %d, %v", c.check, c.runID, got.JobID, got.RunID, ok, c.want.JobID, c.want.RunID, c.found)
			}
		})
	}
}

func TestAFailedCheckTakesTheJobOfALaterSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := newFixture(t)
	fx.update(func() {
		fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 1, Name: "build", Status: "completed", Conclusion: "failure", CheckSuiteID: 501}}
	})
	w := fx.start()
	if err := fx.st.UnmarkActivityNudged(ctx, ids(fx.activity(w))); err != nil {
		t.Fatal(err)
	}

	fx.setJobs("##[error]Process completed with exit code 1.", failedJob(9, "build"))
	fx.svc = fx.newService()
	fx.poll(w)

	var check agent.Item
	for _, it := range toItems(fx.activity(w)) {
		if it.Kind == store.ActivityCheckFailed {
			check = it
		}
	}
	if check.JobID != 9 || check.JobName != "build" {
		t.Fatalf("the row of the failed check carries no job: %+v", check)
	}
}

// failingBuild fails the build in the job jobID of the CI run.
func (fx *fixture) failingBuild(jobID int64) {
	failed := check(1, "build", "failure")
	failed.URL = fmt.Sprintf("https://ci/77/%d", jobID)
	fx.update(func() { fx.pr.CheckRuns = []ghfake.CheckRun{failed} })
	fx.setJobs("##[error]Process completed with exit code 1.", failedJob(jobID, "build"))
}

func checkFailedRow(t *testing.T, fx *fixture, w store.Watch) store.Activity {
	t.Helper()
	for _, a := range fx.activity(w) {
		if a.Kind == store.ActivityCheckFailed {
			return a
		}
	}
	t.Fatalf("the watch has no check_failed row: %v", fx.kinds(w))
	return store.Activity{}
}

func TestARerunOnOneHeadRelinksTheRowAndPublishes(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startSelf()
	fx.failingBuild(9)
	fx.poll(w)
	if got := checkFailedRow(t, fx, w); got.URL != "https://ci/77/9" {
		t.Fatalf("the first row links %s, want the job of the run that failed", got.URL)
	}

	pub := &countingPublisher{}
	fx.st.SetPublisher(pub)
	fx.failingBuild(10)
	fx.poll(w)

	got := checkFailedRow(t, fx, w)
	if !strings.Contains(string(got.Payload), `"job_id":10`) {
		t.Fatalf("payload = %s, want the job of the attempt that failed last", got.Payload)
	}
	if got.URL != "https://ci/77/10" {
		t.Fatalf("the row links %s, want the page of job 10: the app sends the user to the log of the first attempt", got.URL)
	}
	if n := pub.count(events.WatchActivity); n == 0 {
		t.Fatalf("the poll rewrote a row and published no %s: an open app keeps the row it read before", events.WatchActivity)
	}
}

type countingPublisher struct {
	mu   sync.Mutex
	sent []events.Type
}

func (p *countingPublisher) Publish(t events.Type, repo string, number int) events.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, t)
	return events.Event{Type: t, Repo: repo, Number: number}
}

func (p *countingPublisher) count(t events.Type) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, got := range p.sent {
		if got == t {
			n++
		}
	}
	return n
}
