package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

var ErrDetachedHead = errors.New("HEAD is detached, check out a branch or name the pull request")

var NoPromptEnv = []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}

type AuthEnv func(ctx context.Context) ([]string, error)

func (a AuthEnv) Env(ctx context.Context) ([]string, error) {
	if a == nil {
		return NoPromptEnv, nil
	}
	extra, err := a(ctx)
	if err != nil {
		return nil, err
	}
	return append(slices.Clone(NoPromptEnv), extra...), nil
}

func CurrentBranch(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if out == "HEAD" {
		return "", ErrDetachedHead
	}
	return out, nil
}

func RemoteURL(ctx context.Context, dir, name string) (string, error) {
	return run(ctx, dir, "remote", "get-url", name)
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			msg := strings.TrimSpace(string(exitErr.Stderr))
			if msg == "" {
				msg = exitErr.String()
			}
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func ConfigValue(ctx context.Context, dir, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "--get", key)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("git config --get %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}
