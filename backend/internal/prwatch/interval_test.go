package prwatch

import (
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

func TestSetIntervalRetunesARunningService(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	WithInterval(time.Hour)(fx.svc)
	fx.start()

	go func() { _ = fx.svc.Run(t.Context()) }()

	before := testutil.Settle(t, fx.api.Total, "the requests to GitHub")
	fx.svc.SetInterval(50 * time.Millisecond)

	testutil.Eventually(t, func() bool { return fx.api.Total() > before }, "a poll on the interval the setting changed")
}

func TestSetIntervalIgnoresAnIntervalTheDaemonCannotRun(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	WithInterval(time.Minute)(fx.svc)

	fx.svc.SetInterval(0)

	if got := fx.svc.Interval(); got != time.Minute {
		t.Fatalf("Interval() = %s, want the interval it had", got)
	}
}
