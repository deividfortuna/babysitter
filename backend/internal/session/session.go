//go:build !windows

package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	defaultRows  = 30
	defaultCols  = 180
	keepBytes    = 256 << 10
	maxLogBytes  = 4 << 20
	logFileMode  = 0o600
	stopGrace    = 5 * time.Second
	stopPoll     = 50 * time.Millisecond
	readyPoll    = 100 * time.Millisecond
	interruptKey = "\x03"
	submitKey    = "\r"
)

type PTY struct {
	Timing Timing
}

func New() *PTY {
	return &PTY{Timing: DefaultTiming}
}

func (h *PTY) Start(ctx context.Context, spec Spec) (Handle, error) {
	if len(spec.Argv) == 0 {
		return nil, errors.New("a command is required")
	}
	if spec.Dir == "" {
		return nil, errors.New("a working directory is required")
	}
	cmd := exec.CommandContext(context.WithoutCancel(ctx), spec.Argv[0], spec.Argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = append(terminalEnv(os.Environ()), spec.Env...)
	rows, cols := spec.Rows, spec.Cols
	if rows == 0 {
		rows = defaultRows
	}
	if cols == 0 {
		cols = defaultCols
	}
	log, err := openLog(spec.LogPath)
	if err != nil {
		return nil, err
	}
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		log.close()
		return nil, fmt.Errorf("start %s: %w", spec.Argv[0], err)
	}
	p := &process{cmd: cmd, master: master, log: log, timing: h.Timing, done: make(chan struct{})}
	go p.pump()
	return p, nil
}

func terminalEnv(env []string) []string {
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		switch {
		case hasKey(kv, "NO_COLOR"), hasKey(kv, "TERM"), hasKey(kv, "COLORTERM"):
			continue
		}
		out = append(out, kv)
	}
	return append(out, "TERM=xterm-256color", "COLORTERM=truecolor")
}

func hasKey(kv, key string) bool {
	return len(kv) > len(key) && kv[:len(key)] == key && kv[len(key)] == '='
}

type sessionLog struct {
	path string
	f    *os.File
	size int64
}

func openLog(path string) (*sessionLog, error) {
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create session log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
	if err != nil {
		return nil, fmt.Errorf("open session log: %w", err)
	}
	l := &sessionLog{path: path, f: f}
	if info, err := f.Stat(); err == nil {
		l.size = info.Size()
	}
	return l, nil
}

func (l *sessionLog) open() bool {
	return l != nil && l.f != nil
}

func (l *sessionLog) write(b []byte) {
	if l.full(len(b)) {
		l.rotate()
	}
	if !l.open() {
		return
	}
	n, _ := l.f.Write(b)
	l.size += int64(n)
}

func (l *sessionLog) full(n int) bool {
	return l.open() && l.size+int64(n) > maxLogBytes
}

func (l *sessionLog) rotate() {
	l.f.Close()
	l.f, l.size = nil, 0
	if err := os.Rename(l.path, l.path+RotatedSuffix); err != nil {
		return
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
	if err != nil {
		return
	}
	l.f = f
}

func (l *sessionLog) close() {
	if !l.open() {
		return
	}
	l.f.Close()
	l.f = nil
}

type process struct {
	cmd    *exec.Cmd
	master *os.File
	log    *sessionLog
	timing Timing

	writeMu sync.Mutex

	outMu   sync.Mutex
	out     []byte
	wroteAt time.Time

	done    chan struct{}
	exitErr error
}

func (p *process) pump() {
	buf := make([]byte, 32<<10)
	for {
		n, err := p.master.Read(buf)
		if n > 0 {
			p.keep(buf[:n])
			p.log.write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	p.exitErr = p.cmd.Wait()
	p.log.close()
	p.master.Close()
	close(p.done)
}

func (p *process) keep(b []byte) {
	p.outMu.Lock()
	defer p.outMu.Unlock()
	p.out = append(p.out, b...)
	p.wroteAt = time.Now()
	if len(p.out) > keepBytes {
		p.out = append([]byte(nil), p.out[len(p.out)-keepBytes:]...)
	}
}

func (p *process) still() (time.Duration, bool) {
	p.outMu.Lock()
	defer p.outMu.Unlock()
	if p.wroteAt.IsZero() {
		return 0, false
	}
	return time.Since(p.wroteAt), true
}

func (p *process) Ready(ctx context.Context) error {
	deadline := time.Now().Add(p.timing.ReadyWait)
	for time.Now().Before(deadline) {
		if p.exited() {
			return ErrExited
		}
		if quiet, printed := p.still(); printed && quiet >= p.timing.ReadySettle {
			return nil
		}
		if err := sleep(ctx, readyPoll); err != nil {
			return err
		}
	}
	return nil
}

func (p *process) Send(ctx context.Context, text string) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.exited() {
		return ErrExited
	}
	if text != "" {
		for _, chunk := range chunks(text, p.timing.ChunkRunes) {
			if err := p.write(chunk); err != nil {
				return err
			}
			if err := sleep(ctx, p.timing.ChunkDelay); err != nil {
				return err
			}
		}
		if err := sleep(ctx, p.timing.EnterDelay); err != nil {
			return err
		}
	}
	return p.write(submitKey)
}

func (p *process) write(s string) error {
	if _, err := io.WriteString(p.master, s); err != nil {
		if p.exited() {
			return ErrExited
		}
		return fmt.Errorf("write to the agent terminal: %w", err)
	}
	return nil
}

func (p *process) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *process) Interrupt() error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.write(interruptKey)
}

func (p *process) Output(n int) string {
	p.outMu.Lock()
	out := append([]byte(nil), p.out...)
	p.outMu.Unlock()
	if n <= 0 {
		return string(out)
	}
	return lastLines(out, n)
}

func (p *process) Done() <-chan struct{} { return p.done }

func (p *process) Err() error {
	select {
	case <-p.done:
		return p.exitErr
	default:
		return nil
	}
}

func (p *process) PID() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *process) Stop(ctx context.Context) error {
	if p.exited() {
		return nil
	}
	pid := p.PID()
	if pid <= 0 {
		return nil
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	deadline := time.Now().Add(stopGrace)
	for time.Now().Before(deadline) {
		if p.exited() {
			return nil
		}
		if err := sleep(ctx, stopPoll); err != nil {
			return err
		}
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
