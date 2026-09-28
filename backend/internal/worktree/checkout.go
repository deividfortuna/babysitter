package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/execx"
	"github.com/deividfortuna/babysitter/internal/gitrepo"
	"github.com/deividfortuna/babysitter/internal/keyedlock"
)

const (
	gitHubHelperConfig  = "credential.https://github.com.helper=!gh auth git-credential"
	gitTimeout          = 10 * time.Minute
	gitConfigKeyMissing = 5
)

type Checkouts struct {
	root  string
	url   func(repo string) string
	locks keyedlock.Locks[string]
}

func NewCheckouts(root string) *Checkouts {
	return &Checkouts{root: root, url: gitHubURL}
}

func gitHubURL(repo string) string {
	return "https://github.com/" + repo + ".git"
}

func (c *Checkouts) Ensure(ctx context.Context, repo string) (string, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(c.root, owner, name)
	unlock := c.locks.Lock(dir)
	defer unlock()
	if isCheckout(dir) {
		return dir, restoreOrigin(ctx, dir, c.url(owner+"/"+name))
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%s is not a git checkout, left in place", dir)
	}
	if err := c.clone(ctx, owner+"/"+name, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func restoreOrigin(ctx context.Context, dir, url string) error {
	if _, err := execx.RunIn(ctx, dir, "", nil, "git", "remote", "set-url", "origin", url); err != nil {
		return fmt.Errorf("restore the origin of %s: %w", dir, err)
	}
	_, err := execx.RunIn(ctx, dir, "", nil, "git", "config", "--unset-all", "remote.origin.pushurl")
	if err != nil && execx.ExitCode(err) != gitConfigKeyMissing {
		return fmt.Errorf("restore the origin of %s: %w", dir, err)
	}
	return nil
}

func (c *Checkouts) clone(ctx context.Context, repo, dir string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return fmt.Errorf("create checkout dir: %w", err)
	}
	tmp, err := os.MkdirTemp(parent, filepath.Base(dir)+".clone-")
	if err != nil {
		return fmt.Errorf("create checkout dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	url := c.url(repo)
	if _, err := execx.RunIn(ctx, "", "", gitrepo.NoPromptEnv, "git", "clone", "-q", "--no-checkout", "--filter=blob:none",
		"--config", gitHubHelperConfig, "--", url, tmp); err != nil {
		return fmt.Errorf("clone %s: %w", url, err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		return fmt.Errorf("move the clone of %s: %w", repo, err)
	}
	return nil
}

func splitRepo(repo string) (owner, name string, err error) {
	owner, name, _ = strings.Cut(strings.ToLower(repo), "/")
	if !safePathPart(owner) || !safePathPart(name) {
		return "", "", fmt.Errorf("repository %q must be owner/name", repo)
	}
	return owner, name, nil
}

func safePathPart(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`)
}

func isCheckout(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}
