package supervisor

import (
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

func TestLostLinkStopsAfterGrace(t *testing.T) {
	ln, addr, err := Listen(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	lost := make(chan struct{}, 1)
	s := New(50*time.Millisecond, func() { lost <- struct{}{} }, testutil.Logger(t))
	ctx := t.Context()
	go s.Serve(ctx, ln)

	conn, err := dialLink(addr)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Eventually(t, func() bool { return s.Linked() == 1 }, "one linked client")

	conn.Close()
	select {
	case <-lost:
	case <-time.After(2 * time.Second):
		t.Fatal("onLost did not fire after the link dropped")
	}
}

func TestReconnectInsideGraceKeepsDaemon(t *testing.T) {
	ln, addr, err := Listen(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	lost := make(chan struct{}, 1)
	s := New(300*time.Millisecond, func() { lost <- struct{}{} }, testutil.Logger(t))
	ctx := t.Context()
	go s.Serve(ctx, ln)

	first, err := dialLink(addr)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Eventually(t, func() bool { return s.Linked() == 1 }, "one linked client")
	first.Close()
	testutil.Eventually(t, func() bool { return s.Linked() == 0 }, "no linked client")

	second, err := dialLink(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	testutil.Eventually(t, func() bool { return s.Linked() == 1 }, "one linked client")

	select {
	case <-lost:
		t.Fatal("onLost fired although a client reconnected inside the grace period")
	case <-time.After(500 * time.Millisecond):
	}
}

func (s *Supervisor) Linked() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.linked
}
