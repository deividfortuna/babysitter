package prwatch

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func (fx *fixture) checkout(branch string) {
	fx.t.Helper()
	cmd := exec.Command("git", "checkout", "-q", "-B", branch)
	cmd.Dir = fx.dir
	if out, err := cmd.CombinedOutput(); err != nil {
		fx.t.Fatalf("git checkout: %v\n%s", err, out)
	}
}

func (fx *fixture) startSelf() store.Watch {
	fx.t.Helper()
	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir, Provider: ProviderSelf,
	})
	if err != nil {
		fx.t.Fatalf("Start() error = %v", err)
	}
	return w
}

func TestSelfWatchHasNoWorktreeAndNoSession(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startSelf()
	if w.Provider != ProviderSelf || w.WorktreeDir != "" || w.WorkBranch != "" || w.SourceDir != fx.dir {
		t.Fatalf("watch = %+v", w)
	}
	if len(fx.git.createdDirs()) != 0 || fx.host.count() != 0 {
		t.Fatalf("a self watch made a worktree (%v) or opened a session (%d)", fx.git.createdDirs(), fx.host.count())
	}
	equal(t, fx.kinds(w), []string{"watch_started"})
	if info, _ := fx.svc.Session(context.Background(), w); info.State != agent.StateNone {
		t.Fatalf("session = %+v", info)
	}
	if _, err := fx.svc.Send(context.Background(), w.ID, "hi"); !errors.Is(err, ErrSelfWatch) {
		t.Fatalf("Send() error = %v, want ErrSelfWatch", err)
	}
	if _, err := fx.svc.Output(context.Background(), w.ID, 10); !errors.Is(err, ErrSelfWatch) {
		t.Fatalf("Output() error = %v, want ErrSelfWatch", err)
	}
	stopped, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{})
	if err != nil || stopped.Status != store.WatchStopped {
		t.Fatalf("Stop() = %+v, %v", stopped, err)
	}
	if len(fx.git.removedDirs()) != 0 {
		t.Fatalf("the stop removed a worktree: %v", fx.git.removedDirs())
	}
}

func TestSelfWatchRejections(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	target := snapshot.Target{Owner: "octo", Name: "hello", Number: 3}
	if _, err := fx.svc.Start(ctx, StartRequest{Target: target, SourceDir: fx.dir, Provider: ProviderSelf, Model: "sonnet"}); !errors.Is(err, ErrBadModel) {
		t.Fatalf("Start() with a model error = %v, want ErrBadModel", err)
	}
	fx.checkout("other")
	if _, err := fx.svc.Start(ctx, StartRequest{Target: target, SourceDir: fx.dir, Provider: ProviderSelf}); !errors.Is(err, ErrWrongBranch) {
		t.Fatalf("Start() off the head branch error = %v, want ErrWrongBranch", err)
	}
	if ws, _ := fx.st.ListWatches(ctx, store.ListWatchesOptions{}); len(ws) != 0 {
		t.Fatalf("watches after rejections = %v", ws)
	}
	fx.checkout("fix")
	fx.svc.agents = map[string]agent.Runner{}
	if _, err := fx.svc.Start(ctx, StartRequest{Target: target, SourceDir: fx.dir, Provider: ProviderSelf}); err != nil {
		t.Fatalf("Start() without agents error = %v", err)
	}
}

func TestNextHandsOutEachMessageOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startSelf()
	ctx := context.Background()

	out, err := fx.svc.Next(ctx, w.ID, 0)
	if err != nil || out.Message != nil || out.Watch.ID != w.ID {
		t.Fatalf("Next() = %+v, %v", out, err)
	}

	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "Please return the error", URL: "https://c/11"}}
	})
	fx.failBuild(stampedLog("##[group]Run go test ./...", "##[endgroup]", "--- FAIL: TestThing", "##[error]Process completed with exit code 1."))
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "comment", "check_failed"})
	if msgs := fx.notes.messages(); len(msgs) != 2 || !strings.Contains(msgs[1], "build failed") {
		t.Fatalf("notifications = %v", msgs)
	}
	got, _ := fx.st.GetWatch(ctx, w.ID)
	if len(got.ReadyBlockers) == 0 || !strings.Contains(strings.Join(got.ReadyBlockers, "; "), "not told about 1 comment, 1 failed check yet") {
		t.Fatalf("blockers = %v", got.ReadyBlockers)
	}

	out, err = fx.svc.Next(ctx, w.ID, 0)
	if err != nil || out.Message == nil {
		t.Fatalf("Next() = %+v, %v", out, err)
	}
	text := string(out.Message.Payload)
	if !strings.Contains(text, "Please return the error") || !strings.Contains(text, "answer it with `--to 11`") ||
		!strings.Contains(text, "Failed: build (failure)") || !strings.Contains(text, "--- FAIL: TestThing") || !strings.Contains(text, "PR: https://github.com/octo/hello/pull/3") {
		t.Fatalf("message = %s", text)
	}
	if out.Message.Kind != store.ActivityNudged || out.Message.Summary != "told the agent about 1 comment, 1 failed check" {
		t.Fatalf("row = %+v", out.Message)
	}
	equal(t, fx.kinds(w), []string{"watch_started", "comment", "check_failed", "nudged"})
	rows := fx.activity(w)
	if rows[1].NudgedAt == nil || rows[2].NudgedAt == nil {
		t.Fatalf("rows = %+v", rows[1:3])
	}

	out, err = fx.svc.Next(ctx, w.ID, 0)
	if err != nil || out.Message != nil {
		t.Fatalf("second Next() = %+v, %v", out, err)
	}
	equal(t, fx.kinds(w), []string{"watch_started", "comment", "check_failed", "nudged"})
}

func TestNextPollsFirst(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startSelf()
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "Please return the error", URL: "https://c/11"}}
	})
	out, err := fx.svc.Next(context.Background(), w.ID, 0)
	if err != nil || out.Message == nil || !strings.Contains(string(out.Message.Payload), "Please return the error") {
		t.Fatalf("Next() = %+v, %v", out, err)
	}
}

