package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
)

func TestSettingsOfAFreshDatabaseAreTheDefaults(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)

	got, err := s.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, DefaultSettings()) {
		t.Fatalf("Settings() = %+v, want the defaults %+v", got, DefaultSettings())
	}
}

func TestSaveSettingsKeepsWhatItWasGiven(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	approvals := 2
	want := Settings{
		PollInterval:      90 * time.Second,
		WatchInterval:     30 * time.Second,
		ApprovalsRequired: &approvals,
		MergeMethod:       "rebase",
		IncludeExisting:   true,
		IncludeOwn:        true,
		KeepWorktree:      true,
		ApprovalMode:      ApprovalAuto,
		AutoApproveRebase: true,
	}

	saved, err := s.SaveSettings(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved, want) {
		t.Fatalf("SaveSettings returned %+v, want %+v", saved, want)
	}
	got, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Settings() = %+v, want %+v", got, want)
	}
}

func TestSaveSettingsForgetsTheApprovalsAndTakesTheRuleOfTheBranch(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	approvals := 3
	if _, err := s.SaveSettings(ctx, Settings{PollInterval: time.Minute, WatchInterval: time.Minute, ApprovalsRequired: &approvals, ApprovalMode: ApprovalManual}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveSettings(ctx, Settings{PollInterval: time.Minute, WatchInterval: time.Minute, ApprovalMode: ApprovalManual}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.ApprovalsRequired != nil {
		t.Fatalf("ApprovalsRequired = %d, want none so the rule of the base branch decides", *got.ApprovalsRequired)
	}
}

func TestSaveSettingsRejectsValuesTheDaemonCannotRun(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	approvals := -1
	cases := map[string]Settings{
		"poll interval below the floor":  {PollInterval: time.Second, WatchInterval: time.Minute},
		"watch interval below the floor": {PollInterval: time.Minute, WatchInterval: time.Second},
		"poll interval above the roof":   {PollInterval: 25 * time.Hour, WatchInterval: time.Minute},
		"watch interval above the roof":  {PollInterval: time.Minute, WatchInterval: 25 * time.Hour},
		"merge method unknown":           {PollInterval: time.Minute, WatchInterval: time.Minute, MergeMethod: "fast-forward"},
		"approvals below zero":           {PollInterval: time.Minute, WatchInterval: time.Minute, ApprovalsRequired: &approvals},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.SaveSettings(ctx, in); err == nil {
				t.Fatalf("SaveSettings(%+v) accepted the value, want an error", in)
			}
		})
	}
}

func TestSaveSettingsPublishesTheChange(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	pub := &recordingPublisher{}
	s.SetPublisher(pub)

	if _, err := s.SaveSettings(context.Background(), DefaultSettings()); err != nil {
		t.Fatal(err)
	}

	if got := pub.last(); got.Type != events.SettingsChanged {
		t.Fatalf("published %+v, want %s", got, events.SettingsChanged)
	}
}

func TestDefaultSettingsShowNotificationsWithASound(t *testing.T) {
	t.Parallel()
	got := DefaultSettings()
	if !got.NotificationsEnabled || !got.NotificationSound {
		t.Fatalf("DefaultSettings() = %+v, want the notifications on and the sound on", got)
	}
}

func TestSaveSettingsTurnsTheNotificationsOff(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	want := DefaultSettings()
	want.NotificationsEnabled = false
	want.NotificationSound = false

	if _, err := s.SaveSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.NotificationsEnabled || got.NotificationSound {
		t.Fatalf("Settings() = %+v, want both notification settings off", got)
	}
}

func TestSaveSettingsKeepsTheMutedNotificationKinds(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	want := DefaultSettings()
	want.MutedNotificationKinds = []NotificationKind{NotificationReview, NotificationChecks}

	saved, err := s.SaveSettings(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.MutedNotificationKinds, want.MutedNotificationKinds) {
		t.Fatalf("SaveSettings returned %v, want %v", saved.MutedNotificationKinds, want.MutedNotificationKinds)
	}
	got, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.MutedNotificationKinds, want.MutedNotificationKinds) {
		t.Fatalf("Settings() = %v, want %v", got.MutedNotificationKinds, want.MutedNotificationKinds)
	}
}

