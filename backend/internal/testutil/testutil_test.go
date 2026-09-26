package testutil

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestWithinReturnsOnceTheConditionHolds(t *testing.T) {
	t.Parallel()
	var n atomic.Int32
	if !Within(time.Second, func() bool { return n.Add(1) >= 3 }) {
		t.Fatal("Within() = false, want true on the third call")
	}
}

func TestWithinGivesUpAfterTheDeadline(t *testing.T) {
	t.Parallel()
	start := time.Now()
	if Within(20*time.Millisecond, func() bool { return false }) {
		t.Fatal("Within() = true for a condition that never holds")
	}
	if waited := time.Since(start); waited < 20*time.Millisecond {
		t.Fatalf("Within() returned after %s, want at least 20ms", waited)
	}
}

func TestLoggerDropsLinesAfterTheTestEnds(t *testing.T) {
	t.Parallel()
	var late func()
	t.Run("inner", func(t *testing.T) {
		log := Logger(t)
		log.Info("inside the test")
		late = func() { log.Info("after the test") }
	})
	late()
}

func TestSettleReturnsTheValueOnceItStopsChanging(t *testing.T) {
	t.Parallel()
	var n atomic.Int32
	if got := Settle(t, func() int {
		if v := n.Load(); v < 3 {
			return int(n.Add(1))
		}
		return 3
	}, "the counter"); got != 3 {
		t.Fatalf("Settle() = %d, want 3", got)
	}
}