func TestNextWaitsForActivityAndEndsEarly(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	bus := events.NewBus()
	fx.st.SetPublisher(bus)
	fx.svc = New(Deps{
		Log:           testutil.Logger(fx.t),
		Store:         fx.st,
		NewClient:     func(context.Context) (*github.Client, error) { return fx.client, nil },
		Git:           fx.git,
		Notifications: fx.notes,
		Host:          fx.host,
		DataDir:       fx.data,
		Bus:           bus,
	}, WithClock(func() time.Time { return fx.clock() }), WithInterval(time.Minute))
	w := fx.startSelf()
	ctx := context.Background()

	type answer struct {
		out NextMessage
		err error
	}
	done := make(chan answer, 1)
	go func() {
		out, err := fx.svc.Next(ctx, w.ID, 10*time.Second)
		done <- answer{out, err}
	}()
	select {
	case a := <-done:
		t.Fatalf("Next() answered at once: %+v, %v", a.out, a.err)
	case <-time.After(100 * time.Millisecond):
	}
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "Please return the error", URL: "https://c/11"}}
	})
	fx.poll(w)
	select {
	case a := <-done:
		if a.err != nil || a.out.Message == nil || !strings.Contains(string(a.out.Message.Payload), "Please return the error") {
			t.Fatalf("Next() = %+v, %v", a.out, a.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Next() did not wake on the comment")
	}

	go func() {
		out, err := fx.svc.Next(ctx, w.ID, 10*time.Second)
		done <- answer{out, err}
	}()
	fx.waitForNext(w.ID)
	fx.update(func() { fx.pr.State, fx.pr.Merged = "closed", true })
	fx.poll(w)
	select {
	case a := <-done:
		if a.err != nil || a.out.Message != nil || a.out.Watch.Status != store.WatchStopped || a.out.Watch.StopReason != store.StopMerged {
			t.Fatalf("Next() after the merge = %+v, %v", a.out, a.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Next() did not wake on the stop")
	}
	out, err := fx.svc.Next(ctx, w.ID, 10*time.Second)
	if err != nil || out.Message != nil || out.Watch.Status != store.WatchStopped {
		t.Fatalf("Next() on a stopped watch = %+v, %v", out, err)
	}
}

func TestNextEndsEarlyWhenReadyToMerge(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.startSelf()
	fx.poll(w)
	fx.poll(w)
	start := time.Now()
	out, err := fx.svc.Next(context.Background(), w.ID, 5*time.Second)
	if err != nil || out.Message != nil || out.Watch.ReadySince == nil {
		t.Fatalf("Next() = %+v, %v", out, err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("Next() waited on a pull request that is ready to merge")
	}
}

func TestNextRefusesAHostedWatch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	if _, err := fx.svc.Next(context.Background(), w.ID, 0); !errors.Is(err, ErrHostedWatch) {
		t.Fatalf("Next() error = %v, want ErrHostedWatch", err)
	}
}

func TestNextTakesTheJobOfAFailedCheckFromALaterSnapshot(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startSelf()
	var jobs []*ghfake.Job
	fx.failBuild(stampedLog("##[group]Run go test ./...", "##[endgroup]", "--- FAIL: TestThing", "##[error]Process completed with exit code 1."))
	fx.update(func() { jobs, fx.repo.Runs[0].Jobs = fx.repo.Runs[0].Jobs, nil })
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "check_failed"})

	fx.update(func() { fx.repo.Runs[0].Jobs = jobs })
	out, err := fx.svc.Next(context.Background(), w.ID, 0)
	if err != nil || out.Message == nil {
		t.Fatalf("Next() = %+v, %v", out, err)
	}
	text := string(out.Message.Payload)
	if !strings.Contains(text, "--- FAIL: TestThing") || !strings.Contains(text, "actions/jobs/9/logs") {
		t.Fatalf("the message lacks the log of the job the first snapshot did not list:\n%s", text)
	}
}

func TestNextTakesTheJobOfTheRunThatFailedLast(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startSelf()
	log := stampedLog("##[group]Run go test ./...", "##[endgroup]", "--- FAIL: TestThing", "##[error]Process completed with exit code 1.")
	fx.failBuild(log)
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "check_failed"})

	fx.setJobs(log, failedJob(10, "build"))
	out, err := fx.svc.Next(context.Background(), w.ID, 0)
	if err != nil || out.Message == nil {
		t.Fatalf("Next() = %+v, %v", out, err)
	}
	text := string(out.Message.Payload)
	if !strings.Contains(text, "actions/jobs/10/logs") || strings.Contains(text, "actions/jobs/9/logs") {
		t.Fatalf("the message names the job of the first run, not the one the last snapshot lists:\n%s", text)
	}
}

func TestASelfAgentIsBusyFromAMessageToItsNextCall(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.startSelf()
	ctx := context.Background()
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "please add a test", URL: "https://c/11"}}
	})
	out, err := fx.svc.Next(ctx, w.ID, 0)
	if err != nil || out.Message == nil {
		t.Fatalf("Next() = %+v, %v", out, err)
	}
	fx.poll(w)
	fx.poll(w)
	got := fx.watch(w)
	if got.ReadySince != nil || countKind(fx.activity(w), store.ActivityMergeReady) != 0 {
		t.Fatalf("the pull request was called ready while the agent works on the message: since = %v, kinds = %v", got.ReadySince, fx.kinds(w))
	}
	if info, _ := fx.svc.Session(ctx, got); info.State != agent.StateActive {
		t.Fatalf("session = %+v, want the agent working", info)
	}

	out, err = fx.svc.Next(ctx, w.ID, 0)
	if err != nil || out.Message != nil {
		t.Fatalf("second Next() = %+v, %v", out, err)
	}
	if info, _ := fx.svc.Session(ctx, got); info.State != agent.StateNone {
		t.Fatalf("session = %+v, want no agent state once it asked again", info)
	}
	fx.poll(w)
	fx.poll(w)
	if got := fx.watch(w); got.ReadySince == nil || countKind(fx.activity(w), store.ActivityMergeReady) != 1 {
		t.Fatalf("the pull request is not ready once the agent asked again: since = %v, kinds = %v", got.ReadySince, fx.kinds(w))
	}
}

