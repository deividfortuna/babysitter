package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deividfortuna/babysitter/internal/execx"
)

var ErrBranchLeft = errors.New("the private branch of the worktree was not deleted")

type Manager interface {
	Fetch(ctx context.Context, source, upstream string) error
	Create(ctx context.Context, source, dir, branch, upstream string) error
	Remove(ctx context.Context, source, dir, branch string) error
}

type Git struct {
	Run execx.Runner
}

func New() *Git {
	return &Git{Run: execx.Run}
}

func (g *Git) git(ctx context.Context, dir string, args ...string) (string, error) {
	run := g.Run
	if run == nil {
		run = execx.Run
	}
	full := append([]string{"-C", dir}, args...)
	out, err := run(ctx, "git", full...)
	if err != nil {
		return strings.TrimSpace(out), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(out), nil
}

func splitUpstream(upstream string) (remote, branch string, err error) {
	remote, branch, ok := strings.Cut(upstream, "/")
	if !ok || remote == "" || branch == "" {
		return "", "", fmt.Errorf("upstream %q must be remote/branch", upstream)
	}
	return remote, branch, nil
}

func (g *Git) Fetch(ctx context.Context, source, upstream string) error {
	remote, ref, err := splitUpstream(upstream)
	if err != nil {
		return err
	}
	if _, err := g.git(ctx, source, "fetch", "-q", remote, ref); err != nil {
		return fmt.Errorf("fetch %s: %w (was the branch pushed?)", upstream, err)
	}
	return nil
}

func (g *Git) Create(ctx context.Context, source, dir, branch, upstream string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o750); err != nil {
		return fmt.Errorf("create worktree dir: %w", err)
	}
	if _, err := g.git(ctx, source, "worktree", "add", "-q", "-B", branch, dir, upstream); err != nil {
		return err
	}
	return nil
}

func (g *Git) Remove(ctx context.Context, source, dir, branch string) error {
	switch _, err := os.Stat(dir); {
	case err == nil:
		if _, rmErr := g.git(ctx, source, "worktree", "remove", "--force", dir); rmErr != nil {
			if err := removeWorktreeDir(dir); err != nil {
				return errors.Join(rmErr, err)
			}
		}
	case !os.IsNotExist(err):
		return fmt.Errorf("read %s: %w", dir, err)
	}
	_, _ = g.git(ctx, source, "worktree", "prune")
	return g.removeBranch(ctx, source, branch)
}

func (g *Git) removeBranch(ctx context.Context, source, branch string) error {
	if branch == "" {
		return nil
	}
	if !g.branchExists(ctx, source, branch) {
		return nil
	}
	if _, err := g.git(ctx, source, "branch", "-q", "-D", branch); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrBranchLeft, branch, err)
	}
	return nil
}

func (g *Git) branchExists(ctx context.Context, source, branch string) bool {
	_, err := g.git(ctx, source, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func removeWorktreeDir(dir string) error {
	if !isWorktreeDir(dir) {
		return fmt.Errorf("%s is not a git worktree, left in place", dir)
	}
	return os.RemoveAll(dir)
}

const maxGitFileSize = 4096

func isWorktreeDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil || info.IsDir() || info.Size() > maxGitFileSize {
		return false
	}
	b, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(string(b)), "gitdir:")
}
