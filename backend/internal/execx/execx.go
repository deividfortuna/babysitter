package execx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Runner func(ctx context.Context, name string, args ...string) (string, error)

type LookPath func(name string) bool

type ExitError struct {
	Name   string
	Args   []string
	Code   int
	Output string
}

func (e *ExitError) Error() string {
	msg := fmt.Sprintf("%s %s: exit %d", e.Name, strings.Join(e.Args, " "), e.Code)
	if out := strings.TrimSpace(e.Output); out != "" {
		msg += ": " + out
	}
	return msg
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := errors.AsType[*ExitError](err); ok {
		return ee.Code
	}
	return -1
}

func Run(ctx context.Context, name string, args ...string) (string, error) {
	return RunIn(ctx, "", "", nil, name, args...)
}

func Found(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

type DirRunner func(ctx context.Context, dir, stdin string, env []string, name string, args ...string) (string, error)

func RunIn(ctx context.Context, dir, stdin string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return string(out), fmt.Errorf("%s: %w", name, ctx.Err())
		}
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			return string(out), &ExitError{Name: name, Args: args, Code: ee.ExitCode(), Output: string(out)}
		}
		return string(out), fmt.Errorf("%s: %w", name, err)
	}
	return string(out), nil
}
