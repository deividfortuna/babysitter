// Package testutil holds the helpers that tests of several packages share.
// Production code never imports it.
package testutil

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// Timeout bounds every wait in Eventually. It is long so a slow CI runner
// does not fail a test that would pass; a passing test returns as soon as
// the condition holds.
const Timeout = 10 * time.Second

const pollEvery = 5 * time.Millisecond

// Eventually fails the test when cond does not hold within Timeout. The
// message names what the test waited for, as in "waited 10s for <what>".
func Eventually(t testing.TB, cond func() bool, what string, args ...any) {
	t.Helper()
	if !Within(Timeout, cond) {
		t.Fatalf("waited %s for "+what, append([]any{Timeout}, args...)...)
	}
}

// Within reports whether cond holds within d, for tests that report the
// failure with state of their own.
func Within(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(pollEvery)
	}
}

// Settle waits until n returns the same value twice across a quiet period
// and returns that value, for a test that must let background work finish
// before it counts. It fails the test when n keeps changing past Timeout.
func Settle(t testing.TB, n func() int, what string, args ...any) int {
	t.Helper()
	const quiet = 20 * time.Millisecond
	deadline := time.Now().Add(Timeout)
	last := n()
	for {
		time.Sleep(quiet)
		now := n()
		if now == last {
			return now
		}
		if time.Now().After(deadline) {
			t.Fatalf("waited %s for "+what+" to settle", append([]any{Timeout}, args...)...)
		}
		last = now
	}
}

// Logger returns a logger that writes to the output of t, so the logs of
// the code under test show only when the test fails or runs with -v. Lines
// logged after the test ends, by a goroutine that outlives it, are dropped:
// the testing package panics on output from a finished test.
func Logger(t testing.TB) *slog.Logger {
	w := &testWriter{out: t.Output()}
	t.Cleanup(w.close)
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

type testWriter struct {
	mu     sync.Mutex
	out    io.Writer
	closed bool
}

func (w *testWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return len(p), nil
	}
	return w.out.Write(p)
}

func (w *testWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
}
