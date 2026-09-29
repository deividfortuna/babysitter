package prwatch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

var scheduleStart = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

var testCadence = cadence{shortest: time.Minute, longest: 5 * time.Minute}

func dueAfter(t *testing.T, sc *schedule, id int64, c cadence) time.Duration {
	t.Helper()
	for waited := time.Duration(0); waited <= c.longest; waited += time.Minute {
		if sc.due(id, scheduleStart.Add(waited), c) {
			return waited
		}
	}
	t.Fatalf("watch %d is not due within the longest interval %s", id, c.longest)
	return 0
}

func TestAWatchTheScheduleHasNotSeenIsDue(t *testing.T) {
	t.Parallel()
	sc := newSchedule()

	if !sc.due(1, scheduleStart, testCadence) {
		t.Fatal("a watch without a poll is not due, want it polled at once")
	}
}

func TestEachQuietPollDoublesTheWaitUpToTheLongestInterval(t *testing.T) {
	t.Parallel()
	sc := newSchedule()

	var got []time.Duration
	for range 4 {
		sc.polled(1, scheduleStart, slowDown, testCadence)
		got = append(got, dueAfter(t, sc, 1, testCadence))
	}

	want := []time.Duration{2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("waits = %v, want %v", got, want)
		}
	}
}

func TestActivityBringsTheWaitBackToTheShortestInterval(t *testing.T) {
	t.Parallel()
	sc := newSchedule()
	for range 3 {
		sc.polled(1, scheduleStart, slowDown, testCadence)
	}

	sc.polled(1, scheduleStart, speedUp, testCadence)

	if got := dueAfter(t, sc, 1, testCadence); got != time.Minute {
		t.Fatalf("wait after activity = %s, want the shortest interval, 1m0s", got)
	}
}

func TestAFailedPollKeepsTheWait(t *testing.T) {
	t.Parallel()
	sc := newSchedule()
	sc.polled(1, scheduleStart, slowDown, testCadence)
	sc.polled(1, scheduleStart, slowDown, testCadence)

	sc.polled(1, scheduleStart, keepPace, testCadence)

	if got := dueAfter(t, sc, 1, testCadence); got != 4*time.Minute {
		t.Fatalf("wait after a failed poll = %s, want the wait it had, 4m0s", got)
	}
}

func TestAFixedCadenceNeverSlowsDown(t *testing.T) {
	t.Parallel()
	sc := newSchedule()
	fixed := cadence{shortest: time.Minute, longest: time.Minute}

	for range 3 {
		sc.polled(1, scheduleStart, slowDown, fixed)
		if got := dueAfter(t, sc, 1, fixed); got != time.Minute {
			t.Fatalf("wait = %s, want the one interval, 1m0s", got)
		}
	}
}

func TestARowBetweenPollsBringsASlowWatchBackToTheShortestInterval(t *testing.T) {
	t.Parallel()
	sc := newSchedule()
	for range 3 {
		sc.polled(1, scheduleStart, slowDown, testCadence)
	}

	sc.stir(1)
	sc.calm(1)

	if got := dueAfter(t, sc, 1, testCadence); got != time.Minute {
		t.Fatalf("wait after a row between polls = %s, want the shortest interval, 1m0s", got)
	}
}

func TestAWakeMakesOnlyThatWatchDue(t *testing.T) {
	t.Parallel()
	sc := newSchedule()
	sc.polled(1, scheduleStart, slowDown, testCadence)
	sc.polled(2, scheduleStart, slowDown, testCadence)

	sc.wake(2)

	if sc.due(1, scheduleStart, testCadence) {
		t.Fatal("watch 1 is due, want only the watch that woke")
	}
	if !sc.due(2, scheduleStart, testCadence) {
		t.Fatal("watch 2 is not due after the wake")
	}
}

func TestANewCadenceStartsEveryWatchAtTheShortestInterval(t *testing.T) {
	t.Parallel()
	sc := newSchedule()
	for range 3 {
		sc.polled(1, scheduleStart, slowDown, testCadence)
	}

	sc.restart()

	if got := dueAfter(t, sc, 1, testCadence); got != time.Minute {
		t.Fatalf("wait after a new cadence = %s, want the shortest interval, 1m0s", got)
	}
}

func TestTheScheduleForgetsTheWatchesThatStopped(t *testing.T) {
	t.Parallel()
	sc := newSchedule()
	sc.polled(1, scheduleStart, slowDown, testCadence)
	sc.polled(2, scheduleStart, slowDown, testCadence)

	sc.keep(map[int64]bool{2: true})

	if !sc.due(1, scheduleStart, testCadence) {
		t.Fatal("the schedule still holds the wait of watch 1, which stopped")
	}
	if sc.due(2, scheduleStart, testCadence) {
		t.Fatal("the schedule lost the wait of watch 2, which still runs")
	}
}

func TestTheNextPassWaitsForTheEarliestWatchAndNeverLongerThanTheShortestInterval(t *testing.T) {
	t.Parallel()
	sc := newSchedule()
	if got := sc.untilNext(scheduleStart, testCadence); got != time.Minute {
		t.Fatalf("wait without watches = %s, want the shortest interval so a new watch is seen", got)
	}

	sc.polled(1, scheduleStart, speedUp, testCadence)
	later := scheduleStart.Add(40 * time.Second)
	if got := sc.untilNext(later, testCadence); got != 20*time.Second {
		t.Fatalf("wait = %s, want the 20s until watch 1 is due", got)
	}

	sc.wake(1)
	if got := sc.untilNext(later, testCadence); got != 0 {
		t.Fatalf("wait = %s, want none for a watch that woke", got)
	}
}

