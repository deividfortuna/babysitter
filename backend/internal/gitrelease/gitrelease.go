package gitrelease

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/execx"
)

var ErrConflict = errors.New("the rebase conflicts")

type ConflictError struct {
	Files []string
}

func (e *ConflictError) Error() string {
	return ErrConflict.Error() + " in " + strings.Join(e.Files, ", ")
}

func (e *ConflictError) Is(target error) bool { return target == ErrConflict }

var ErrLeaseRefused = errors.New("the pull request branch moved, so the lease refused the push")

type Git interface {
	Fetch(ctx context.Context, dir, branch string) (string, error)
	Head(ctx context.Context, dir string) (string, error)
	Contains(ctx context.Context, dir, sha, ancestor string) (bool, error)
	MergeBase(ctx context.Context, dir, a, b string) (string, error)
	FastForward(ctx context.Context, dir, sha string) error
	Reset(ctx context.Context, dir, sha string) error
	HasMerges(ctx context.Context, dir, since, sha string) (bool, error)
	Missing(ctx context.Context, dir, work, head, since string) ([]string, error)
	Push(ctx context.Context, dir string, p Push) error
	Rebase(ctx context.Context, dir, onto string) error
	Discard(ctx context.Context, dir, sha string) error
	Log(ctx context.Context, dir, from, to string) ([]Commit, error)
	Files(ctx context.Context, dir, from, to string) ([]File, error)
	Diff(ctx context.Context, dir, from, to string, limit int) (string, bool, error)
	Dirty(ctx context.Context, dir string) ([]string, error)
}

type Commit struct {
	SHA     string
	Subject string
}

type File struct {
	Path    string
	Status  string
	Added   int
	Deleted int
}

type Push struct {
	SHA    string
	Branch string
	Lease  string
}

const defaultTimeout = 2 * time.Minute

type Runner struct {
	Run     execx.DirRunner
	Timeout time.Duration
}

func New() *Runner {
	return &Runner{Run: execx.RunIn, Timeout: defaultTimeout}
}

var noPrompt = []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}

func (g *Runner) git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := g.raw(ctx, dir, args...)
	return strings.TrimSpace(out), err
}

