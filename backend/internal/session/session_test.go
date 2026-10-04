package session

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

const (
	helperEnv     = "BABYSITTER_SESSION_HELPER"
	helperLife    = time.Minute
	testStopGrace = 100 * time.Millisecond
)

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
		ignoreTheRequestToStop()
		fmt.Println("ready")
		time.Sleep(helperLife)
	case "hold":
		fmt.Print("hello")
		time.Sleep(helperLife)
	case "size":
		rows, cols := consoleSize()
		fmt.Printf("%d %d\n", rows, cols)
		time.Sleep(helperLife)
	case "resize":
		printSizeChanges()
	case "parent":
		startStubbornChild()
	case "quiet":
	}
}

func printSizeChanges() {
	rows, cols := consoleSize()
	fmt.Println("ready")
	for range time.Tick(50 * time.Millisecond) {
		r, c := consoleSize()
		changed := r != rows || c != cols
		if changed {
			rows, cols = r, c
			fmt.Printf("size %d %d\n", rows, cols)
		}
	}
}

func startStubbornChild() {
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), helperEnv+"=stubborn")
	if err := child.Start(); err != nil {
		fmt.Printf("child failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("child %d\n", child.Process.Pid)
	time.Sleep(helperLife)
}

func helperSpec(t *testing.T, mode string) Spec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Spec{Dir: t.TempDir(), Argv: []string{exe}, Env: []string{helperEnv + "=" + mode}}
}

func quickStop() *PTY {
	return &PTY{Timing: DefaultTiming, stopGrace: testStopGrace}
}

func waitFor(t *testing.T, h Handle, want string) {
	t.Helper()
	if testutil.Within(testutil.Timeout, func() bool { return strings.Contains(Strip(h.Output(0)), want) }) {
		return
	}
	t.Fatalf("output never showed %q:\n%s", want, Strip(h.Output(0)))
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

func assertStopKillsAStubbornProcess(t *testing.T) {
	t.Helper()
	h, err := quickStop().Start(context.Background(), helperSpec(t, "stubborn"))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitFor(t, h, "ready")
	ctx, cancel := context.WithTimeout(context.Background(), testutil.Timeout)
	defer cancel()
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-h.Done():
	case <-time.After(time.Second):
		t.Fatal("the process that ignored the request to stop is still running")
	}
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
	if got := Strip(h.Output(linesOfAnAnswer)); !strings.Contains(got, "got:second") || strings.Contains(got, "hello agent") {
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
	case <-time.After(testutil.Timeout):
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
	for _, command := range []string{"/no/such/command", "no-such-command-babysitter"} {
		if _, err := New().Start(context.Background(), Spec{Dir: t.TempDir(), Argv: []string{command}}); err == nil {
			t.Fatalf("Start() of the missing command %q succeeded", command)
		}
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
	if _, err := os.Stat(log + RotatedSuffix); err != nil {
		t.Fatalf("the full log was not kept beside the new one: %v", err)
	}
	if runtime.GOOS != "windows" {
		assertPrivateFile(t, log)
	}
}

func TestStopReportsADeadContextBeforeTheKill(t *testing.T) {
	t.Parallel()
	h, err := quickStop().Start(context.Background(), helperSpec(t, "stubborn"))
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
	settle := 150 * time.Millisecond
	host := &PTY{Timing: Timing{ChunkRunes: 512, ReadyWait: testutil.Timeout, ReadySettle: settle}}
	p, err := host.Start(context.Background(), helperSpec(t, "hold"))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(context.Background())

	start := time.Now()
	if err := p.Ready(context.Background()); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	took := time.Since(start)
	if took < settle {
		t.Fatalf("Ready() took %s, less than the settle time of %s", took, settle)
	}
	if took >= host.Timing.ReadyWait {
		t.Fatalf("Ready() took %s: the terminal never settled and the wait ran out", took)
	}
}

func TestReadyAnswersTheExitOfTheProcess(t *testing.T) {
	t.Parallel()
	host := &PTY{Timing: Timing{ChunkRunes: 512, ReadyWait: 2 * time.Second, ReadySettle: time.Second}}
	p, err := host.Start(context.Background(), helperSpec(t, "quiet"))
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
