package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghclient"
)

type Settings struct {
	PollInterval                time.Duration
	WatchInterval               time.Duration
	WatchMaxInterval            time.Duration
	CheckMaxInterval            time.Duration
	ApprovalsRequired           *int
	MergeMethod                 string
	IncludeExisting             bool
	IncludeOwn                  bool
	KeepWorktree                bool
	NotificationsEnabled        bool
	NotificationsBackgroundOnly bool
	MutedNotificationKinds      []NotificationKind
	SilentNotificationKinds     []NotificationKind
	ApprovalMode                ApprovalMode
	AutoApproveRebase           bool
	Provider                    string
	Model                       string
	Effort                      string
	BranchUpdate                BranchUpdate
	UpdateOnGitHub              bool
	ScreenReader                bool
}

type ApprovalMode string

const (
	ApprovalAuto   ApprovalMode = "auto"
	ApprovalManual ApprovalMode = "manual"
)

var ApprovalModes = []ApprovalMode{ApprovalAuto, ApprovalManual}

func (m ApprovalMode) Valid() bool { return slices.Contains(ApprovalModes, m) }

type BranchUpdate string

const (
	BranchRebase BranchUpdate = "rebase"
	BranchMerge  BranchUpdate = "merge"
)

var BranchUpdates = []BranchUpdate{BranchRebase, BranchMerge}

func (b BranchUpdate) Valid() bool { return slices.Contains(BranchUpdates, b) }

const (
	MinInterval = 10 * time.Second
	MaxInterval = 24 * time.Hour
)

var ErrInvalidSettings = errors.New("invalid settings")

var SettingsProviders = []string{"claude", "copilot"}

func DefaultSettings() Settings {
	return Settings{
		PollInterval:                time.Minute,
		WatchInterval:               3 * time.Minute,
		WatchMaxInterval:            15 * time.Minute,
		CheckMaxInterval:            15 * time.Minute,
		NotificationsEnabled:        true,
		NotificationsBackgroundOnly: true,
		ApprovalMode:                ApprovalManual,
		Provider:                    "claude",
		BranchUpdate:                BranchRebase,
		UpdateOnGitHub:              true,
		ScreenReader:                false,
	}
}

func (s Settings) Validate() error {
	if s.PollInterval < MinInterval || s.PollInterval > MaxInterval {
		return fmt.Errorf("%w: the repository poll interval must be between %s and %s, got %s", ErrInvalidSettings, MinInterval, MaxInterval, s.PollInterval)
	}
	if s.WatchInterval < MinInterval || s.WatchInterval > MaxInterval {
		return fmt.Errorf("%w: the watch poll interval must be between %s and %s, got %s", ErrInvalidSettings, MinInterval, MaxInterval, s.WatchInterval)
	}
	if s.WatchMaxInterval < s.WatchInterval || s.WatchMaxInterval > MaxInterval {
		return fmt.Errorf("%w: the longest watch poll interval must be between the watch poll interval %s and %s, got %s", ErrInvalidSettings, s.WatchInterval, MaxInterval, s.WatchMaxInterval)
	}
	if s.CheckMaxInterval < MinInterval || s.CheckMaxInterval > MaxInterval {
		return fmt.Errorf("%w: the longest check read interval must be between %s and %s, got %s", ErrInvalidSettings, MinInterval, MaxInterval, s.CheckMaxInterval)
	}
	if s.MergeMethod != "" && !slices.Contains(ghclient.MergeMethodsKnown, s.MergeMethod) {
		return fmt.Errorf("%w: unknown merge method %q: use %s", ErrInvalidSettings, s.MergeMethod, strings.Join(ghclient.MergeMethodsKnown, ", "))
	}
	if !slices.Contains(SettingsProviders, s.Provider) {
		return fmt.Errorf("%w: unknown provider %q: use %s", ErrInvalidSettings, s.Provider, strings.Join(SettingsProviders, " or "))
	}
	if !s.ApprovalMode.Valid() {
		return fmt.Errorf("%w: unknown approval mode %q: use auto or manual", ErrInvalidSettings, s.ApprovalMode)
	}
	if !s.BranchUpdate.Valid() {
		return fmt.Errorf("%w: unknown branch update %q: use rebase or merge", ErrInvalidSettings, s.BranchUpdate)
	}
	if s.ApprovalsRequired != nil && *s.ApprovalsRequired < 0 {
		return fmt.Errorf("%w: the approvals must be 0 or more, got %d", ErrInvalidSettings, *s.ApprovalsRequired)
	}
	if err := validateKinds(s.MutedNotificationKinds); err != nil {
		return err
	}
	return validateKinds(s.SilentNotificationKinds)
}

func validateKinds(kinds []NotificationKind) error {
	for _, kind := range kinds {
		if !kind.Valid() {
			return fmt.Errorf("%w: unknown notification kind %q: use %s", ErrInvalidSettings, kind, JoinKinds())
		}
	}
	return nil
}

func (s Settings) ShowsNotification(k NotificationKind) bool {
	return s.NotificationsEnabled && !slices.Contains(s.MutedNotificationKinds, k)
}

func (s Settings) PlaysSound(k NotificationKind) bool {
	return !slices.Contains(s.SilentNotificationKinds, k)
}