func TestNextWakesWhenThePullRequestBecomesReady(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	bus := events.NewBus()
	fx.st.SetPublisher(bus)
	fx.svc = New(Deps{
		Log:           testutil.Logger(fx.t),
		Store:         fx.st,
		NewClient:     func(context.Context) (*github.Client, error) { return fx.client, nil },
		Git:           fx.git,
		Notifications: fx.notes,
		Host:          fx.host,
		DataDir:       fx.data,
		Bus:           bus,
	}, WithClock(func() time.Time { return fx.clock() }), WithInterval(time.Minute))
	fx.good()
	fx.update(func() { fx.pr.Draft = true })
	w := fx.startSelf()
	fx.poll(w)
	done := make(chan NextMessage, 1)
	go func() {
		out, _ := fx.svc.Next(context.Background(), w.ID, 10*time.Second)
		done <- out
	}()
	fx.waitForNext(w.ID)
	fx.update(func() { fx.pr.Draft = false })
	fx.poll(w)
	fx.poll(w)
	select {
	case out := <-done:
		if out.Message != nil || out.Watch.ReadySince == nil {
			t.Fatalf("Next() = %+v", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Next() did not wake when the pull request became ready")
	}
}

func TestASelfWatchOnADetachedHeadIsRejectedAsTheWrongBranch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	cmd := exec.Command("git", "checkout", "-q", "--detach")
	cmd.Dir = fx.dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout --detach: %v\n%s", err, out)
	}
	target := snapshot.Target{Owner: "octo", Name: "hello", Number: 3}
	_, err := fx.svc.Start(context.Background(), StartRequest{Target: target, SourceDir: fx.dir, Provider: ProviderSelf})
	if !errors.Is(err, ErrWrongBranch) {
		t.Fatalf("Start() on a detached head error = %v, want ErrWrongBranch", err)
	}
}

func (fx *fixture) busService(bus *events.Bus) {
	fx.t.Helper()
	fx.st.SetPublisher(bus)
	fx.svc = New(Deps{
		Log:           testutil.Logger(fx.t),
		Store:         fx.st,
		NewClient:     func(context.Context) (*github.Client, error) { return fx.client, nil },
		Git:           fx.git,
		Notifications: fx.notes,
		Host:          fx.host,
		DataDir:       fx.data,
		Bus:           bus,
	}, WithClock(func() time.Time { return fx.clock() }), WithInterval(time.Minute))
}

func TestNextTakesTheMessageBackWhenTheClientGaveUp(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	bus := events.NewBus()
	fx.busService(bus)
	w := fx.startSelf()
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "Please return the error", URL: "https://c/11"}}
	})
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "comment"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	unsubscribe := bus.Subscribe(func(e events.Event) {
		if e.Type == events.WatchActivity {
			cancel()
		}
	})
	out, err := fx.svc.Next(ctx, w.ID, 0)
	unsubscribe()
	if !errors.Is(err, context.Canceled) || out.Message != nil {
		t.Fatalf("Next() = %+v, %v, want the message taken back", out, err)
	}
	equal(t, fx.kinds(w), []string{"watch_started", "comment"})

	out, err = fx.svc.Next(context.Background(), w.ID, 0)
	if err != nil || out.Message == nil || !strings.Contains(string(out.Message.Payload), "Please return the error") {
		t.Fatalf("the comment nobody was told about is not handed out again: %+v, %v", out, err)
	}
}

