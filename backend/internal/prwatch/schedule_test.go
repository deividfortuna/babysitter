package prwatch

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

type scheduleClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *scheduleClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *scheduleClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestSchedule() (*schedule, *scheduleClock) {
	clock := &scheduleClock{now: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	return newSchedule(clock.read), clock
}

var testCadence = cadence{shortest: time.Minute, longest: 5 * time.Minute}

func dueAfter(t *testing.T, sc *schedule, clock *scheduleClock, id int64, c cadence) time.Duration {
	t.Helper()
	var waited time.Duration
	for !sc.due(id, c) {
		if waited > c.longest {
			t.Fatalf("watch %d is not due after %s, longer than the longest interval %s", id, waited, c.longest)
		}
		clock.advance(time.Minute)
		waited += time.Minute
	}
	return waited
}

func TestAWatchTheScheduleHasNotSeenIsDue(t *testing.T) {
	t.Parallel()
	sc, _ := newTestSchedule()

	if !sc.due(1, testCadence) {
		t.Fatal("a watch without a poll is not due, want it polled at once")
	}
}

func TestEachQuietPollDoublesTheWaitUpToTheLongestInterval(t *testing.T) {
	t.Parallel()
	sc, clock := newTestSchedule()

	var got []time.Duration
	for range 4 {
		sc.polled(1, slowDown, testCadence)
		got = append(got, dueAfter(t, sc, clock, 1, testCadence))
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
	sc, clock := newTestSchedule()
	for range 3 {
		sc.polled(1, slowDown, testCadence)
	}

	sc.polled(1, speedUp, testCadence)

	if got := dueAfter(t, sc, clock, 1, testCadence); got != time.Minute {
		t.Fatalf("wait after activity = %s, want the shortest interval, 1m0s", got)
	}
}

func TestAFailedPollKeepsTheWait(t *testing.T) {
	t.Parallel()
	sc, clock := newTestSchedule()
	sc.polled(1, slowDown, testCadence)
	sc.polled(1, slowDown, testCadence)

	sc.polled(1, keepPace, testCadence)

	if got := dueAfter(t, sc, clock, 1, testCadence); got != 4*time.Minute {
		t.Fatalf("wait after a failed poll = %s, want the wait it had, 4m0s", got)
	}
}

func TestAFixedCadenceNeverSlowsDown(t *testing.T) {
	t.Parallel()
	sc, clock := newTestSchedule()
	fixed := cadence{shortest: time.Minute, longest: time.Minute}

	for range 3 {
		sc.polled(1, slowDown, fixed)
		if got := dueAfter(t, sc, clock, 1, fixed); got != time.Minute {
			t.Fatalf("wait = %s, want the one interval, 1m0s", got)
		}
	}
}

func TestAWakeMakesOnlyThatWatchDue(t *testing.T) {
	t.Parallel()
	sc, _ := newTestSchedule()
	sc.polled(1, slowDown, testCadence)
	sc.polled(2, slowDown, testCadence)

	sc.wake(2)

	if sc.due(1, testCadence) {
		t.Fatal("watch 1 is due, want only the watch that woke")
	}
	if !sc.due(2, testCadence) {
		t.Fatal("watch 2 is not due after the wake")
	}
}

func TestANewCadenceStartsEveryWatchAtTheShortestInterval(t *testing.T) {
	t.Parallel()
	sc, clock := newTestSchedule()
	for range 3 {
		sc.polled(1, slowDown, testCadence)
	}

	sc.restart()

	if got := dueAfter(t, sc, clock, 1, testCadence); got != time.Minute {
		t.Fatalf("wait after a new cadence = %s, want the shortest interval, 1m0s", got)
	}
}

func TestTheScheduleForgetsTheWatchesThatStopped(t *testing.T) {
	t.Parallel()
	sc, _ := newTestSchedule()
	sc.polled(1, slowDown, testCadence)
	sc.polled(2, slowDown, testCadence)

	sc.keep(map[int64]bool{2: true})

	if !sc.due(1, testCadence) {
		t.Fatal("the schedule still holds the wait of watch 1, which stopped")
	}
	if sc.due(2, testCadence) {
		t.Fatal("the schedule lost the wait of watch 2, which still runs")
	}
}

func TestTheNextPassWaitsForTheEarliestWatchAndNeverLongerThanTheShortestInterval(t *testing.T) {
	t.Parallel()
	sc, clock := newTestSchedule()
	if got := sc.untilNext(testCadence); got != time.Minute {
		t.Fatalf("wait without watches = %s, want the shortest interval so a new watch is seen", got)
	}

	sc.polled(1, speedUp, testCadence)
	clock.advance(40 * time.Second)
	if got := sc.untilNext(testCadence); got != 20*time.Second {
		t.Fatalf("wait = %s, want the 20s until watch 1 is due", got)
	}

	sc.wake(1)
	if got := sc.untilNext(testCadence); got != 0 {
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

func TestAQuietWatchWaitsLongerBeforeTheNextPoll(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.svc.schedule.now = fx.clock
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)

	fx.advance(time.Minute)
	if fx.svc.schedule.due(w.ID, fx.svc.cadence()) {
		t.Fatal("a watch where nothing happened is due after the shortest interval, want a longer wait")
	}
	fx.advance(time.Minute)
	if !fx.svc.schedule.due(w.ID, fx.svc.cadence()) {
		t.Fatal("a quiet watch is not due after twice the shortest interval")
	}
}

func TestAWorkingAgentKeepsTheWatchAtTheShortestInterval(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.svc.schedule.now = fx.clock
	w := fx.start()
	fx.poll(w)
	fx.poll(w)

	fx.advance(time.Minute)
	if !fx.svc.schedule.due(w.ID, fx.svc.cadence()) {
		t.Fatal("a watch whose agent works is not due after the shortest interval")
	}
}

func TestAWatchWithRunningChecksStaysAtTheShortestInterval(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.svc.schedule.now = fx.clock
	w := fx.start()
	fx.agentIdle(w)
	fx.update(func() { fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 2, Name: "build", Status: "in_progress"}} })
	fx.poll(w)
	fx.poll(w)

	fx.advance(time.Minute)
	if !fx.svc.schedule.due(w.ID, fx.svc.cadence()) {
		t.Fatal("a watch with a check that runs is not due after the shortest interval")
	}
}
