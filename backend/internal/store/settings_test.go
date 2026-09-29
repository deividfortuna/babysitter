package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
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
		WatchMaxInterval:  10 * time.Minute,
		CheckMaxInterval:  20 * time.Minute,
		ApprovalsRequired: &approvals,
		MergeMethod:       "rebase",
		IncludeExisting:   true,
		IncludeOwn:        true,
		KeepWorktree:      true,
		ApprovalMode:      ApprovalAuto,
		AutoApproveRebase: true,
		Provider:          "copilot",
		Model:             "auto",
		BranchUpdate:      BranchMerge,
		UpdateOnGitHub:    false,
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
	if _, err := s.SaveSettings(ctx, Settings{PollInterval: time.Minute, WatchInterval: time.Minute, WatchMaxInterval: time.Minute, CheckMaxInterval: time.Minute, ApprovalsRequired: &approvals, ApprovalMode: ApprovalManual, Provider: "claude", BranchUpdate: BranchRebase}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.SaveSettings(ctx, Settings{PollInterval: time.Minute, WatchInterval: time.Minute, WatchMaxInterval: time.Minute, CheckMaxInterval: time.Minute, ApprovalMode: ApprovalManual, Provider: "claude", BranchUpdate: BranchRebase}); err != nil {
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
		"merge method unknown":           {PollInterval: time.Minute, WatchInterval: time.Minute, WatchMaxInterval: time.Hour, MergeMethod: "fast-forward"},
		"approvals below zero":           {PollInterval: time.Minute, WatchInterval: time.Minute, WatchMaxInterval: time.Hour, ApprovalsRequired: &approvals},
		"longest below the watch":        {PollInterval: time.Minute, WatchInterval: 5 * time.Minute, WatchMaxInterval: time.Minute},
		"longest above the roof":         {PollInterval: time.Minute, WatchInterval: time.Minute, WatchMaxInterval: 25 * time.Hour},
		"check wait below the floor":     {PollInterval: time.Minute, WatchInterval: time.Minute, WatchMaxInterval: time.Hour, CheckMaxInterval: time.Second},
		"check wait above the roof":      {PollInterval: time.Minute, WatchInterval: time.Minute, WatchMaxInterval: time.Hour, CheckMaxInterval: 25 * time.Hour},
		"branch update unknown":          {PollInterval: time.Minute, WatchInterval: time.Minute, WatchMaxInterval: time.Hour, ApprovalMode: ApprovalAuto, Provider: "claude", BranchUpdate: "squash"},
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
	if !got.NotificationsEnabled {
		t.Fatalf("DefaultSettings() = %+v, want the notifications on", got)
	}
	if len(got.SilentNotificationKinds) != 0 {
		t.Fatalf("DefaultSettings() silences %v, want every kind with a sound", got.SilentNotificationKinds)
	}
	if !got.NotificationsBackgroundOnly {
		t.Fatalf("DefaultSettings() = %+v, want the notifications only while the app is in the background", got)
	}
}

func TestSaveSettingsTurnsTheNotificationsOff(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	want := DefaultSettings()
	want.NotificationsEnabled = false
	want.NotificationsBackgroundOnly = false

	if _, err := s.SaveSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.NotificationsEnabled {
		t.Fatalf("Settings() = %+v, want the notifications off", got)
	}
	if got.NotificationsBackgroundOnly {
		t.Fatalf("Settings() = %+v, want the notifications shown while the app has the focus too", got)
	}
}

func TestSaveSettingsKeepsTheSilentNotificationKinds(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	next := DefaultSettings()
	next.SilentNotificationKinds = []NotificationKind{NotificationMerge, NotificationReview, NotificationMerge}

	saved, err := s.SaveSettings(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	want := []NotificationKind{NotificationReview, NotificationMerge}
	if !reflect.DeepEqual(saved.SilentNotificationKinds, want) {
		t.Fatalf("SaveSettings returned %v, want %v", saved.SilentNotificationKinds, want)
	}
	got, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.SilentNotificationKinds, want) {
		t.Fatalf("Settings() = %v, want %v", got.SilentNotificationKinds, want)
	}
}

func TestSaveSettingsRejectsAnUnknownSilentNotificationKind(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	next := DefaultSettings()
	next.SilentNotificationKinds = []NotificationKind{"rumour"}

	if _, err := s.SaveSettings(context.Background(), next); !errors.Is(err, ErrInvalidSettings) {
		t.Fatalf("SaveSettings error = %v, want one that wraps ErrInvalidSettings", err)
	}
}

func TestSaveSettingsKeepsASilentKindThisBuildDoesNotKnow(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `UPDATE settings SET silent_notification_kinds = 'merge,rumour'`); err != nil {
		t.Fatal(err)
	}
	got, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []NotificationKind{NotificationMerge}; !reflect.DeepEqual(got.SilentNotificationKinds, want) {
		t.Fatalf("Settings() silences %v, want %v", got.SilentNotificationKinds, want)
	}

	got.SilentNotificationKinds = []NotificationKind{NotificationChecks}
	if _, err := s.SaveSettings(ctx, got); err != nil {
		t.Fatal(err)
	}

	var silent string
	if err := s.db.QueryRowContext(ctx, `SELECT silent_notification_kinds FROM settings WHERE id = 1`).Scan(&silent); err != nil {
		t.Fatal(err)
	}
	if silent != "checks,rumour" {
		t.Fatalf("the row holds %q, want the unknown kind kept beside the new one", silent)
	}
}

