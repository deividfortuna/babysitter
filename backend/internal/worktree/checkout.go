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
	gitHubHelperKey     = "credential.https://github.com.helper"
	ghHelper            = "!gh auth git-credential"
	gitTimeout          = 10 * time.Minute
	gitConfigKeyMissing = 5
)

type Checkouts struct {
	Auth   gitrepo.AuthEnv
	Helper string

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
		if err := restoreOrigin(ctx, dir, c.url(owner+"/"+name)); err != nil {
			return "", err
		}
		return dir, c.restoreHelpers(ctx, dir)
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

func (c *Checkouts) helpers() []string {
	if c.Helper == "" {
		return []string{ghHelper}
	}
	return []string{c.Helper, ghHelper}
}

func (c *Checkouts) restoreHelpers(ctx context.Context, dir string) error {
	_, err := execx.RunIn(ctx, dir, "", nil, "git", "config", "--unset-all", gitHubHelperKey)
	if err != nil && execx.ExitCode(err) != gitConfigKeyMissing {
		return fmt.Errorf("restore the credential helpers of %s: %w", dir, err)
	}
	for _, helper := range c.helpers() {
		if _, err := execx.RunIn(ctx, dir, "", nil, "git", "config", "--add", gitHubHelperKey, helper); err != nil {
			return fmt.Errorf("restore the credential helpers of %s: %w", dir, err)
		}
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
	env, err := c.Auth.Env(ctx, "clone")
	if err != nil {
		return fmt.Errorf("clone %s: %w", url, err)
	}
	args := []string{"clone", "-q", "--no-checkout", "--filter=blob:none"}
	for _, helper := range c.helpers() {
		args = append(args, "--config", gitHubHelperKey+"="+helper)
	}
	args = append(args, "--", url, tmp)
	if _, err := execx.RunIn(ctx, "", "", env, "git", args...); err != nil {
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