func (g *Runner) raw(ctx context.Context, dir string, args ...string) (string, error) {
	run := g.Run
	if run == nil {
		run = execx.RunIn
	}
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(g.Timeout, defaultTimeout))
	defer cancel()
	out, err := run(ctx, dir, "", noPrompt, "git", args...)
	if err != nil {
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func (g *Runner) Fetch(ctx context.Context, dir, branch string) (string, error) {
	tracking := "refs/remotes/origin/" + branch
	if _, err := g.git(ctx, dir, "fetch", "-q", "--no-tags", "origin", "+refs/heads/"+branch+":"+tracking); err != nil {
		return "", err
	}
	return g.git(ctx, dir, "rev-parse", "--verify", "-q", tracking+"^{commit}")
}

func (g *Runner) Head(ctx context.Context, dir string) (string, error) {
	return g.git(ctx, dir, "rev-parse", "--verify", "-q", "HEAD^{commit}")
}

func (g *Runner) Contains(ctx context.Context, dir, sha, ancestor string) (bool, error) {
	_, err := g.git(ctx, dir, "merge-base", "--is-ancestor", ancestor, sha)
	switch execx.ExitCode(err) {
	case 0:
		return true, nil
	case 1:
		return false, nil
	}
	return false, err
}

func (g *Runner) MergeBase(ctx context.Context, dir, a, b string) (string, error) {
	return g.git(ctx, dir, "merge-base", a, b)
}

func (g *Runner) FastForward(ctx context.Context, dir, sha string) error {
	_, err := g.git(ctx, dir, "merge", "-q", "--ff-only", sha)
	return err
}

func (g *Runner) Reset(ctx context.Context, dir, sha string) error {
	_, err := g.git(ctx, dir, "reset", "-q", "--keep", sha)
	return err
}

func (g *Runner) HasMerges(ctx context.Context, dir, since, sha string) (bool, error) {
	out, err := g.git(ctx, dir, "rev-list", "--merges", "-n", "1", since+".."+sha)
	return out != "", err
}

func (g *Runner) Missing(ctx context.Context, dir, work, head, since string) ([]string, error) {
	args := []string{"cherry", work, head}
	if since != "" {
		args = append(args, since)
	}
	out, err := g.git(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	var missing []string
	for line := range strings.Lines(out) {
		if sha, ok := strings.CutPrefix(strings.TrimSpace(line), "+ "); ok {
			missing = append(missing, sha)
		}
	}
	return missing, nil
}

func (g *Runner) Push(ctx context.Context, dir string, p Push) error {
	args := []string{"push", "--no-verify"}
	if p.Lease != "" {
		args = append(args, "--force-with-lease=refs/heads/"+p.Branch+":"+p.Lease)
	}
	args = append(args, "origin", p.SHA+":refs/heads/"+p.Branch)
	out, err := g.git(ctx, dir, args...)
	if err != nil && strings.Contains(out+err.Error(), "stale info") {
		return fmt.Errorf("%w: %w", ErrLeaseRefused, err)
	}
	return err
}

func (g *Runner) Discard(ctx context.Context, dir, sha string) error {
	_, err := g.git(ctx, dir, "reset", "-q", "--hard", sha)
	return err
}

func (g *Runner) Log(ctx context.Context, dir, from, to string) ([]Commit, error) {
	out, err := g.git(ctx, dir, "log", "--reverse", "--format=%H%x09%s", from+".."+to)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for line := range strings.Lines(out) {
		sha, subject, _ := strings.Cut(strings.TrimRight(line, "\n"), "\t")
		commits = append(commits, Commit{SHA: sha, Subject: subject})
	}
	return commits, nil
}

func (g *Runner) Dirty(ctx context.Context, dir string) ([]string, error) {
	out, err := g.raw(ctx, dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	var files []string
	for line := range strings.Lines(out) {
		files = append(files, strings.TrimRight(line, "\n"))
	}
	return files, nil
}

func (g *Runner) Files(ctx context.Context, dir, from, to string) ([]File, error) {
	statuses, err := g.git(ctx, dir, "diff", "--no-renames", "--name-status", from, to)
	if err != nil {
		return nil, err
	}
	counts, err := g.git(ctx, dir, "diff", "--no-renames", "--numstat", from, to)
	if err != nil {
		return nil, err
	}
	lines := map[string][2]int{}
	for line := range strings.Lines(counts) {
		fields := strings.SplitN(strings.TrimRight(line, "\n"), "\t", 3)
		if len(fields) != 3 {
			continue
		}
		added, _ := strconv.Atoi(fields[0])
		deleted, _ := strconv.Atoi(fields[1])
		lines[fields[2]] = [2]int{added, deleted}
	}
	var files []File
	for line := range strings.Lines(statuses) {
		status, path, ok := strings.Cut(strings.TrimRight(line, "\n"), "\t")
		if !ok {
			continue
		}
		n := lines[path]
		files = append(files, File{Path: path, Status: status, Added: n[0], Deleted: n[1]})
	}
	return files, nil
}

func (g *Runner) Diff(ctx context.Context, dir, from, to string, limit int) (string, bool, error) {
	out, err := g.raw(ctx, dir, "diff", "--no-color", "--no-ext-diff", "--no-renames", from, to)
	if err != nil {
		return "", false, err
	}
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
}

func (g *Runner) Rebase(ctx context.Context, dir, onto string) error {
	_, err := g.git(ctx, dir, "rebase", "-q", onto)
	if err == nil || !g.rebasing(ctx, dir) {
		return err
	}
	files, _ := g.git(ctx, dir, "diff", "--name-only", "--diff-filter=U")
	if _, abortErr := g.git(ctx, dir, "rebase", "--abort"); abortErr != nil {
		return fmt.Errorf("%w, and the abort failed: %w", err, abortErr)
	}
	if files == "" {
		return err
	}
	return &ConflictError{Files: strings.Fields(files)}
}

func (g *Runner) rebasing(ctx context.Context, dir string) bool {
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		path, err := g.git(ctx, dir, "rev-parse", "--git-path", state)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
