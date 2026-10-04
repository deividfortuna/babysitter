//go:build !windows

package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type PTY struct {
	Timing Timing

	stopGrace time.Duration
}

func New() *PTY {
	return &PTY{Timing: DefaultTiming}
}

// Start runs the agent in a pty of its own session, so Stop can signal
// the agent and every process it started as one group.
func (h *PTY) Start(ctx context.Context, spec Spec) (Handle, error) {
	if err := checkSpec(spec); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(context.WithoutCancel(ctx), spec.Argv[0], spec.Argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = append(terminalEnv(os.Environ()), spec.Env...)
	rows, cols := size(spec)
	log, err := openLog(spec.LogPath)
	if err != nil {
		return nil, err
	}
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		log.close()
		return nil, fmt.Errorf("start %s: %w", spec.Argv[0], err)
	}
	pid := cmd.Process.Pid
	p := &process{
		term:      ptyFile{master},
		pid:       pid,
		log:       log,
		timing:    h.Timing,
		stopGrace: h.graceBeforeKill(),
		done:      make(chan struct{}),
		wait:      cmd.Wait,
		terminate: func() { _ = syscall.Kill(-pid, syscall.SIGTERM) },
		kill:      func() { _ = syscall.Kill(-pid, syscall.SIGKILL) },
	}
	go p.pump()
	return p, nil
}

type ptyFile struct {
	*os.File
}

func (f ptyFile) Resize(rows, cols uint16) error {
	return pty.Setsize(f.File, &pty.Winsize{Rows: rows, Cols: cols})
}
