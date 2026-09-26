package daemon

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/store"
)

func TestRunningSettingsKeepTheStoredOnesWithoutAFlag(t *testing.T) {
	t.Parallel()
	stored := store.Settings{PollInterval: 2 * time.Minute, WatchInterval: 5 * time.Minute, KeepWorktree: true, ApprovalMode: store.ApprovalAuto}

	got, err := runningSettings(stored, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, stored) {
		t.Fatalf("settings = %+v, want the stored %+v", got, stored)
	}
}

func TestRunningSettingsTakeTheIntervalsOfTheFlags(t *testing.T) {
	t.Parallel()
	stored := store.Settings{PollInterval: 2 * time.Minute, WatchInterval: 5 * time.Minute, KeepWorktree: true, ApprovalMode: store.ApprovalAuto}

	got, err := runningSettings(stored, Config{Interval: 30 * time.Second, WatchInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if got.PollInterval != 30*time.Second || got.WatchInterval != time.Minute {
		t.Fatalf("intervals = %s/%s, want the ones the flags asked for", got.PollInterval, got.WatchInterval)
	}
	if !got.KeepWorktree {
		t.Fatal("a flag about the intervals changed the rest of the settings")
	}
}

func TestRunningSettingsSayNoToAnIntervalTheDaemonCannotRun(t *testing.T) {
	t.Parallel()
	stored := store.DefaultSettings()
	cases := map[string]Config{
		"below the floor":     {Interval: time.Millisecond},
		"a negative interval": {Interval: -30 * time.Second},
		"a negative watch":    {WatchInterval: -time.Second},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := runningSettings(stored, cfg)
			if err == nil {
				t.Fatal("the daemon started quietly, want an error that names the interval")
			}
			if !strings.Contains(err.Error(), "interval") {
				t.Fatalf("error = %v, want one that names the interval", err)
			}
		})
	}
}
