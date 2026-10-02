package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var ErrDetachedHead = errors.New("HEAD is detached, check out a branch or name the pull request")

var NoPromptEnv = []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}

const GHHelper = "!gh auth git-credential"

func ConfigEnv(pairs [][2]string) []string {
	env := []string{"GIT_CONFIG_COUNT=" + strconv.Itoa(len(pairs))}
	for i, p := range pairs {
		env = append(env, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, p[0]), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, p[1]))
	}
	return env
}

type AuthEnv func(ctx context.Context) ([]string, error)

var networkCommands = []string{"fetch", "push", "clone", "pull", "ls-remote"}

func (a AuthEnv) Env(ctx context.Context, command string) ([]string, error) {
	if a == nil {
		return NoPromptEnv, nil
	}
	extra, err := a(ctx)
	if err == nil {
		return append(slices.Clone(NoPromptEnv), extra...), nil
	}
	if slices.Contains(networkCommands, command) {
		return nil, err
	}
	return append(slices.Clone(NoPromptEnv), "GIT_NO_LAZY_FETCH=1"), nil
}

var ErrNotHTTPS = errors.New("the GitHub App reaches GitHub only over HTTPS")

var scpRemote = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):`)

func (a AuthEnv) CheckRemote(ctx context.Context, remote string, push bool, git func(ctx context.Context, args ...string) (string, error)) error {
	if !a.usesApp(ctx) {
		return nil
	}
	args := []string{"remote", "get-url"}
	if push {
		args = append(args, "--push")
	}
	remoteURL, err := git(ctx, append(args, remote)...)
	if err != nil {
		return err
	}
	return CheckAppRemote(remoteURL)
}

func (a AuthEnv) usesApp(ctx context.Context) bool {
	if a == nil {
		return false
	}
	extra, err := a(ctx)
	return err == nil && len(extra) > 0
}

func CheckAppRemote(remoteURL string) error {
	scheme, host := remoteHost(remoteURL)
	if scheme == "https" || !isGitHubHost(host) {
		return nil
	}
	return fmt.Errorf("%w: give the remote %s an https://github.com/ URL", ErrNotHTTPS, remoteURL)
}

func remoteHost(remoteURL string) (scheme, host string) {
	if strings.Contains(remoteURL, "://") {
		u, err := url.Parse(remoteURL)
		if err != nil {
			return "", ""
		}
		return strings.ToLower(u.Scheme), u.Hostname()
	}
	if m := scpRemote.FindStringSubmatch(remoteURL); m != nil {
		return "ssh", m[1]
	}
	return "", ""
}

func isGitHubHost(host string) bool {
	host = strings.ToLower(host)
	return host == "github.com" || strings.HasSuffix(host, ".github.com")
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
