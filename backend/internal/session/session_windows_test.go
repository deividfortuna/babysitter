//go:build windows

package session

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/deividfortuna/babysitter/internal/processalive"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

const helperEnv = "BABYSITTER_SESSION_HELPER"

// The test binary plays the agent. A session starts it again with a mode
// in the environment, so the tests need no shell and no script on disk.
func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		runHelper(mode)
		return
	}
	os.Exit(m.Run())
}

func runHelper(mode string) {
	switch mode {
	case "echo":
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			fmt.Printf("got:%s\n", sc.Text())
		}
	case "bye":
		fmt.Println("bye")
		os.Exit(3)
	case "fresh":
		fmt.Println("fresh output")
	case "stubborn":
		signal.Ignore(os.Interrupt)
		fmt.Println("ready")
		select {}
	case "hold":
		fmt.Print("hello")
		time.Sleep(5 * time.Second)
	case "size":
		rows, cols := consoleSize()
		fmt.Printf("%d %d\n", rows, cols)
		time.Sleep(5 * time.Second)
	case "resize":
		// The baseline comes before "ready", so a resize that follows the
		// word is always a change from it.
		rows, cols := consoleSize()
		fmt.Println("ready")
		for {
			time.Sleep(50 * time.Millisecond)
			if r, c := consoleSize(); r != rows || c != cols {
				rows, cols = r, c
				fmt.Printf("size %d %d\n", rows, cols)
			}
		}
	case "parent":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), helperEnv+"=stubborn")
		if err := child.Start(); err != nil {
			fmt.Printf("child failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("child %d\n", child.Process.Pid)
		select {}
	case "quiet":
	}
}

func consoleSize() (rows, cols int) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(os.Stdout.Fd()), &info); err != nil {
		return 0, 0
	}
	return int(info.Window.Bottom - info.Window.Top + 1), int(info.Window.Right - info.Window.Left + 1)
}

func helperSpec(t *testing.T, mode string) Spec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Spec{Dir: t.TempDir(), Argv: []string{exe}, Env: []string{helperEnv + "=" + mode}}
}

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
	spec := helperSpec(t, "echo")
	spec.LogPath = log
	h, err := host.Start(context.Background(), spec)
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
	// The console echoes what Send types, so the two last lines are the
	// echo of the message and the answer of the helper.
	if got := Strip(h.Output(2)); !strings.Contains(got, "got:second") || strings.Contains(got, "hello agent") {
		t.Fatalf("last lines = %q", got)
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
	h, err := New().Start(context.Background(), helperSpec(t, "bye"))
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
	if _, err := New().Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{"no-such-command-babysitter"}}); err == nil {
		t.Fatal("Start() of a missing command succeeded")
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

	spec := helperSpec(t, "fresh")
	spec.LogPath = log
	h, err := New().Start(context.Background(), spec)
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

func TestStopKillsAProcessThatIgnoresTheInterrupt(t *testing.T) {
	t.Parallel()
	h, err := New().Start(context.Background(), helperSpec(t, "stubborn"))
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
		t.Fatal("the process that ignored the interrupt is still running")
	}
}

func TestStopEndsAChildTheAgentStartsBeforeItJoinsTheJob(t *testing.T) {
	t.Parallel()
	host := &PTY{Timing: DefaultTiming, beforeJob: func() { time.Sleep(2 * time.Second) }}
	h, err := host.Start(context.Background(), helperSpec(t, "parent"))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitFor(t, h, "child ")
	out := Strip(h.Output(0))
	_, line, _ := strings.Cut(out, "child ")
	var child int
	if _, err := fmt.Sscanf(line, "%d", &child); err != nil {
		t.Fatalf("no child pid in %q: %v", out, err)
	}
	t.Cleanup(func() {
		if p, err := os.FindProcess(child); err == nil {
			_ = p.Kill()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), stopGrace+5*time.Second)
	defer cancel()
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	<-h.Done()
	if !testutil.Within(testutil.Timeout, func() bool { return !processalive.Alive(child) }) {
		t.Fatalf("the child %d of the agent outlived Stop", child)
	}
}

func TestStopReportsADeadContextBeforeTheKill(t *testing.T) {
	t.Parallel()
	h, err := New().Start(context.Background(), helperSpec(t, "stubborn"))
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
	p, err := h.Start(context.Background(), helperSpec(t, "hold"))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(context.Background())

	start := time.Now()
	if err := p.Ready(context.Background()); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	if took := time.Since(start); took < 150*time.Millisecond || took > 2*time.Second {
		t.Fatalf("Ready() took %s, want about the settle time", took)
	}
}

func TestReadyAnswersTheExitOfTheProcess(t *testing.T) {
	t.Parallel()
	h := &PTY{Timing: Timing{ChunkRunes: 512, ReadyWait: 2 * time.Second, ReadySettle: time.Second}}
	p, err := h.Start(context.Background(), helperSpec(t, "quiet"))
	if err != nil {
		t.Fatal(err)
	}
	<-p.Done()
	if err := p.Ready(context.Background()); !errors.Is(err, ErrExited) {
		t.Fatalf("Ready() error = %v, want ErrExited", err)
	}
}

func TestStartGivesTheTerminalTheSizeOfTheSpec(t *testing.T) {
	t.Parallel()
	spec := helperSpec(t, "size")
	spec.Rows, spec.Cols = 42, 132
	h, err := New().Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop(context.Background())
	waitFor(t, h, "42 132")
}

func TestResizeChangesTheSizeTheProgramSees(t *testing.T) {
	t.Parallel()
	h, err := New().Start(context.Background(), helperSpec(t, "resize"))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop(context.Background())
	waitFor(t, h, "ready")

	if err := h.Resize(55, 210); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}

	waitFor(t, h, "size 55 210")
}

func TestResizeAnswersTheExitOfTheProcess(t *testing.T) {
	t.Parallel()
	h, err := New().Start(context.Background(), helperSpec(t, "quiet"))
	if err != nil {
		t.Fatal(err)
	}
	<-h.Done()

	if err := h.Resize(40, 100); !errors.Is(err, ErrExited) {
		t.Fatalf("Resize() error = %v, want ErrExited", err)
	}
}
