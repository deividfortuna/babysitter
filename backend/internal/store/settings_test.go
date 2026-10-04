package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
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
	d := DefaultSettings()
	defaults := map[string]bool{
		"the notifications on": d.NotificationsEnabled,
		"the notifications only while the app is in the background": d.NotificationsBackgroundOnly,
		"every kind with a sound":                                   len(d.SilentNotificationKinds) == 0,
		"every kind shown":                                          len(d.MutedNotificationKinds) == 0,
		"the author releases each turn":                             d.ApprovalMode == ApprovalManual,
		"no rebase approved on its own":                             !d.AutoApproveRebase,
	}
	for want, holds := range defaults {
		if !holds {
			t.Errorf("DefaultSettings() = %+v, want %s", d, want)
		}
	}
}

func notificationKindFields() map[string]struct {
	kinds  func(*Settings) *[]NotificationKind
	seed   string
	stored string
} {
	return map[string]struct {
		kinds  func(*Settings) *[]NotificationKind
		seed   string
		stored string
	}{
		"muted": {
			kinds:  func(in *Settings) *[]NotificationKind { return &in.MutedNotificationKinds },
			seed:   `UPDATE settings SET muted_notification_kinds = 'review,rumour'`,
			stored: `SELECT muted_notification_kinds FROM settings WHERE id = 1`,
		},
		"silent": {
			kinds:  func(in *Settings) *[]NotificationKind { return &in.SilentNotificationKinds },
			seed:   `UPDATE settings SET silent_notification_kinds = 'review,rumour'`,
			stored: `SELECT silent_notification_kinds FROM settings WHERE id = 1`,
		},
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
		ScreenReader:      false,
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
	cases := map[string]struct {
		change func(*Settings)
		reason string
	}{
		"poll interval below the floor":  {func(in *Settings) { in.PollInterval = time.Second }, "the repository poll interval"},
		"poll interval above the roof":   {func(in *Settings) { in.PollInterval = 25 * time.Hour }, "the repository poll interval"},
		"watch interval below the floor": {func(in *Settings) { in.WatchInterval = time.Second }, "the watch poll interval must"},
		"watch interval above the roof":  {func(in *Settings) { in.WatchInterval = 25 * time.Hour }, "the watch poll interval must"},
		"longest below the watch":        {func(in *Settings) { in.WatchMaxInterval = time.Minute }, "the longest watch poll interval"},
		"longest above the roof":         {func(in *Settings) { in.WatchMaxInterval = 25 * time.Hour }, "the longest watch poll interval"},
		"check wait below the floor":     {func(in *Settings) { in.CheckMaxInterval = time.Second }, "the longest check read interval"},
		"check wait above the roof":      {func(in *Settings) { in.CheckMaxInterval = 25 * time.Hour }, "the longest check read interval"},
		"merge method unknown":           {func(in *Settings) { in.MergeMethod = "fast-forward" }, "unknown merge method"},
		"provider unknown":               {func(in *Settings) { in.Provider = "self" }, "unknown provider"},
		"approval mode unknown":          {func(in *Settings) { in.ApprovalMode = "sometimes" }, "unknown approval mode"},
		"branch update unknown":          {func(in *Settings) { in.BranchUpdate = "squash" }, "unknown branch update"},
		"approvals below zero":           {func(in *Settings) { in.ApprovalsRequired = &approvals }, "the approvals must be 0 or more"},
		"muted kind unknown":             {func(in *Settings) { in.MutedNotificationKinds = []NotificationKind{"rumour"} }, "unknown notification kind"},
		"silent kind unknown":            {func(in *Settings) { in.SilentNotificationKinds = []NotificationKind{"rumour"} }, "unknown notification kind"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := DefaultSettings()
			c.change(&in)
			_, err := s.SaveSettings(ctx, in)
			if !errors.Is(err, ErrInvalidSettings) {
				t.Fatalf("SaveSettings(%+v) error = %v, want one that wraps ErrInvalidSettings", in, err)
			}
			if !strings.Contains(err.Error(), c.reason) {
				t.Fatalf("SaveSettings(%+v) error = %v, want the reason %q", in, err, c.reason)
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

func TestSaveSettingsOrdersTheNotificationKindsAndDropsTheRepeats(t *testing.T) {
	t.Parallel()
	for name, field := range notificationKindFields() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, _ := openTemp(t)
			ctx := context.Background()
			next := DefaultSettings()
			*field.kinds(&next) = []NotificationKind{NotificationMerge, NotificationReview, NotificationMerge}

			saved, err := s.SaveSettings(ctx, next)
			if err != nil {
				t.Fatal(err)
			}
			want := []NotificationKind{NotificationReview, NotificationMerge}
			if got := *field.kinds(&saved); !reflect.DeepEqual(got, want) {
				t.Fatalf("SaveSettings returned %v, want %v", got, want)
			}
			read, err := s.Settings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got := *field.kinds(&read); !reflect.DeepEqual(got, want) {
				t.Fatalf("Settings() = %v, want %v", got, want)
			}
		})
	}
}

func TestSettingsLeaveOutAnUnknownKindOnReadAndKeepItOnSave(t *testing.T) {
	t.Parallel()
	for name, field := range notificationKindFields() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, _ := openTemp(t)
			ctx := context.Background()
			if _, err := s.db.ExecContext(ctx, field.seed); err != nil {
				t.Fatal(err)
			}
			got, err := s.Settings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if want := []NotificationKind{NotificationReview}; !reflect.DeepEqual(*field.kinds(&got), want) {
				t.Fatalf("Settings() holds %v, want %v", *field.kinds(&got), want)
			}

			*field.kinds(&got) = []NotificationKind{NotificationChecks}
			if _, err := s.SaveSettings(ctx, got); err != nil {
				t.Fatalf("SaveSettings() of what Settings() returned failed: %v", err)
			}

			var stored string
			if err := s.db.QueryRowContext(ctx, field.stored).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored != "checks,rumour" {
				t.Fatalf("the row holds %q, want the unknown kind kept beside the new one", stored)
			}
		})
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

func TestUpgradeTurnsTheScreenReaderModeOff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, schemaBeforeScreenReaderOff); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE settings SET screen_reader = 1 WHERE id = 1"); err != nil {
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
	if got.ScreenReader {
		t.Fatal("ScreenReader = true after the upgrade, want false")
	}
}
