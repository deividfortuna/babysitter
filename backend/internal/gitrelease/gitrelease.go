package gitrelease

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/execx"
	"github.com/deividfortuna/babysitter/internal/gitrepo"
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
	Parent(ctx context.Context, dir, sha string) (string, error)
	Contains(ctx context.Context, dir, sha, ancestor string) (bool, error)
	MergeBase(ctx context.Context, dir, a, b string) (string, error)
	FastForward(ctx context.Context, dir, sha string) error
	Reset(ctx context.Context, dir, sha string) error
	HasMerges(ctx context.Context, dir, since, sha string) (bool, error)
	Missing(ctx context.Context, dir, work, head, since string) ([]string, error)
	Push(ctx context.Context, dir string, p Push) error
	Rebase(ctx context.Context, dir, onto string) error
	Merge(ctx context.Context, dir, sha string) error
	Discard(ctx context.Context, dir, sha string) error
	Log(ctx context.Context, dir, from, to string) ([]Commit, error)
	Files(ctx context.Context, dir, from, to string) ([]File, error)
	Diff(ctx context.Context, dir, from, to string, limit int, paths ...string) (string, bool, error)
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
	Binary  bool
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
	Auth    gitrepo.AuthEnv
}

func New() *Runner {
	return &Runner{Run: execx.RunIn, Timeout: defaultTimeout}
}

func (g *Runner) git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := g.raw(ctx, dir, args...)
	return strings.TrimSpace(out), err
}

func (g *Runner) raw(ctx context.Context, dir string, args ...string) (string, error) {
	env, err := g.Auth.Env(ctx, args[0])
	if err != nil {
		return "", err
	}
	run := g.Run
	if run == nil {
		run = execx.RunIn
	}
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(g.Timeout, defaultTimeout))
	defer cancel()
	out, err := run(ctx, dir, "", env, "git", args...)
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

func (g *Runner) Parent(ctx context.Context, dir, sha string) (string, error) {
	return g.git(ctx, dir, "rev-parse", "--verify", "-q", sha+"^1^{commit}")
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

var diffFlags = []string{"--no-renames", "--no-color", "--no-ext-diff", "--no-textconv", "--src-prefix=a/", "--dst-prefix=b/"}

func (g *Runner) diff(ctx context.Context, dir string, args ...string) (string, error) {
	return g.raw(ctx, dir, slices.Concat([]string{"--literal-pathspecs", "diff"}, diffFlags, args)...)
}

func (g *Runner) Files(ctx context.Context, dir, from, to string) ([]File, error) {
	statuses, err := g.diff(ctx, dir, "-z", "--name-status", from, to)
	if err != nil {
		return nil, err
	}
	counts, err := g.diff(ctx, dir, "-z", "--numstat", from, to)
	if err != nil {
		return nil, err
	}
	stats := numstat(counts)
	fields := strings.Split(strings.TrimSuffix(statuses, "\x00"), "\x00")
	var files []File
	for pair := range slices.Chunk(fields, 2) {
		if len(pair) != 2 {
			continue
		}
		file := stats[pair[1]]
		file.Path, file.Status = pair[1], pair[0]
		files = append(files, file)
	}
	return files, nil
}

func numstat(out string) map[string]File {
	stats := map[string]File{}
	for record := range strings.SplitSeq(out, "\x00") {
		fields := strings.SplitN(record, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		added, addedErr := strconv.Atoi(fields[0])
		deleted, deletedErr := strconv.Atoi(fields[1])
		binary := addedErr != nil && deletedErr != nil
		stats[fields[2]] = File{Added: added, Deleted: deleted, Binary: binary}
	}
	return stats
}

func (g *Runner) Diff(ctx context.Context, dir, from, to string, limit int, paths ...string) (string, bool, error) {
	out, err := g.diff(ctx, dir, slices.Concat([]string{from, to, "--"}, paths)...)
	if err != nil {
		return "", false, err
	}
	if len(out) <= limit {
		return out, false, nil
	}
	return wholeFiles(out, limit), true, nil
}

const fileStart = "\ndiff --git "

func wholeFiles(patch string, limit int) string {
	window := patch[:min(len(patch), limit+len(fileStart)-1)]
	return patch[:strings.LastIndex(window, fileStart)+1]
}

func (g *Runner) Rebase(ctx context.Context, dir, onto string) error {
	return g.bringIn(ctx, dir, "rebase", g.rebasing, "rebase", "-q", onto)
}

func (g *Runner) Merge(ctx context.Context, dir, sha string) error {
	return g.bringIn(ctx, dir, "merge", g.merging, "-c", "core.hooksPath="+os.DevNull, "merge", "-q", "--no-edit", "--no-ff", sha)
}

func (g *Runner) bringIn(ctx context.Context, dir, command string, inProgress func(context.Context, string) bool, args ...string) error {
	_, err := g.git(ctx, dir, args...)
	if err == nil || !inProgress(ctx, dir) {
		return err
	}
	files, _ := g.git(ctx, dir, "diff", "--name-only", "--diff-filter=U")
	if _, abortErr := g.git(ctx, dir, command, "--abort"); abortErr != nil {
		return fmt.Errorf("%w, and the abort failed: %w", err, abortErr)
	}
	if files == "" {
		return err
	}
	return &ConflictError{Files: strings.Fields(files)}
}

func (g *Runner) merging(ctx context.Context, dir string) bool {
	_, err := g.git(ctx, dir, "rev-parse", "-q", "--verify", "MERGE_HEAD")
	return err == nil
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