func TestSaveSettingsOrdersTheMutedKindsAndDropsTheRepeats(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	next := DefaultSettings()
	next.MutedNotificationKinds = []NotificationKind{NotificationMerge, NotificationAgent, NotificationMerge}

	saved, err := s.SaveSettings(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	want := []NotificationKind{NotificationAgent, NotificationMerge}
	if !reflect.DeepEqual(saved.MutedNotificationKinds, want) {
		t.Fatalf("SaveSettings returned %v, want %v", saved.MutedNotificationKinds, want)
	}
}

func TestSaveSettingsRejectsAnUnknownNotificationKind(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	next := DefaultSettings()
	next.MutedNotificationKinds = []NotificationKind{"rumour"}

	if _, err := s.SaveSettings(context.Background(), next); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("SaveSettings error = %v, want one that wraps ErrInvalidSettings", err)
	}
}

func TestDefaultSettingsMuteNoKind(t *testing.T) {
	t.Parallel()
	if got := DefaultSettings().MutedNotificationKinds; len(got) != 0 {
		t.Fatalf("DefaultSettings() mutes %v, want every kind shown", got)
	}
}

func TestShowsNotificationFollowsTheSwitchAndTheMutedKinds(t *testing.T) {
	t.Parallel()
	muted := DefaultSettings()
	muted.MutedNotificationKinds = []NotificationKind{NotificationReview}
	off := DefaultSettings()
	off.NotificationsEnabled = false

	cases := []struct {
		name     string
		settings Settings
		kind     NotificationKind
		want     bool
	}{
		{"a kind nobody muted", muted, NotificationChecks, true},
		{"the muted kind", muted, NotificationReview, false},
		{"every kind while the notifications are off", off, NotificationChecks, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.settings.ShowsNotification(c.kind); got != c.want {
				t.Fatalf("ShowsNotification(%q) = %t, want %t", c.kind, got, c.want)
			}
		})
	}
}

func TestSettingsLeaveOutAMutedKindThisBuildDoesNotKnow(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `UPDATE settings SET muted_notification_kinds = 'review,rumour'`); err != nil {
		t.Fatal(err)
	}

	got, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []NotificationKind{NotificationReview}; !reflect.DeepEqual(got.MutedNotificationKinds, want) {
		t.Fatalf("Settings() mutes %v, want %v", got.MutedNotificationKinds, want)
	}
	if _, err := s.SaveSettings(ctx, got); err != nil {
		t.Fatalf("SaveSettings() of what Settings() returned failed: %v", err)
	}
}

func TestSaveSettingsKeepsAMutedKindThisBuildDoesNotKnow(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `UPDATE settings SET muted_notification_kinds = 'review,rumour'`); err != nil {
		t.Fatal(err)
	}
	got, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got.NotificationSound = false
	got.MutedNotificationKinds = []NotificationKind{NotificationChecks}
	if _, err := s.SaveSettings(ctx, got); err != nil {
		t.Fatal(err)
	}

	var muted string
	if err := s.db.QueryRowContext(ctx, `SELECT muted_notification_kinds FROM settings WHERE id = 1`).Scan(&muted); err != nil {
		t.Fatal(err)
	}
	if muted != "checks,rumour" {
		t.Fatalf("the row holds %q, want the unknown kind kept beside the new one", muted)
	}
}

func TestSaveSettingsCleansTheUnknownKindsItKeeps(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `UPDATE settings SET muted_notification_kinds = 'review, rumour,rumour,,'`); err != nil {
		t.Fatal(err)
	}
	got, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got.MutedNotificationKinds = []NotificationKind{NotificationChecks}
	if _, err := s.SaveSettings(ctx, got); err != nil {
		t.Fatal(err)
	}

	var muted string
	if err := s.db.QueryRowContext(ctx, `SELECT muted_notification_kinds FROM settings WHERE id = 1`).Scan(&muted); err != nil {
		t.Fatal(err)
	}
	if muted != "checks,rumour" {
		t.Fatalf("the row holds %q, want each unknown kind once and trimmed", muted)
	}
}