func TestASelfAgentStopsBeingBusyAfterItsDeadline(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.startSelf()
	ctx := context.Background()
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "please add a test", URL: "https://c/11"}}
	})
	out, err := fx.svc.Next(ctx, w.ID, 0)
	if err != nil || out.Message == nil {
		t.Fatalf("Next() = %+v, %v", out, err)
	}

	fx.advance(selfWorkDeadline)
	if info, _ := fx.svc.Session(ctx, fx.watch(w)); info.State != agent.StateNone {
		t.Fatalf("session = %+v, want no agent state once the deadline passed", info)
	}
	fx.poll(w)
	fx.poll(w)
	if got := fx.watch(w); got.ReadySince == nil || countKind(fx.activity(w), store.ActivityMergeReady) != 1 {
		t.Fatalf("the pull request stays blocked by an agent that is gone: since = %v, kinds = %v", got.ReadySince, fx.kinds(w))
	}
}

func TestNextWaitsForTheDaemonToCallThePullRequestReady(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.busService(events.NewBus())
	fx.good()
	w := fx.startSelf()
	fx.poll(w)
	if got := fx.watch(w); got.ReadySince == nil || countKind(fx.activity(w), store.ActivityMergeReady) != 0 {
		t.Fatalf("after the first good poll: since = %v, kinds = %v", got.ReadySince, fx.kinds(w))
	}

	done := make(chan NextMessage, 1)
	go func() {
		out, _ := fx.svc.Next(context.Background(), w.ID, 10*time.Second)
		done <- out
	}()
	select {
	case out := <-done:
		t.Fatalf("Next() called the pull request ready one interval before the daemon did: %+v", out)
	case <-time.After(300 * time.Millisecond):
	}

	fx.poll(w)
	select {
	case out := <-done:
		if out.Message != nil || out.Watch.ReadySince == nil {
			t.Fatalf("Next() = %+v", out)
		}
		if countKind(fx.activity(w), store.ActivityMergeReady) != 1 {
			t.Fatalf("Next() answered ready without the merge_ready row: %v", fx.kinds(w))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Next() did not wake when the daemon called the pull request ready")
	}
}

func TestNextReportsTheRowsOfItsPollWhenTheCallerGivesUp(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startSelf()
	fx.failBuild("boom")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fx.notes.onSend = func() { cancel() }

	if _, err := fx.svc.Next(ctx, w.ID, 0); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Next() error = %v", err)
	}
	for _, a := range fx.activity(w) {
		if a.Kind == store.ActivityCheckFailed && !a.Reported {
			t.Fatalf("activity %d (%s) stayed unreported after the caller gave up", a.ID, a.Kind)
		}
	}
}

func TestReadinessCountsOnlyOnceItStoodAWholeInterval(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.startSelf()

	fx.poll(w)
	since, blockers := fx.svc.Readiness(fx.watch(w), agent.StateNone)
	if since != nil || countKind(fx.activity(w), store.ActivityMergeReady) != 0 {
		t.Fatalf("readiness before it settled: since = %v, blockers = %v, kinds = %v", since, blockers, fx.kinds(w))
	}

	fx.advance(2 * time.Minute)
	fx.poll(w)
	since, blockers = fx.svc.Readiness(fx.watch(w), agent.StateNone)
	if since == nil || countKind(fx.activity(w), store.ActivityMergeReady) != 1 {
		t.Fatalf("readiness hidden after it settled: since = %v, blockers = %v, kinds = %v", since, blockers, fx.kinds(w))
	}
}

