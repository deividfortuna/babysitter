package autostart

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/gitrepo"
	"github.com/deividfortuna/babysitter/internal/store"
)

var ErrBadCheckout = errors.New("invalid checkout")

type checkoutError struct{ reason string }

func (e *checkoutError) Error() string { return e.reason }

func (e *checkoutError) Is(target error) bool { return target == ErrBadCheckout }

func badCheckout(format string, args ...any) error {
	return &checkoutError{reason: fmt.Sprintf(format, args...)}
}

func Queue(ctx context.Context, st QueueStore, repo store.Repo) ([]store.PullRequest, error) {
	cfg, err := st.GetRepoConfig(ctx, repo.ID)
	if err != nil || !cfg.AutoStarts() || !cfg.DependabotOn() {
		return []store.PullRequest{}, err
	}
	prs, err := st.OpenPRs(ctx, repo.ID)
	if err != nil {
		return nil, err
	}
	out := []store.PullRequest{}
	for _, c := range eligible(cfg, "", prs) {
		if c.pr.Fork {
			continue
		}
		taken, err := isTaken(ctx, st, repo, c.pr.Number, time.Now())
		if err != nil {
			return nil, err
		}
		if !taken {
			out = append(out, c.pr)
		}
	}
	slices.SortStableFunc(out, oldestFirst)
	return out, nil
}

func CheckCheckout(ctx context.Context, dir string, repo store.Repo) error {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return badCheckout("%s is not a folder", dir)
	}
	origin, err := gitrepo.RemoteURL(ctx, dir, "origin")
	if err != nil {
		return badCheckout("%s is not a git checkout with an origin remote", dir)
	}
	owner, name, err := store.ParseFullName(origin)
	if err != nil || !strings.EqualFold(owner+"/"+name, repo.FullName()) {
		return badCheckout("%s has no remote for %s", dir, repo.FullName())
	}
	return nil
}
