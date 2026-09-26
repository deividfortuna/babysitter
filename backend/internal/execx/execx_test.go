package execx

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunReportsExitCode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	out, err := Run(ctx, "sh", "-c", "echo hello; exit 3")
	if ExitCode(err) != 3 {
		t.Fatalf("exit code = %d (%v), want 3", ExitCode(err), err)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Errorf("output = %q, want hello", out)
	}
	if !strings.Contains(err.Error(), "exit 3") || !strings.Contains(err.Error(), "hello") {
		t.Errorf("error = %q", err)
	}
	if out, err := Run(ctx, "sh", "-c", "echo ok"); err != nil || strings.TrimSpace(out) != "ok" {
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
	_, err := Run(ctx, "sleep", "5")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if ExitCode(err) != -1 {
		t.Errorf("exit code = %d, want -1", ExitCode(err))
	}
	if !strings.Contains(err.Error(), "sleep") || !strings.Contains(err.Error(), "deadline exceeded") {
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
	if !Found("sh") {
		t.Error("sh: want found")
	}
	if Found("no-such-tool-babysitter") {
		t.Error("missing tool: want not found")
	}
}

func TestRunIn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, err := RunIn(context.Background(), dir, "hello\n", []string{"BABYSITTER_TEST_VAR=yes"}, "sh", "-c", "pwd; cat; echo $BABYSITTER_TEST_VAR")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello") || !strings.Contains(out, "yes") || !strings.Contains(out, filepath.Base(dir)) {
		t.Fatalf("out = %q", out)
	}
	_, err = RunIn(context.Background(), dir, "", nil, "sh", "-c", "exit 3")
	if ExitCode(err) != 3 {
		t.Fatalf("exit code = %d, %v", ExitCode(err), err)
	}
}
