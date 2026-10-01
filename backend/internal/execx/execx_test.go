package execx

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// shell gives the argv that runs a script in the shell of the platform:
// sh on Unix, cmd on Windows. Each test names both forms of its script.
func shell(sh, cmd string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/d", "/c", cmd}
	}
	return []string{"sh", "-c", sh}
}

// sleeper gives a command that runs for seconds: sleep on Unix, and
// PowerShell on Windows, where sleep is a shell builtin with children
// that a kill would leave behind.
func sleeper() []string {
	if runtime.GOOS == "windows" {
		return []string{"powershell", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep 5"}
	}
	return []string{"sleep", "5"}
}

func shellName() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "sh"
}

func TestRunReportsExitCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	argv := shell("echo hello; exit 3", "echo hello& exit 3")
	out, err := Run(ctx, argv[0], argv[1:]...)
	if ExitCode(err) != 3 {
		t.Fatalf("exit code = %d (%v), want 3", ExitCode(err), err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Errorf("output = %q, want hello", out)
	}
	if !strings.Contains(err.Error(), "exit 3") || !strings.Contains(err.Error(), "hello") {
		t.Errorf("error = %q", err)
	}
	argv = shell("echo ok", "echo ok")
	if out, err := Run(ctx, argv[0], argv[1:]...); err != nil || strings.TrimSpace(out) != "ok" {
		t.Errorf("Run() = %q, %v", out, err)
	}
	if _, err := Run(ctx, "no-such-tool-babysitter"); err == nil || ExitCode(err) != -1 {
		t.Errorf("missing tool: err = %v, exit code %d, want -1", err, ExitCode(err))
	}
}

func TestRunReportsTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	argv := sleeper()
	_, err := Run(ctx, argv[0], argv[1:]...)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if ExitCode(err) != -1 {
		t.Errorf("exit code = %d, want -1", ExitCode(err))
	}
	if !strings.Contains(err.Error(), argv[0]) || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Errorf("error = %q", err)
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	if ExitCode(nil) != 0 {
		t.Error("nil: want 0")
	}
	if ExitCode(errors.New("boom")) != -1 {
		t.Error("plain error: want -1")
	}
	if ExitCode(&ExitError{Code: 7}) != 7 {
		t.Error("ExitError: want 7")
	}
}

func TestFound(t *testing.T) {
	t.Parallel()
	if !Found(shellName()) {
		t.Errorf("%s: want found", shellName())
	}
	if Found("no-such-tool-babysitter") {
		t.Error("missing tool: want not found")
	}
}

func TestRunIn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	argv := shell("pwd; cat; echo $BABYSITTER_TEST_VAR", "cd& more& echo %BABYSITTER_TEST_VAR%")
	out, err := RunIn(context.Background(), dir, "hello\n", []string{"BABYSITTER_TEST_VAR=yes"}, argv[0], argv[1:]...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello") || !strings.Contains(out, "yes") || !strings.Contains(out, filepath.Base(dir)) {
		t.Fatalf("out = %q", out)
	}
	argv = shell("exit 3", "exit 3")
	_, err = RunIn(context.Background(), dir, "", nil, argv[0], argv[1:]...)
	if ExitCode(err) != 3 {
		t.Fatalf("exit code = %d, %v", ExitCode(err), err)
	}
}
