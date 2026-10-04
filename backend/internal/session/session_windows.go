//go:build windows

package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/charmbracelet/x/conpty"
	"golang.org/x/sys/windows"
)

type PTY struct {
	Timing Timing

	beforeJob func()
	stopGrace time.Duration
}

func New() *PTY {
	return &PTY{Timing: DefaultTiming}
}

// Start runs the agent in a pseudo console, the pty of Windows 10 1809
// and later. A job object holds the agent and every process it starts:
// Stop ends the whole tree, and nothing outlives the daemon.
func (h *PTY) Start(_ context.Context, spec Spec) (Handle, error) {
	if err := checkSpec(spec); err != nil {
		return nil, err
	}
	path, err := lookPath(spec.Argv[0])
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", spec.Argv[0], err)
	}
	log, err := openLog(spec.LogPath)
	if err != nil {
		return nil, err
	}
	rows, cols := size(spec)
	term, err := newConsole(rows, cols)
	if err != nil {
		log.close()
		return nil, err
	}
	env := append(terminalEnv(os.Environ()), spec.Env...)
	suspended := &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	pid, handle, err := term.pty.Spawn(path, spec.Argv, &syscall.ProcAttr{Dir: spec.Dir, Env: env, Sys: suspended})
	if err != nil {
		term.Close()
		log.close()
		return nil, fmt.Errorf("start %s: %w", spec.Argv[0], err)
	}
	if h.beforeJob != nil {
		h.beforeJob()
	}
	if err := term.adopt(windows.Handle(handle)); err != nil {
		term.kill()
		term.Close()
		log.close()
		return nil, fmt.Errorf("start %s: %w", spec.Argv[0], err)
	}

	exited := make(chan struct{})
	var exitErr error
	go func() {
		exitErr = term.waitExit()
		close(exited)
		// The pseudo console keeps the output pipe open after the agent
		// exits. Closing it drains the last output to pump and ends the read.
		term.Close()
	}()
	p := &process{
		term:      term,
		pid:       pid,
		log:       log,
		timing:    h.Timing,
		stopGrace: h.graceBeforeKill(),
		done:      make(chan struct{}),
		wait: func() error {
			<-exited
			return exitErr
		},
		kill: term.kill,
	}
	// Windows has no signal to ask a console program to exit. Ctrl+C is
	// what the user would press, and kill comes after the grace period.
	p.terminate = func() { _ = p.Interrupt() }
	go p.pump()
	return p, nil
}

// lookPath resolves a bare command name on PATH, as os/exec does. A name
// with a directory stays as it is, and Spawn resolves it against Dir.
func lookPath(name string) (string, error) {
	if filepath.Base(name) != name {
		return name, nil
	}
	return exec.LookPath(name)
}

// A console is a pseudo console with the job object that holds the
// agent. Close is safe to call more than once and from several
// goroutines, because the exit watcher and pump both call it.
type console struct {
	pty *conpty.ConPty
	job windows.Handle

	mu     sync.Mutex
	proc   windows.Handle
	closed bool
}

func newConsole(rows, cols uint16) (*console, error) {
	pty, err := conpty.New(int(cols), int(rows), 0)
	if err != nil {
		return nil, fmt.Errorf("create the pseudo console: %w", err)
	}
	job, err := killOnCloseJob()
	if err != nil {
		pty.Close()
		return nil, err
	}
	return &console{pty: pty, job: job}, nil
}

// killOnCloseJob makes a job object that ends every process in it when
// its last handle closes, so a daemon that dies takes its agents along.
func killOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create the job of the agent: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("configure the job of the agent: %w", err)
	}
	return job, nil
}

// adopt takes the process handle of an agent created suspended, puts the
// agent in the job and only then lets it run, so every process it starts
// is in the job too.
func (c *console) adopt(proc windows.Handle) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.proc = proc
	if err := windows.AssignProcessToJobObject(c.job, proc); err != nil {
		return fmt.Errorf("put the agent in its job: %w", err)
	}
	return resume(proc)
}

// resume lets the main thread of a suspended process run. Spawn closes
// the handle of the thread, so a snapshot of the threads finds it again.
func resume(proc windows.Handle) error {
	pid, err := windows.GetProcessId(proc)
	if err != nil {
		return fmt.Errorf("read the pid of the agent: %w", err)
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("list the threads of the agent: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID == pid {
			return resumeThread(entry.ThreadID)
		}
	}
	return fmt.Errorf("find the thread of the agent: %w", err)
}

func resumeThread(id uint32) error {
	thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, id)
	if err != nil {
		return fmt.Errorf("open the thread of the agent: %w", err)
	}
	defer func() { _ = windows.CloseHandle(thread) }()
	if _, err := windows.ResumeThread(thread); err != nil {
		return fmt.Errorf("resume the agent: %w", err)
	}
	return nil
}

func (c *console) waitExit() error {
	if _, err := windows.WaitForSingleObject(c.proc, windows.INFINITE); err != nil {
		return fmt.Errorf("wait for the agent: %w", err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(c.proc, &code); err != nil {
		return fmt.Errorf("read the exit code of the agent: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("exit status %d", code)
	}
	return nil
}

func (c *console) kill() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	_ = windows.TerminateJobObject(c.job, 1)
	if c.proc != 0 {
		_ = windows.TerminateProcess(c.proc, 1)
	}
}

func (c *console) Read(b []byte) (int, error) { return c.pty.Read(b) }

func (c *console) Write(b []byte) (int, error) { return c.pty.Write(b) }

func (c *console) Resize(rows, cols uint16) error {
	return c.pty.Resize(int(cols), int(rows))
}

func (c *console) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	err := c.pty.Close()
	if c.proc != 0 {
		_ = windows.CloseHandle(c.proc)
	}
	_ = windows.CloseHandle(c.job)
	return err
}