func TestAKickPollsOnlyTheWatchItNames(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	three := fx.start()
	four := fx.startNumber(4)
	fx.svc.pass(context.Background())

	var mu sync.Mutex
	polled := map[string]int{}
	fx.api.Observe(func(a ghfake.Action) {
		if a.Route != ghfake.RoutePull {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		polled[prNumberOf(a.Path)]++
	})

	fx.svc.Kick(four.ID)
	fx.svc.pass(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if polled["4"] != 1 || polled["3"] != 0 {
		t.Fatalf("polls after a kick of watch %d = %v, want one of #4 and none of #3 (watch %d)", four.ID, polled, three.ID)
	}
}

func (fx *fixture) due(w store.Watch) bool {
	fx.t.Helper()
	return fx.svc.schedule.due(w.ID, fx.clock(), fx.svc.cadence())
}

func TestAQuietWatchWaitsLongerBeforeTheNextPoll(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)

	fx.advance(time.Minute)
	if fx.due(w) {
		t.Fatal("a watch where nothing happened is due after the shortest interval, want a longer wait")
	}
	fx.advance(time.Minute)
	if !fx.due(w) {
		t.Fatal("a quiet watch is not due after twice the shortest interval")
	}
}

func TestAWatchThatIsNotQuietStaysAtTheShortestInterval(t *testing.T) {
	t.Parallel()
	cases := map[string]func(fx *fixture, w store.Watch){
		"the agent works": func(*fixture, store.Watch) {},
		"a check runs": func(fx *fixture, w store.Watch) {
			fx.agentIdle(w)
			fx.update(func() { fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 2, Name: "build", Status: "in_progress"}} })
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			w := fx.start()
			arrange(fx, w)
			fx.poll(w)
			fx.poll(w)

			fx.advance(time.Minute)
			if !fx.due(w) {
				t.Fatal("the watch is not due after the shortest interval")
			}
		})
	}
}

func (sc *schedule) waitOf(id int64) time.Duration {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.slots[id].wait
}

func TestARowThePollRecordsAfterTheDiffKeepsTheWatchAtTheShortestInterval(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)

	fx.poll(w)
	if kinds := fx.kinds(w); kinds[len(kinds)-1] != "merge_ready" {
		t.Fatalf("activity = %v, want the second poll to record merge_ready", kinds)
	}

	fx.advance(time.Minute)
	if !fx.due(w) {
		t.Fatal("a watch whose poll recorded merge_ready is not due after the shortest interval")
	}
}

func TestTheIntervalsOfTheStartDoNotResetTheWaitOfTheFirstPass(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.agentIdle(w)

	go func() { _ = fx.svc.Run(t.Context()) }()

	quiet := 2 * time.Minute
	testutil.Eventually(t, func() bool { return fx.svc.schedule.waitOf(w.ID) == quiet }, "the quiet first pass to double the wait")
	if testutil.Within(100*time.Millisecond, func() bool { return fx.svc.schedule.waitOf(w.ID) != quiet }) {
		t.Fatalf("wait = %s after the start, want the %s of the quiet first pass", fx.svc.schedule.waitOf(w.ID), quiet)
	}
}

func TestSettingsWithTheSameIntervalsKeepTheWaits(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.agentIdle(w)
	go func() { _ = fx.svc.Run(t.Context()) }()
	quiet := 2 * time.Minute
	testutil.Eventually(t, func() bool { return fx.svc.schedule.waitOf(w.ID) == quiet }, "the quiet first pass to double the wait")

	fx.svc.SetInterval(fx.svc.Interval())
	fx.svc.SetMaxInterval(fx.svc.MaxInterval())

	if testutil.Within(100*time.Millisecond, func() bool { return fx.svc.schedule.waitOf(w.ID) != quiet }) {
		t.Fatalf("wait = %s after the same intervals again, want %s", fx.svc.schedule.waitOf(w.ID), quiet)
	}
}

func TestAPassThatCannotReachGitHubWaitsOneIntervalBeforeItTriesAgain(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	WithInterval(time.Hour)(fx.svc)
	fx.start()
	var tries atomic.Int32
	fx.svc.newClient = func(context.Context) (*github.Client, error) {
		tries.Add(1)
		return nil, errors.New("no token")
	}

	go func() { _ = fx.svc.Run(t.Context()) }()

	if testutil.Within(200*time.Millisecond, func() bool { return tries.Load() > 3 }) {
		t.Fatalf("the loop made %d GitHub clients at once, want it to wait one interval after a failed pass", tries.Load())
	}
}

func TestAMessageToASelfAgentKeepsTheWatchAtTheShortestInterval(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.svc.schedule.now = fx.clock
	w := fx.startSelf()
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "please add a test", URL: "https://c/11"}}
	})
	fx.poll(w)

	out, err := fx.svc.Next(context.Background(), w.ID, 0)
	if err != nil || out.Message == nil {
		t.Fatalf("Next() = %+v, %v, want the comment as a message", out, err)
	}

	fx.advance(time.Minute)
	if !fx.svc.schedule.due(w.ID, fx.svc.cadence()) {
		t.Fatal("a watch whose self agent took a message is not due after the shortest interval")
	}
}
