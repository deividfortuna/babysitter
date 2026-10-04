package cli

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func runSettings(t *testing.T, d *fakeDaemon, args ...string) (string, error) {
	t.Helper()
	return runAgainstDaemon(t, d, "settings", args...)
}

func TestSettingsGetPrintsWhatTheDaemonHolds(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runSettings(t, d, "get")
	if err != nil {
		t.Fatalf("settings get error = %v", err)
	}
	for _, want := range []string{"1m0s", "3m0s", "rule of the base branch", "first method the repository allows"} {
		if !strings.Contains(out, want) {
			t.Fatalf("settings get = %q, want it to carry %q", out, want)
		}
	}
}

func TestSettingsSetWritesOnlyTheFlagsThatWereTyped(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runSettings(t, d, "set", "--watch-interval", "45s", "--watch-max-interval", "10m", "--check-max-interval", "20m", "--keep-worktree")
	if err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if len(d.settingsPut) != 1 {
		t.Fatalf("the daemon got %d writes, want 1", len(d.settingsPut))
	}
	got := d.settingsPut[0]
	if got["watchIntervalSeconds"] != float64(45) || got["watchMaxIntervalSeconds"] != float64(600) ||
		got["checkMaxIntervalSeconds"] != float64(1200) || got["keepWorktree"] != true {
		t.Fatalf("body = %v, want the intervals and the worktree of the flags", got)
	}
	if got["pollIntervalSeconds"] != float64(60) {
		t.Fatalf("body = %v, want the poll interval the daemon already had", got)
	}
	if !strings.Contains(out, "45s") {
		t.Fatalf("settings set = %q, want the settings it wrote", out)
	}
}

func TestSettingsSetTakesTheApprovalsAndGivesThemBackToTheBranch(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runSettings(t, d, "set", "--approvals", "2"); err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if got := d.settingsPut[0]["approvalsRequired"]; got != float64(2) {
		t.Fatalf("approvals = %v, want 2", got)
	}

	if _, err := runSettings(t, d, "set", "--approvals", "branch"); err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if got := d.settingsPut[1]["approvalsRequired"]; got != nil {
		t.Fatalf("approvals = %v, want none so the rule of the base branch decides", got)
	}
}

func TestWatchStopHelpNamesTheWorktreeRuleOfTheWatch(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runWatch(t, d, "stop", "--help")
	if err != nil {
		t.Fatalf("watch stop --help error = %v", err)
	}
	if !strings.Contains(out, "the rule the watch started with") || strings.Contains(out, "daemon") {
		t.Fatalf("help = %q, want the rule the watch started with, not the daemon", out)
	}
}

func TestSettingsSetChangesTheAgentOfANewWatch(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runSettings(t, d, "set", "--provider", "copilot"); err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if got := d.settingsPut[0]; got["provider"] != "copilot" || got["model"] != "" {
		t.Fatalf("body = %v, want copilot with its default model", got)
	}

	if _, err := runSettings(t, d, "set", "--provider", "copilot", "--model", "auto"); err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if got := d.settingsPut[1]; got["provider"] != "copilot" || got["model"] != "auto" {
		t.Fatalf("body = %v, want copilot with the model of the flag", got)
	}

	if _, err := runSettings(t, d, "set", "--model", "gpt-5.3-codex", "--effort", "xhigh"); err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if got := d.settingsPut[2]; got["model"] != "gpt-5.3-codex" || got["effort"] != "xhigh" {
		t.Fatalf("body = %v, want the model and the effort of the flags", got)
	}
}

func TestSettingsSetGivesANewModelItsDefaultEffort(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runSettings(t, d, "set", "--effort", "high"); err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if got := d.settingsPut[0]; got["effort"] != "high" {
		t.Fatalf("body = %v, want the effort of the flag", got)
	}

	if _, err := runSettings(t, d, "set", "--model", "haiku"); err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if got := d.settingsPut[1]; got["model"] != "haiku" || got["effort"] != "" {
		t.Fatalf("body = %v, want haiku at its default effort", got)
	}
}

func TestWatchStartRejectsApprovalsThatAreNotANumber(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runWatch(t, d, "start", "octo/hello#3", "--approvals", "some"); err == nil {
		t.Fatal("watch start took approvals that are not a number, want an error")
	}
	if len(d.starts) != 0 {
		t.Fatalf("the daemon got %v, want nothing for a request the command refuses", d.starts)
	}
}

func TestSettingsSetRejections(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"nothing to change":      {"set"},
		"only a global flag":     {"set", "-o", "json"},
		"approvals nonsense":     {"set", "--approvals", "some"},
		"sound and silent kinds": {"set", "--notification-sound=false", "--silent-notifications", "review"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newFakeDaemon()
			if _, err := runSettings(t, d, args...); err == nil {
				t.Fatalf("settings %v was accepted, want an error", args)
			}
			if len(d.settingsPut) != 0 {
				t.Fatalf("the daemon got %v, want nothing for a request the command refuses", d.settingsPut)
			}
			if d.settingsGets != 0 {
				t.Fatalf("the daemon was read %d times for a request the command refuses, want none", d.settingsGets)
			}
		})
	}
}

