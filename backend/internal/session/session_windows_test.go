//go:build windows

package session

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/deividfortuna/babysitter/internal/processalive"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

const linesOfAnAnswer = 2

func ignoreTheRequestToStop() {
	signal.Ignore(os.Interrupt)
}

func consoleSize() (rows, cols int) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(os.Stdout.Fd()), &info); err != nil {
		return 0, 0
	}
	return int(info.Window.Bottom - info.Window.Top + 1), int(info.Window.Right - info.Window.Left + 1)
}

func TestStopKillsAProcessThatIgnoresTheInterrupt(t *testing.T) {
	t.Parallel()
	assertStopKillsAStubbornProcess(t)
}

func TestStopEndsAChildTheAgentStartsBeforeItJoinsTheJob(t *testing.T) {
	t.Parallel()
	host := quickStop()
	host.beforeJob = func() { time.Sleep(2 * time.Second) }
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
	ctx, cancel := context.WithTimeout(context.Background(), testutil.Timeout)
	defer cancel()
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	<-h.Done()
	if !testutil.Within(testutil.Timeout, func() bool { return !processalive.Alive(child) }) {
		t.Fatalf("the child %d of the agent outlived Stop", child)
	}
}