func kindsValue(kinds []NotificationKind, stored string) string {
	out := make([]string, 0, len(kinds))
	for _, known := range NotificationKinds {
		if slices.Contains(kinds, known) {
			out = append(out, string(known))
		}
	}
	for part := range strings.SplitSeq(stored, ",") {
		part = strings.TrimSpace(part)
		if isUnknownKind(part) && !slices.Contains(out, part) {
			out = append(out, part)
		}
	}
	return strings.Join(out, ",")
}

func isUnknownKind(part string) bool {
	return part != "" && !NotificationKind(part).Valid()
}

func parseKinds(value string) []NotificationKind {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]NotificationKind, 0, len(parts))
	for _, part := range parts {
		kind := NotificationKind(part)
		if kind.Valid() {
			out = append(out, kind)
		}
	}
	return out
}

const settingsColumns = "poll_interval_ms, watch_interval_ms, watch_max_interval_ms, check_max_interval_ms, approvals_required, merge_method, include_existing, include_own, keep_worktree, notifications_enabled, notifications_background_only, muted_notification_kinds, silent_notification_kinds, approval_mode, auto_approve_rebase, provider, model, effort, branch_update, update_on_github, screen_reader"

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	var (
		out       Settings
		pollMS    int64
		watchMS   int64
		maxMS     int64
		checkMS   int64
		approvals sql.NullInt64
		muted     string
		silent    string
	)
	err := s.db.QueryRowContext(ctx, "SELECT "+settingsColumns+" FROM settings WHERE id = 1").
		Scan(&pollMS, &watchMS, &maxMS, &checkMS, &approvals, &out.MergeMethod, &out.IncludeExisting, &out.IncludeOwn, &out.KeepWorktree,
			&out.NotificationsEnabled, &out.NotificationsBackgroundOnly, &muted, &silent, &out.ApprovalMode, &out.AutoApproveRebase,
			&out.Provider, &out.Model, &out.Effort, &out.BranchUpdate, &out.UpdateOnGitHub, &out.ScreenReader)
	if err != nil {
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}
	out.PollInterval = time.Duration(pollMS) * time.Millisecond
	out.WatchInterval = time.Duration(watchMS) * time.Millisecond
	out.WatchMaxInterval = time.Duration(maxMS) * time.Millisecond
	out.CheckMaxInterval = time.Duration(checkMS) * time.Millisecond
	out.MutedNotificationKinds = parseKinds(muted)
	out.SilentNotificationKinds = parseKinds(silent)
	if approvals.Valid {
		n := int(approvals.Int64)
		out.ApprovalsRequired = &n
	}
	return out, nil
}

func (s *Store) SaveSettings(ctx context.Context, next Settings) (Settings, error) {
	if err := next.Validate(); err != nil {
		return Settings{}, err
	}
	var approvals any
	if next.ApprovalsRequired != nil {
		approvals = *next.ApprovalsRequired
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Settings{}, fmt.Errorf("save settings: %w", err)
	}
	defer tx.Rollback()
	var storedMuted, storedSilent string
	err = tx.QueryRowContext(ctx, "SELECT muted_notification_kinds, silent_notification_kinds FROM settings WHERE id = 1").Scan(&storedMuted, &storedSilent)
	if err != nil {
		return Settings{}, fmt.Errorf("save settings: %w", err)
	}
	muted := kindsValue(next.MutedNotificationKinds, storedMuted)
	next.MutedNotificationKinds = parseKinds(muted)
	silent := kindsValue(next.SilentNotificationKinds, storedSilent)
	next.SilentNotificationKinds = parseKinds(silent)
	_, err = tx.ExecContext(ctx, `
UPDATE settings SET
    poll_interval_ms   = ?,
    watch_interval_ms  = ?,
    watch_max_interval_ms = ?,
    check_max_interval_ms = ?,
    approvals_required = ?,
    merge_method       = ?,
    include_existing   = ?,
    include_own        = ?,
    keep_worktree      = ?,
    notifications_enabled = ?,
    notifications_background_only = ?,
    muted_notification_kinds  = ?,
    silent_notification_kinds = ?,
    approval_mode       = ?,
    auto_approve_rebase = ?,
    provider            = ?,
    model               = ?,
    effort              = ?,
    branch_update       = ?,
    update_on_github    = ?,
    screen_reader       = ?
WHERE id = 1`,
		next.PollInterval.Milliseconds(), next.WatchInterval.Milliseconds(), next.WatchMaxInterval.Milliseconds(), next.CheckMaxInterval.Milliseconds(), approvals,
		next.MergeMethod, next.IncludeExisting, next.IncludeOwn, next.KeepWorktree,
		next.NotificationsEnabled, next.NotificationsBackgroundOnly, muted, silent, next.ApprovalMode, next.AutoApproveRebase,
		next.Provider, next.Model, next.Effort, next.BranchUpdate, next.UpdateOnGitHub, next.ScreenReader)
	if err != nil {
		return Settings{}, fmt.Errorf("save settings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Settings{}, fmt.Errorf("save settings: %w", err)
	}
	s.publish(events.SettingsChanged, "", 0)
	return next, nil
}