func TestPlaysSoundFollowsTheSilentKinds(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	settings.SilentNotificationKinds = []NotificationKind{NotificationChecks}

	if settings.PlaysSound(NotificationChecks) {
		t.Fatal("PlaysSound(checks) = true, want the silent kind without a sound")
	}
	if !settings.PlaysSound(NotificationReview) {
		t.Fatal("PlaysSound(review) = false, want a kind nobody silenced with a sound")
	}
}

func TestUpgradeKeepsTheNotificationsSilentWhenTheSoundWasOff(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		sound int
		want  []NotificationKind
	}{
		"the sound was on":  {sound: 1, want: nil},
		"the sound was off": {sound: 0, want: NotificationKinds},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "babysitter.db")
			db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
			if err != nil {
				t.Fatal(err)
			}
			if err := migrateTo(ctx, db, 33); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "UPDATE settings SET notification_sound = ? WHERE id = 1", c.sound); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			s, err := Open(path)
			if err != nil {
				t.Fatalf("Open() after the upgrade error = %v", err)
			}
			defer s.Close()
			got, err := s.Settings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.SilentNotificationKinds, c.want) {
				t.Fatalf("SilentNotificationKinds = %v, want %v", got.SilentNotificationKinds, c.want)
			}
			if !got.NotificationsBackgroundOnly {
				t.Fatalf("Settings() = %+v, want the notifications only in the background, as the app showed them before", got)
			}
			if _, err := s.db.ExecContext(ctx, "SELECT notification_sound FROM settings"); err == nil {
				t.Fatal("the settings still hold the notification_sound column, want it dropped")
			}
		})
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

	got.NotificationsBackgroundOnly = true
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

func TestUpgradeGivesTheWatchALongestIntervalNoShorterThanItsInterval(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		watchMS int64
		want    time.Duration
	}{
		"the default interval": {watchMS: 180000, want: 15 * time.Minute},
		"a long interval":      {watchMS: 1800000, want: 30 * time.Minute},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "babysitter.db")
			db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
			if err != nil {
				t.Fatal(err)
			}
			if err := migrateTo(ctx, db, schemaBeforeWatchMaxInterval); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "UPDATE settings SET watch_interval_ms = ? WHERE id = 1", c.watchMS); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			s, err := Open(path)
			if err != nil {
				t.Fatalf("Open() after the upgrade error = %v", err)
			}
			defer s.Close()
			got, err := s.Settings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got.WatchMaxInterval != c.want {
				t.Fatalf("WatchMaxInterval = %s, want %s", got.WatchMaxInterval, c.want)
			}
		})
	}
}
