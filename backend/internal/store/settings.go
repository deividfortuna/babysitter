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
	PollInterval           time.Duration
	WatchInterval          time.Duration
	ApprovalsRequired      *int
	MergeMethod            string
	IncludeExisting        bool
	IncludeOwn             bool
	KeepWorktree           bool
	NotificationsEnabled   bool
	NotificationSound      bool
	MutedNotificationKinds []NotificationKind
	ApprovalMode           ApprovalMode
	AutoApproveRebase      bool
	Provider               string
	Model                  string
	Effort                 string
}

type ApprovalMode string

const (
	ApprovalAuto   ApprovalMode = "auto"
	ApprovalManual ApprovalMode = "manual"
)

var ApprovalModes = []ApprovalMode{ApprovalAuto, ApprovalManual}

func (m ApprovalMode) Valid() bool { return slices.Contains(ApprovalModes, m) }

const (
	MinInterval = 10 * time.Second
	MaxInterval = 24 * time.Hour
)

var ErrInvalidSettings = errors.New("invalid settings")

var SettingsProviders = []string{"claude", "copilot"}

func DefaultSettings() Settings {
	return Settings{
		PollInterval:         time.Minute,
		WatchInterval:        3 * time.Minute,
		NotificationsEnabled: true,
		NotificationSound:    true,
		ApprovalMode:         ApprovalManual,
		Provider:             "claude",
	}
}

func (s Settings) Validate() error {
	if s.PollInterval < MinInterval || s.PollInterval > MaxInterval {
		return fmt.Errorf("%w: the repository poll interval must be between %s and %s, got %s", ErrInvalidSettings, MinInterval, MaxInterval, s.PollInterval)
	}
	if s.WatchInterval < MinInterval || s.WatchInterval > MaxInterval {
		return fmt.Errorf("%w: the watch poll interval must be between %s and %s, got %s", ErrInvalidSettings, MinInterval, MaxInterval, s.WatchInterval)
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
	if s.ApprovalsRequired != nil && *s.ApprovalsRequired < 0 {
		return fmt.Errorf("%w: the approvals must be 0 or more, got %d", ErrInvalidSettings, *s.ApprovalsRequired)
	}
	for _, kind := range s.MutedNotificationKinds {
		if !kind.Valid() {
			return fmt.Errorf("%w: unknown notification kind %q: use %s", ErrInvalidSettings, kind, JoinKinds())
		}
	}
	return nil
}

func (s Settings) ShowsNotification(k NotificationKind) bool {
	return s.NotificationsEnabled && !slices.Contains(s.MutedNotificationKinds, k)
}

func mutedKindsValue(kinds []NotificationKind, stored string) string {
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

func parseMutedKinds(value string) []NotificationKind {
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

const settingsColumns = "poll_interval_ms, watch_interval_ms, approvals_required, merge_method, include_existing, include_own, keep_worktree, notifications_enabled, notification_sound, muted_notification_kinds, approval_mode, auto_approve_rebase, provider, model, effort"

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	var (
		out       Settings
		pollMS    int64
		watchMS   int64
		approvals sql.NullInt64
		muted     string
	)
	err := s.db.QueryRowContext(ctx, "SELECT "+settingsColumns+" FROM settings WHERE id = 1").
		Scan(&pollMS, &watchMS, &approvals, &out.MergeMethod, &out.IncludeExisting, &out.IncludeOwn, &out.KeepWorktree,
			&out.NotificationsEnabled, &out.NotificationSound, &muted, &out.ApprovalMode, &out.AutoApproveRebase,
			&out.Provider, &out.Model, &out.Effort)
	if err != nil {
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}
	out.PollInterval = time.Duration(pollMS) * time.Millisecond
	out.WatchInterval = time.Duration(watchMS) * time.Millisecond
	out.MutedNotificationKinds = parseMutedKinds(muted)
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
	var stored string
	err = tx.QueryRowContext(ctx, "SELECT muted_notification_kinds FROM settings WHERE id = 1").Scan(&stored)
	if err != nil {
		return Settings{}, fmt.Errorf("save settings: %w", err)
	}
	muted := mutedKindsValue(next.MutedNotificationKinds, stored)
	next.MutedNotificationKinds = parseMutedKinds(muted)
	_, err = tx.ExecContext(ctx, `
UPDATE settings SET
    poll_interval_ms   = ?,
    watch_interval_ms  = ?,
    approvals_required = ?,
    merge_method       = ?,
    include_existing   = ?,
    include_own        = ?,
    keep_worktree      = ?,
    notifications_enabled = ?,
    notification_sound    = ?,
    muted_notification_kinds = ?,
    approval_mode       = ?,
    auto_approve_rebase = ?,
    provider            = ?,
    model               = ?,
    effort              = ?
WHERE id = 1`,
		next.PollInterval.Milliseconds(), next.WatchInterval.Milliseconds(), approvals,
		next.MergeMethod, next.IncludeExisting, next.IncludeOwn, next.KeepWorktree,
		next.NotificationsEnabled, next.NotificationSound, muted, next.ApprovalMode, next.AutoApproveRebase,
		next.Provider, next.Model, next.Effort)
	if err != nil {
		return Settings{}, fmt.Errorf("save settings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Settings{}, fmt.Errorf("save settings: %w", err)
	}
	s.publish(events.SettingsChanged, "", 0)
	return next, nil
}
