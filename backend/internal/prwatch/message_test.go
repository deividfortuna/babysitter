package prwatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/session"
	"github.com/deividfortuna/babysitter/internal/store"
)

func TestSessionReadsNoWatchRowOfItsOwn(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	w.AgentSession = "held-by-the-caller"
	info, err := fx.svc.Session(context.Background(), w)
	if err != nil {
		t.Fatalf("Session() error = %v", err)
	}
	if info.PID == 0 {
		t.Fatalf("Session() = %+v, want the live session of the watch", info)
	}
	if info.AgentSession != w.AgentSession {
		t.Fatalf("agent session = %q, want %q from the watch the caller holds", info.AgentSession, w.AgentSession)
	}
}

func TestSendReturnsTheRowItRecordedPastTheActivityWindow(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	for i := range 250 {
		if _, _, err := fx.st.InsertActivity(context.Background(), store.Activity{
			WatchID: w.ID, Kind: store.ActivityHeartbeat, Ref: fmt.Sprintf("filler-%d", i), At: fx.clock(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	fx.advance(time.Minute)
	row, err := fx.svc.Send(context.Background(), w.ID, "look at the failing test")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if row.ID == 0 || row.Kind != store.ActivityNudged {
		t.Fatalf("Send() = %+v, want a recorded nudged row", row)
	}
	if row.Summary != "you told the agent: look at the failing test" {
		t.Fatalf("summary = %q", row.Summary)
	}
}

func TestOutputReadsOnPastARotationOfTheLog(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	ctx := context.Background()
	if _, err := fx.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(fx.data, "sessions", fmt.Sprintf("%d.log", w.ID))
	if err := os.MkdirAll(filepath.Dir(log), 0o750); err != nil {
		t.Fatal(err)
	}
	var older bytes.Buffer
	for i := range 100 {
		fmt.Fprintf(&older, "old line %d\n", i)
	}
	if err := os.WriteFile(log+session.RotatedSuffix, older.Bytes(), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, []byte("new line 0\nnew line 1\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	out, err := fx.svc.Output(ctx, w.ID, 5)
	if err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	want := "old line 97\nold line 98\nold line 99\nnew line 0\nnew line 1\n"
	if out != want {
		t.Fatalf("Output() = %q, want %q", out, want)
	}
}

func TestOutputReadsOnlyTheTailOfALargeLog(t *testing.T) {
	fx := newFixture(t)
	w := fx.start()
	ctx := context.Background()
	if _, err := fx.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(fx.data, "sessions", fmt.Sprintf("%d.log", w.ID))
	if err := os.MkdirAll(filepath.Dir(log), 0o750); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	for b.Len() < 8<<20 {
		b.WriteString("the agent of a long watch printed this\n")
	}
	b.WriteString("the last line\n")
	if err := os.WriteFile(log, b.Bytes(), 0o640); err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out, err := fx.svc.Output(ctx, w.ID, 1)
	runtime.ReadMemStats(&after)

	if err != nil || out != "the last line\n" {
		t.Fatalf("Output() = %q, %v", out, err)
	}
	if read := after.TotalAlloc - before.TotalAlloc; read > 1<<20 {
		t.Fatalf("Output() took %d bytes to read the end of a log of %d bytes", read, b.Len())
	}
}

func TestResizeGivesTheLiveSessionTheSize(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	size := TerminalSize{Rows: 48, Cols: 210}

	if err := fx.svc.Resize(context.Background(), w.ID, size); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}

	if got := fx.host.last().terminalSize(); got != size {
		t.Fatalf("terminal size = %+v, want %+v", got, size)
	}
}

func TestOverlappingResizesLeaveTheSessionAtTheKeptSize(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	ctx := context.Background()
	older, newer := TerminalSize{Rows: 30, Cols: 87}, TerminalSize{Rows: 60, Cols: 177}
	entered, release := make(chan struct{}), make(chan struct{})
	h := fx.host.last()
	h.mu.Lock()
	h.onResize = func(size TerminalSize) {
		if size == older {
			close(entered)
			<-release
		}
	}
	h.mu.Unlock()

	olderDone, newerDone := make(chan error, 1), make(chan error, 1)
	go func() { olderDone <- fx.svc.Resize(ctx, w.ID, older) }()
	<-entered
	go func() { newerDone <- fx.svc.Resize(ctx, w.ID, newer) }()
	select {
	case err := <-newerDone:
		newerDone <- err
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for _, done := range []chan error{olderDone, newerDone} {
		if err := <-done; err != nil {
			t.Fatalf("Resize() error = %v", err)
		}
	}

	if live, kept := h.terminalSize(), fx.svc.sizes.get(w.ID); live != kept {
		t.Fatalf("live session at %+v, kept size %+v, want the same size", live, kept)
	}
}

func TestAResizeDuringASessionStartReachesTheNewSession(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	ctx := context.Background()
	fx.host.last().exit(errors.New("exit status 1"))
	fx.waitKinds(w, []string{"watch_started", "session_started", "nudged", "session_exited"})
	size := TerminalSize{Rows: 48, Cols: 210}
	fx.host.mu.Lock()
	fx.host.onStart = func() {
		if err := fx.svc.Resize(ctx, w.ID, size); err != nil {
			t.Errorf("Resize() error = %v", err)
		}
	}
	fx.host.mu.Unlock()

	if _, err := fx.svc.Send(ctx, w.ID, "go on"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if got := fx.host.last().terminalSize(); got != size {
		t.Fatalf("new session at %+v, want %+v", got, size)
	}
}

func TestANewSessionStartsAtTheLastSize(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	ctx := context.Background()
	if err := fx.svc.Resize(ctx, w.ID, TerminalSize{Rows: 48, Cols: 210}); err != nil {
		t.Fatal(err)
	}
	fx.host.last().exit(errors.New("exit status 1"))
	fx.waitKinds(w, []string{"watch_started", "session_started", "nudged", "session_exited"})

	if _, err := fx.svc.Send(ctx, w.ID, "go on"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if spec := fx.host.last().spec; fx.host.count() != 2 || spec.Rows != 48 || spec.Cols != 210 {
		t.Fatalf("sessions = %d, spec size = %dx%d, want a second session at 48x210", fx.host.count(), spec.Rows, spec.Cols)
	}
}

func TestResizeWithoutASessionKeepsTheSizeForTheNextOne(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	ctx := context.Background()
	fx.host.last().exit(nil)
	fx.waitKinds(w, []string{"watch_started", "session_started", "nudged", "session_exited"})

	if err := fx.svc.Resize(ctx, w.ID, TerminalSize{Rows: 60, Cols: 90}); err != nil {
		t.Fatalf("Resize() after the exit error = %v", err)
	}
	if _, err := fx.svc.Send(ctx, w.ID, "go on"); err != nil {
		t.Fatal(err)
	}

	if spec := fx.host.last().spec; spec.Rows != 60 || spec.Cols != 90 {
		t.Fatalf("spec size = %dx%d, want 60x90", spec.Rows, spec.Cols)
	}
}

func TestResizeRefusesAWatchWithoutASessionOfTheDaemon(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	self := fx.startSelf()
	size := TerminalSize{Rows: 40, Cols: 100}

	if err := fx.svc.Resize(ctx, self.ID, size); !errors.Is(err, ErrSelfWatch) {
		t.Fatalf("Resize() of a self watch error = %v, want ErrSelfWatch", err)
	}
	if err := fx.svc.Resize(ctx, 999, size); !errors.Is(err, store.ErrWatchNotFound) {
		t.Fatalf("Resize() of a missing watch error = %v, want ErrWatchNotFound", err)
	}
	if _, err := fx.svc.Stop(ctx, self.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := fx.svc.Resize(ctx, self.ID, size); !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("Resize() of a stopped watch error = %v, want ErrWatchStopped", err)
	}
}
