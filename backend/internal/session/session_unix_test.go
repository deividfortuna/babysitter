//go:build !windows

package session

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/creack/pty"
)

const linesOfAnAnswer = 1

func ignoreTheRequestToStop() {
	signal.Ignore(syscall.SIGTERM)
}

func consoleSize() (rows, cols int) {
	rows, cols, _ = pty.Getsize(os.Stdout)
	return rows, cols
}

func TestTheSessionLogIsPrivate(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "logs", "1.log")
	spec := helperSpec(t, "fresh")
	spec.LogPath = log

	h, err := New().Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	<-h.Done()

	assertPrivateFile(t, log)
}

func TestStopKillsAProcessThatIgnoresTheTerminationSignal(t *testing.T) {
	t.Parallel()
	assertStopKillsAStubbornProcess(t)
}