func TestSettingsSetNotificationSoundSetsEveryKind(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		flag string
		want []any
	}{
		"sound off": {flag: "--notification-sound=false", want: []any{"agent", "review", "checks", "watch", "merge", "auto"}},
		"sound on":  {flag: "--notification-sound", want: []any{}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newFakeDaemon()
			d.settings["silentNotificationKinds"] = []string{"review"}

			if _, err := runSettings(t, d, "set", c.flag); err != nil {
				t.Fatalf("settings set error = %v", err)
			}
			if len(d.settingsPut) != 1 {
				t.Fatalf("the daemon got %d writes, want 1", len(d.settingsPut))
			}
			if got := d.settingsPut[0]["silentNotificationKinds"]; !reflect.DeepEqual(got, c.want) {
				t.Fatalf("silent kinds = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSettingsSetChangesOneSwitch(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		key  string
		held any
		want any
		row  string
	}{
		{"screen reader mode off", []string{"--screen-reader=false"}, "screenReader", true, false, `Screen reader mode of the agent +no\n`},
		{"notifications off", []string{"--notifications=false"}, "notificationsEnabled", true, false, `Show notifications +no\n`},
		{"notifications only in the background", []string{"--notifications-background-only"}, "notificationsBackgroundOnly", false, true, `Only in the background +yes\n`},
		{"approval mode auto", []string{"--approval-mode", "auto"}, "approvalMode", "manual", "auto", `Approval mode +auto\n`},
		{"clean rebase approved on its own", []string{"--auto-approve-rebase"}, "autoApproveRebase", false, true, `Approve a clean rebase or merge on its own +yes\n`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newFakeDaemon()
			d.settings[tc.key] = tc.held

			out, err := runSettings(t, d, append([]string{"set"}, tc.args...)...)
			if err != nil {
				t.Fatalf("settings set error = %v", err)
			}
			if len(d.settingsPut) != 1 {
				t.Fatalf("the daemon got %d writes, want 1", len(d.settingsPut))
			}
			if got := d.settingsPut[0][tc.key]; got != tc.want {
				t.Fatalf("%s = %v, want %v", tc.key, got, tc.want)
			}
			if !regexp.MustCompile(tc.row).MatchString(out) {
				t.Fatalf("settings set = %q, want the row %q", out, tc.row)
			}
		})
	}
}

func TestSettingsSetWritesTheNotificationKinds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		flag  string
		value string
		key   string
		held  []string
		want  []any
		row   string
	}{
		{"silent kinds", "--silent-notifications", " merge, agent ", "silentNotificationKinds", nil, []any{"merge", "agent"}, `Silent notification kinds +merge, agent\n`},
		{"muted kinds", "--mute-notifications", "review,checks", "mutedNotificationKinds", nil, []any{"review", "checks"}, `Muted notification kinds +review, checks\n`},
		{"every kind shown again", "--mute-notifications", "", "mutedNotificationKinds", []string{"review"}, []any{}, `Muted notification kinds +none\n`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newFakeDaemon()
			d.settings[tc.key] = tc.held

			out, err := runSettings(t, d, "set", tc.flag, tc.value)
			if err != nil {
				t.Fatalf("settings set error = %v", err)
			}
			if len(d.settingsPut) != 1 {
				t.Fatalf("the daemon got %d writes, want 1", len(d.settingsPut))
			}
			if got := d.settingsPut[0][tc.key]; !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s = %v, want %v", tc.key, got, tc.want)
			}
			if !regexp.MustCompile(tc.row).MatchString(out) {
				t.Fatalf("settings set = %q, want the row %q", out, tc.row)
			}
		})
	}
}

func TestSettingsGetPrintsTheMutedNotificationKinds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		muted []string
		row   string
	}{
		{"two kinds muted", []string{"review", "merge"}, `Muted notification kinds +review, merge\n`},
		{"no kind muted", nil, `Muted notification kinds +none\n`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newFakeDaemon()
			d.settings["mutedNotificationKinds"] = tc.muted

			out, err := runSettings(t, d, "get")
			if err != nil {
				t.Fatalf("settings get error = %v", err)
			}
			if !regexp.MustCompile(tc.row).MatchString(out) {
				t.Fatalf("settings get = %q, want the row %q", out, tc.row)
			}
		})
	}
}

func TestSettingsSetPassesOnWhatTheDaemonRefuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		args    []string
		key     string
		sent    any
		refusal string
	}{
		{"a poll interval out of bounds", []string{"--poll-interval", "2s"}, "pollIntervalSeconds", float64(2), "the repository poll interval must be between 10s and 24h0m0s, got 2s"},
		{"an unknown merge method", []string{"--merge-method", "fast-forward"}, "mergeMethod", "fast-forward", `unknown merge method "fast-forward": use squash, merge, rebase`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newFakeDaemon()
			d.settingsRefusal = tc.refusal

			_, err := runSettings(t, d, append([]string{"set"}, tc.args...)...)
			if passedOn := err != nil && strings.Contains(err.Error(), tc.refusal); !passedOn {
				t.Fatalf("error = %v, want the one the daemon gave: %q", err, tc.refusal)
			}
			if got := d.settingsPut[0][tc.key]; got != tc.sent {
				t.Fatalf("%s = %v, want the %v the user typed", tc.key, got, tc.sent)
			}
		})
	}
}

func TestSettingsSetLeavesTheBoundOfTheApprovalsToTheDaemon(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.settingsRefusal = "the approvals must be 0 or more, got -1"

	_, err := runSettings(t, d, "set", "--approvals", "-1")
	if passedOn := err != nil && strings.Contains(err.Error(), d.settingsRefusal); !passedOn {
		t.Fatalf("settings set -1 approvals = %v, want the answer of the daemon", err)
	}
	if len(d.settingsPut) != 1 {
		t.Fatalf("the daemon got %d writes, want 1", len(d.settingsPut))
	}
	if got := d.settingsPut[0]["approvalsRequired"]; got != float64(-1) {
		t.Fatalf("approvals = %v, want the -1 the user typed", got)
	}
}
