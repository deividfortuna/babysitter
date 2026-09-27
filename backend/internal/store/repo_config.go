package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghclient"
)

var (
	ErrInvalidRepoConfig = errors.New("invalid repository configuration")
	ErrAlreadyClaimed    = errors.New("the pull request was already taken by auto start")
)

type DependabotApproval string

const (
	ApproveNever DependabotApproval = "never"
	ApproveAsk   DependabotApproval = "ask"
	ApproveGreen DependabotApproval = "green"
)

var DependabotApprovals = []DependabotApproval{ApproveNever, ApproveAsk, ApproveGreen}

func (a DependabotApproval) Valid() bool { return slices.Contains(DependabotApprovals, a) }

type WatchOverrides struct {
	Provider          string
	Model             string
	ApprovalMode      ApprovalMode
	MergeMethod       string
	ApprovalsSet      bool
	Approvals         *int
	IncludeExisting   *bool
	AutoApproveRebase *bool
	IncludeOwn        *bool
	KeepWorktree      *bool
}

type RepoConfig struct {
	RepoID             int64
	CheckoutDir        string
	OwnSince           *time.Time
	IncludeDrafts      bool
	DependabotSince    *time.Time
	Overrides          WatchOverrides
	DependabotScope    dependabot.Level
	DependabotApproval DependabotApproval
	DependabotLimit    int
}

func DefaultRepoConfig(repoID int64) RepoConfig {
	return RepoConfig{
		RepoID:             repoID,
		DependabotScope:    dependabot.Patch,
		DependabotApproval: ApproveNever,
		DependabotLimit:    1,
	}
}

func (c RepoConfig) OwnOn() bool { return c.OwnSince != nil }

func (c RepoConfig) DependabotOn() bool { return c.DependabotSince != nil }

func (c RepoConfig) AutoStarts() bool { return c.CheckoutDir != "" && (c.OwnOn() || c.DependabotOn()) }

func (c RepoConfig) Validate() error {
	o := c.Overrides
	switch {
	case c.CheckoutDir == "" && (c.OwnOn() || c.DependabotOn()):
		return fmt.Errorf("%w: set the checkout before you turn on auto start", ErrInvalidRepoConfig)
	case !c.DependabotScope.Valid():
		return fmt.Errorf("%w: unknown Dependabot merge scope %q: use patch, minor or major", ErrInvalidRepoConfig, c.DependabotScope)
	case !c.DependabotApproval.Valid():
		return fmt.Errorf("%w: unknown Dependabot approval %q: use never, ask or green", ErrInvalidRepoConfig, c.DependabotApproval)
	case c.DependabotLimit < 1:
		return fmt.Errorf("%w: the Dependabot limit must be 1 or more, got %d", ErrInvalidRepoConfig, c.DependabotLimit)
	case o.ApprovalMode != "" && !o.ApprovalMode.Valid():
		return fmt.Errorf("%w: unknown approval mode %q: use auto or manual", ErrInvalidRepoConfig, o.ApprovalMode)
	case o.MergeMethod != "" && !slices.Contains(ghclient.MergeMethodsKnown, o.MergeMethod):
		return fmt.Errorf("%w: unknown merge method %q: use %s", ErrInvalidRepoConfig, o.MergeMethod, strings.Join(ghclient.MergeMethodsKnown, ", "))
	case o.Approvals != nil && *o.Approvals < 0:
		return fmt.Errorf("%w: the approvals must be 0 or more, got %d", ErrInvalidRepoConfig, *o.Approvals)
	}
	return nil
}

const repoConfigColumns = `repo_id, checkout_dir, own_since, include_drafts, dependabot_since,
	provider, model, approval_mode, merge_method, approvals_set, approvals_count, include_existing,
	dependabot_scope, dependabot_approval, dependabot_limit, auto_approve_rebase, include_own, keep_worktree`

func (s *Store) GetRepoConfig(ctx context.Context, repoID int64) (RepoConfig, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+repoConfigColumns+" FROM repo_config WHERE repo_id = ?", repoID)
	c, err := scanRepoConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultRepoConfig(repoID), nil
	}
	if err != nil {
		return RepoConfig{}, fmt.Errorf("get repository configuration: %w", err)
	}
	return c, nil
}

