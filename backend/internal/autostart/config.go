package autostart

import (
	"context"
	"time"

	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/store"
)

type Change struct {
	CheckoutDir         *string
	AutoStartMine       *bool
	IncludeDrafts       *bool
	AutoWatchDependabot *bool
	Overrides           *store.WatchOverrides
	DependabotScope     *dependabot.Level
	DependabotApproval  *store.DependabotApproval
	DependabotLimit     *int
}

type ConfigStore interface {
	GetRepoConfig(ctx context.Context, repoID int64) (store.RepoConfig, error)
	SaveRepoConfig(ctx context.Context, c store.RepoConfig) (store.RepoConfig, error)
}

func Configure(ctx context.Context, st ConfigStore, repo store.Repo, c Change, now time.Time) (store.RepoConfig, error) {
	cfg, err := st.GetRepoConfig(ctx, repo.ID)
	if err != nil {
		return store.RepoConfig{}, err
	}
	if c.CheckoutDir != nil && *c.CheckoutDir != "" && *c.CheckoutDir != cfg.CheckoutDir {
		if err := CheckCheckout(ctx, *c.CheckoutDir, repo); err != nil {
			return store.RepoConfig{}, err
		}
	}
	return st.SaveRepoConfig(ctx, apply(cfg, c, now))
}

func apply(cfg store.RepoConfig, c Change, now time.Time) store.RepoConfig {
	set(&cfg.CheckoutDir, c.CheckoutDir)
	set(&cfg.IncludeDrafts, c.IncludeDrafts)
	set(&cfg.Overrides, c.Overrides)
	set(&cfg.DependabotScope, c.DependabotScope)
	set(&cfg.DependabotApproval, c.DependabotApproval)
	set(&cfg.DependabotLimit, c.DependabotLimit)
	cfg.OwnSince = toggle(cfg.OwnSince, c.AutoStartMine, now)
	cfg.DependabotSince = toggle(cfg.DependabotSince, c.AutoWatchDependabot, now)
	return cfg
}

func set[T any](field *T, value *T) {
	if value != nil {
		*field = *value
	}
}

func toggle(since *time.Time, on *bool, now time.Time) *time.Time {
	switch {
	case on == nil:
		return since
	case !*on:
		return nil
	case since == nil:
		at := now.UTC().Truncate(time.Second)
		return &at
	default:
		return since
	}
}
