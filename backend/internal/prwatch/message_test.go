package prwatch

import (
	"bytes"
	"context"
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