func (s *Store) SaveRepoConfig(ctx context.Context, c RepoConfig) (RepoConfig, error) {
	if err := c.Validate(); err != nil {
		return RepoConfig{}, err
	}
	o := c.Overrides
	_, err := s.db.ExecContext(ctx, `
INSERT INTO repo_config (`+repoConfigColumns+`)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (repo_id) DO UPDATE SET
	checkout_dir = excluded.checkout_dir,
	own_since = excluded.own_since,
	include_drafts = excluded.include_drafts,
	dependabot_since = excluded.dependabot_since,
	provider = excluded.provider,
	model = excluded.model,
	approval_mode = excluded.approval_mode,
	merge_method = excluded.merge_method,
	approvals_set = excluded.approvals_set,
	approvals_count = excluded.approvals_count,
	include_existing = excluded.include_existing,
	dependabot_scope = excluded.dependabot_scope,
	dependabot_approval = excluded.dependabot_approval,
	dependabot_limit = excluded.dependabot_limit,
	auto_approve_rebase = excluded.auto_approve_rebase,
	include_own = excluded.include_own,
	keep_worktree = excluded.keep_worktree`,
		c.RepoID, c.CheckoutDir, timePtrToDB(c.OwnSince), c.IncludeDrafts, timePtrToDB(c.DependabotSince),
		o.Provider, o.Model, o.ApprovalMode, o.MergeMethod, o.ApprovalsSet, o.Approvals, o.IncludeExisting,
		c.DependabotScope, c.DependabotApproval, c.DependabotLimit, o.AutoApproveRebase, o.IncludeOwn, o.KeepWorktree)
	if isForeignKeyFailure(err) {
		return RepoConfig{}, ErrRepoNotFound
	}
	if err != nil {
		return RepoConfig{}, fmt.Errorf("save repository configuration: %w", err)
	}
	s.publish(events.RepoChanged, s.repoFullName(ctx, c.RepoID), 0)
	return s.GetRepoConfig(ctx, c.RepoID)
}

func scanRepoConfig(sc scanner) (RepoConfig, error) {
	var (
		c                  RepoConfig
		ownSince, depSince sql.NullString
		approvals          sql.NullInt64
		includeExisting    sql.NullBool
		autoRebase         sql.NullBool
		includeOwn         sql.NullBool
		keepWorktree       sql.NullBool
		o                  = &c.Overrides
	)
	err := sc.Scan(&c.RepoID, &c.CheckoutDir, &ownSince, &c.IncludeDrafts, &depSince,
		&o.Provider, &o.Model, &o.ApprovalMode, &o.MergeMethod, &o.ApprovalsSet, &approvals, &includeExisting,
		&c.DependabotScope, &c.DependabotApproval, &c.DependabotLimit, &autoRebase, &includeOwn, &keepWorktree)
	if err != nil {
		return RepoConfig{}, err
	}
	if c.OwnSince, err = timePtrFromDB(ownSince); err != nil {
		return RepoConfig{}, err
	}
	if c.DependabotSince, err = timePtrFromDB(depSince); err != nil {
		return RepoConfig{}, err
	}
	if approvals.Valid {
		n := int(approvals.Int64)
		o.Approvals = &n
	}
	o.IncludeExisting = boolPtrFromDB(includeExisting)
	o.AutoApproveRebase = boolPtrFromDB(autoRebase)
	o.IncludeOwn = boolPtrFromDB(includeOwn)
	o.KeepWorktree = boolPtrFromDB(keepWorktree)
	return c, nil
}

func boolPtrFromDB(v sql.NullBool) *bool {
	if !v.Valid {
		return nil
	}
	return &v.Bool
}

func (s *Store) ClaimAutoStart(ctx context.Context, repoID int64, number int, now, staleBefore time.Time) error {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO auto_start_claims (repo_id, number, claimed_at) VALUES (?, ?, ?)
ON CONFLICT (repo_id, number) DO UPDATE SET claimed_at = excluded.claimed_at
WHERE auto_start_claims.claimed_at < ?`,
		repoID, number, timeToDB(now), timeToDB(staleBefore))
	if err != nil {
		return fmt.Errorf("claim auto start: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return ErrAlreadyClaimed
	}
	return nil
}

func (s *Store) ReleaseAutoStart(ctx context.Context, repoID int64, number int) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM auto_start_claims WHERE repo_id = ? AND number = ?", repoID, number); err != nil {
		return fmt.Errorf("release auto start: %w", err)
	}
	return nil
}

func (s *Store) AutoStartClaimed(ctx context.Context, repoID int64, number int, since time.Time) (bool, error) {
	var claimed bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM auto_start_claims WHERE repo_id = ? AND number = ? AND claimed_at >= ?)",
		repoID, number, timeToDB(since)).Scan(&claimed)
	if err != nil {
		return false, fmt.Errorf("auto start claimed: %w", err)
	}
	return claimed, nil
}
