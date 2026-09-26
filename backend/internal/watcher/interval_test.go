package watcher

import (
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

func TestSetIntervalRetunesARunningWatcher(t *testing.T) {
	fx := newFixture(t)
	WithInterval(time.Hour)(fx.w)

	go func() { _ = fx.w.Run(t.Context()) }()

	testutil.Eventually(t, func() bool { return fx.gh.CountPath("/repos/o/r/pulls") >= 1 }, "the first pass")
	fx.w.SetInterval(time.Millisecond)

	testutil.Eventually(t, func() bool { return fx.gh.CountPath("/repos/o/r/pulls") >= 2 }, "a pass on the new interval")
}

func TestSetIntervalIgnoresAnIntervalTheDaemonCannotRun(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	WithInterval(time.Minute)(fx.w)

	fx.w.SetInterval(0)

	if got := fx.w.Interval(); got != time.Minute {
		t.Fatalf("Interval() = %s, want the interval it had", got)
	}
}