func TestARestartHandsOutAMessageThatNeverReachedTheAgent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.startSelf()
	ctx := context.Background()
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "please add a test", URL: "https://c/11"}}
	})
	fx.poll(w)

	out, err := fx.svc.take(ctx, fx.client, w.ID)
	if err != nil || out.Message == nil {
		t.Fatalf("take() = %+v, %v", out, err)
	}

	fx.svc = fx.newService()
	fx.svc.recover(ctx)
	fx.svc.wg.Wait()

	fx.poll(w)
	if blockers := fx.watch(w).ReadyBlockers; !slices.Contains(blockers, "the agent was not told about 1 comment yet") {
		t.Fatalf("a message that never reached the agent does not block the merge: %v", blockers)
	}
	again, err := fx.svc.Next(ctx, w.ID, 0)
	if err != nil || again.Message == nil {
		t.Fatalf("the message never reached the agent and is gone: %+v, %v", again, err)
	}
	if !strings.Contains(string(again.Message.Payload), "please add a test") {
		t.Fatalf("the message lost the comment it carried:\n%s", again.Message.Payload)
	}
}

func TestARestartKeepsASelfAgentAtWorkOnItsMessage(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.startSelf()
	ctx := context.Background()
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "please add a test", URL: "https://c/11"}}
	})
	out, err := fx.svc.Next(ctx, w.ID, 0)
	if err != nil || out.Message == nil {
		t.Fatalf("Next() = %+v, %v", out, err)
	}

	fx.svc = fx.newService()
	fx.svc.recover(ctx)
	fx.svc.wg.Wait()

	if info, _ := fx.svc.Session(ctx, fx.watch(w)); info.State != agent.StateActive {
		t.Fatalf("after a restart the agent that holds a message is %q, want it still working", info.State)
	}
	if _, err := fx.svc.Merge(ctx, w.ID, MergeOptions{}); !errors.Is(err, ErrNotReady) || !strings.Contains(err.Error(), "the agent is still working") {
		t.Fatalf("Merge() error = %v, want ErrNotReady naming the working agent", err)
	}

	if _, err := fx.svc.Next(ctx, w.ID, 0); err != nil {
		t.Fatalf("second Next() error = %v", err)
	}
	if info, _ := fx.svc.Session(ctx, fx.watch(w)); info.State != agent.StateNone {
		t.Fatalf("session = %+v, want no agent state once it asked again", info)
	}
}

func TestNextRefusesASecondCallerWhileOneWaits(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	bus := events.NewBus()
	fx.st.SetPublisher(bus)
	fx.svc = New(Deps{
		Log:           testutil.Logger(fx.t),
		Store:         fx.st,
		NewClient:     func(context.Context) (*github.Client, error) { return fx.client, nil },
		Git:           fx.git,
		Notifications: fx.notes,
		Host:          fx.host,
		DataDir:       fx.data,
		Bus:           bus,
	}, WithClock(func() time.Time { return fx.clock() }), WithInterval(time.Minute))
	fx.good()
	w := fx.startSelf()
	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		_, err := fx.svc.Next(ctx, w.ID, 10*time.Second)
		done <- err
	}()
	fx.waitForNext(w.ID)

	if _, err := fx.svc.Next(ctx, w.ID, 0); !errors.Is(err, ErrNextInFlight) {
		t.Fatalf("the second Next() error = %v, want ErrNextInFlight", err)
	}

	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "please add a test", URL: "https://c/11"}}
	})
	if err := fx.svc.Poll(ctx, w.ID); err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the first Next() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the first Next() did not wake on the comment")
	}
	if info, _ := fx.svc.Session(ctx, fx.watch(w)); info.State != agent.StateActive {
		t.Fatalf("session = %+v, want the agent that took the message at work", info)
	}

	if _, err := fx.svc.Next(ctx, w.ID, 0); err != nil {
		t.Fatalf("Next() once the first caller left = %v", err)
	}
}

// waitForNext returns once a Next call on the watch is in flight and done
// with its first poll, so what the test changes next is what wakes it.
func (fx *fixture) waitForNext(id int64) {
	fx.t.Helper()
	testutil.Eventually(fx.t, func() bool { return fx.svc.waiting.inFlight(id) }, "Next() on watch %d", id)
	testutil.Settle(fx.t, fx.api.Total, "the poll of Next()")
}

func (r *waiting) inFlight(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.m[id]
	return ok
}
