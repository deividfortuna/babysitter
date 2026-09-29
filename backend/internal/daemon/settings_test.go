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
	stored := store.Settings{PollInterval: 2 * time.Minute, WatchInterval: 5 * time.Minute, WatchMaxInterval: 15 * time.Minute, CheckMaxInterval: 15 * time.Minute, KeepWorktree: true, ApprovalMode: store.ApprovalAuto, Provider: "claude", BranchUpdate: store.BranchRebase}

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
	stored := store.Settings{PollInterval: 2 * time.Minute, WatchInterval: 5 * time.Minute, WatchMaxInterval: 15 * time.Minute, CheckMaxInterval: 15 * time.Minute, KeepWorktree: true, ApprovalMode: store.ApprovalAuto, Provider: "claude", BranchUpdate: store.BranchRebase}

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

func TestRunningSettingsTakeTheLongestIntervalOfTheFlag(t *testing.T) {
	t.Parallel()
	stored := store.DefaultSettings()

	got, err := runningSettings(stored, Config{WatchMaxInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if got.WatchInterval != stored.WatchInterval || got.WatchMaxInterval != time.Hour {
		t.Fatalf("intervals = %s/%s, want the stored watch interval and the longest of the flag", got.WatchInterval, got.WatchMaxInterval)
	}
}

func TestRunningSettingsTakeTheLongestCheckIntervalOfTheFlag(t *testing.T) {
	t.Parallel()
	stored := store.DefaultSettings()

	got, err := runningSettings(stored, Config{CheckMaxInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if got.CheckMaxInterval != time.Hour || !intervalsOverridden(got, stored) {
		t.Fatalf("CheckMaxInterval = %s, want the one of the flag, 1h0m0s, for this run only", got.CheckMaxInterval)
	}
}

func TestRunningSettingsRaiseTheLongestIntervalToAWatchIntervalAboveIt(t *testing.T) {
	t.Parallel()
	stored := store.DefaultSettings()

	got, err := runningSettings(stored, Config{WatchInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if got.WatchMaxInterval != time.Hour {
		t.Fatalf("WatchMaxInterval = %s, want the watch interval of the flag, 1h0m0s", got.WatchMaxInterval)
	}
}

func TestRunningSettingsSayNoToAnIntervalTheDaemonCannotRun(t *testing.T) {
	t.Parallel()
	stored := store.DefaultSettings()
	cases := map[string]Config{
		"below the floor":     {Interval: time.Millisecond},
		"a negative interval": {Interval: -30 * time.Second},
		"a negative watch":    {WatchInterval: -time.Second},
		"a negative longest":  {WatchMaxInterval: -time.Second},
		"a negative check":    {CheckMaxInterval: -time.Second},
		"a longest too short": {WatchInterval: 10 * time.Minute, WatchMaxInterval: 5 * time.Minute},
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
