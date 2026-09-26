//go:build !windows

package supervisor

import (
	"net"
	"os"
	"path/filepath"
	"strings"
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

	conn, err := net.Dial("unix", addr)
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

	first, err := net.Dial("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Eventually(t, func() bool { return s.Linked() == 1 }, "one linked client")
	first.Close()
	testutil.Eventually(t, func() bool { return s.Linked() == 0 }, "no linked client")

	second, err := net.Dial("unix", addr)
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

func TestListenFallbackIsUniquePerListener(t *testing.T) {
	first, firstAddr, err := Listen(t.Context(), longDataDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Close() })

	second, secondAddr, err := Listen(t.Context(), longDataDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.Close() })

	if firstAddr == secondAddr {
		t.Fatalf("both listeners took %s, so one unlinks the socket of the other", firstAddr)
	}
	conn, err := net.Dial("unix", firstAddr)
	if err != nil {
		t.Fatalf("the first socket is gone: %v", err)
	}
	conn.Close()
}

func (s *Supervisor) Linked() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.linked
}

func longDataDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), strings.Repeat("d", 80))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
