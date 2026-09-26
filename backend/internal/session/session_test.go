//go:build !windows

package session

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

func waitFor(t *testing.T, h Handle, want string) {
	t.Helper()
	if testutil.Within(testutil.Timeout, func() bool { return strings.Contains(Strip(h.Output(0)), want) }) {
		return
	}
	t.Fatalf("output never showed %q:\n%s", want, Strip(h.Output(0)))
}

func TestSendTypesAMessageAndSubmitsIt(t *testing.T) {
	t.Parallel()
	host := &PTY{Timing: Timing{ChunkRunes: 4, ChunkDelay: time.Millisecond, EnterDelay: 5 * time.Millisecond}}
	log := filepath.Join(t.TempDir(), "logs", "1.log")
	h, err := host.Start(context.Background(), Spec{
		Dir:     t.TempDir(),
		Argv:    []string{"sh", "-c", `while read line; do echo "got:$line"; done`},
		LogPath: log,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer h.Stop(context.Background())
	if h.PID() <= 0 {
		t.Fatalf("pid = %d", h.PID())
	}
	if err := h.Send(context.Background(), "hello agent"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	waitFor(t, h, "got:hello agent")
	if err := h.Send(context.Background(), "second"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	waitFor(t, h, "got:second")
	if got := Strip(h.Output(1)); !strings.Contains(got, "got:second") || strings.Contains(got, "hello agent") {
		t.Fatalf("last line = %q", got)
	}
	select {
	case <-h.Done():
		t.Fatal("the process exited on its own")
	default:
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	<-h.Done()
	if err := h.Send(context.Background(), "late"); !errors.Is(err, ErrExited) {
		t.Fatalf("Send() after exit error = %v, want ErrExited", err)
	}
	data, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(data), "got:hello agent") {
		t.Fatalf("log = %q, %v", data, err)
	}
}

func TestDoneReportsAnExit(t *testing.T) {
	t.Parallel()
	h, err := New().Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{"sh", "-c", "echo bye; exit 3"}})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the process never exited")
	}
	if h.Err() == nil || !strings.Contains(h.Err().Error(), "3") {
		t.Fatalf("Err() = %v", h.Err())
	}
	if !strings.Contains(h.Output(0), "bye") {
		t.Fatalf("output = %q", h.Output(0))
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() of an exited process error = %v", err)
	}
}

func TestStartRejectsAMissingCommand(t *testing.T) {
	t.Parallel()
	if _, err := New().Start(context.Background(), Spec{Dir: t.TempDir()}); err == nil {
		t.Fatal("Start() without a command succeeded")
	}
	if _, err := New().Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{"/no/such/command"}}); err == nil {
		t.Fatal("Start() of a missing command succeeded")
	}
}

func TestChunks(t *testing.T) {
	t.Parallel()
	got := chunks("héllo wörld", 4)
	if len(got) != 3 || got[0] != "héll" || got[1] != "o wö" || got[2] != "rld" {
		t.Fatalf("chunks = %q", got)
	}
	if got := chunks("abc", 0); len(got) != 1 || got[0] != "abc" {
		t.Fatalf("chunks without a size = %q", got)
	}
}

func TestStrip(t *testing.T) {
	t.Parallel()
	in := "\x1b[31mred\x1b[0m \x1b]0;title\x07plain\r\n\x1b[?25l"
	if got := Strip(in); got != "red plain\r\n" {
		t.Fatalf("Strip = %q", got)
	}
}

func TestLastLines(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"whole lines", "a\nb\nc\n", 2, "b\nc\n"},
		{"beyond the start", "a\nb", 5, "a\nb"},
		{"the line being written", "a\nb", 1, "b"},
		{"the line being written, of two", "a\nb\nc", 2, "b\nc"},
		{"the last whole line", "a\nb\n", 1, "b\n"},
		{"one line without a break", "abc", 1, "abc"},
		{"nothing", "", 3, ""},
		{"all of it", "a\nb\nc\n", 0, "a\nb\nc\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lastLines([]byte(c.in), c.n); got != c.want {
				t.Fatalf("lastLines(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
			}
		})
	}
}

func TestTheSessionLogDoesNotGrowPastTheCap(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "logs", "1.log")
	if err := os.MkdirAll(filepath.Dir(log), 0o750); err != nil {
		t.Fatal(err)
	}
	old := bytes.Repeat([]byte("an earlier session printed this\n"), (5<<20)/32)
	if err := os.WriteFile(log, old, 0o640); err != nil {
		t.Fatal(err)
	}

	h, err := New().Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{"sh", "-c", "echo fresh output"}, LogPath: log})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	<-h.Done()

	info, err := os.Stat(log)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > maxLogBytes {
		t.Fatalf("the log is %d bytes, past the cap of %d", info.Size(), maxLogBytes)
	}
	data, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(data), "fresh output") {
		t.Fatalf("the log lost what the session printed: %v", err)
	}
	if _, err := os.Stat(log + ".1"); err != nil {
		t.Fatalf("the full log was not kept beside the new one: %v", err)
	}
}

func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("%s mode = %o, want 600", path, got)
	}
}

func TestTheSessionLogIsPrivate(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "logs", "1.log")

	h, err := New().Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{"sh", "-c", "echo hello"}, LogPath: log})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	<-h.Done()

	assertPrivateFile(t, log)
}

func TestTheSessionLogStaysPrivateAfterItRotates(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "1.log")
	full := bytes.Repeat([]byte("an earlier session printed this\n"), (5<<20)/32)
	if err := os.WriteFile(log, full, 0o600); err != nil {
		t.Fatal(err)
	}

	h, err := New().Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{"sh", "-c", "echo fresh output"}, LogPath: log})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	<-h.Done()

	assertPrivateFile(t, log)
}

func TestStopKillsAProcessThatIgnoresTheTerminationSignal(t *testing.T) {
	t.Parallel()
	h, err := New().Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{"sh", "-c", `trap "" TERM; echo ready; while :; do sleep 1; done`}})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitFor(t, h, "ready")
	ctx, cancel := context.WithTimeout(context.Background(), stopGrace+5*time.Second)
	defer cancel()
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-h.Done():
	case <-time.After(time.Second):
		t.Fatal("the process that ignored the termination signal is still running")
	}
}

func TestStopReportsADeadContextBeforeTheKill(t *testing.T) {
	t.Parallel()
	h, err := New().Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{"sh", "-c", `trap "" TERM; echo ready; while :; do sleep 1; done`}})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitFor(t, h, "ready")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.Stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop() error = %v, want the cancelled context", err)
	}
	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() with a live context error = %v", err)
	}
	<-h.Done()
}

func TestReadyWaitsForTheTerminalToSettle(t *testing.T) {
	t.Parallel()
	h := &PTY{Timing: Timing{ChunkRunes: 512, ReadyWait: 2 * time.Second, ReadySettle: 150 * time.Millisecond}}
	p, err := h.Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{"sh", "-c", "printf hello; sleep 5"}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(context.Background())

	start := time.Now()
	if err := p.Ready(context.Background()); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	if took := time.Since(start); took < 150*time.Millisecond || took > time.Second {
		t.Fatalf("Ready() took %s, want about the settle time", took)
	}
}

func TestReadyAnswersTheExitOfTheProcess(t *testing.T) {
	t.Parallel()
	h := &PTY{Timing: Timing{ChunkRunes: 512, ReadyWait: 2 * time.Second, ReadySettle: time.Second}}
	p, err := h.Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{"sh", "-c", "exit 0"}})
	if err != nil {
		t.Fatal(err)
	}
	<-p.Done()
	if err := p.Ready(context.Background()); !errors.Is(err, ErrExited) {
		t.Fatalf("Ready() error = %v, want ErrExited", err)
	}
}
