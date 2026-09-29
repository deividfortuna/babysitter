package cli

import (
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

func TestWatchStartAsksForTheRuleOfTheBaseBranch(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runWatch(t, d, "start", "octo/hello#3", "--approvals", "branch"); err != nil {
		t.Fatalf("watch start error = %v", err)
	}

	body := d.starts[0]
	if _, named := body["approvalsRequired"]; !named || body["approvalsRequired"] != nil {
		t.Fatalf("approvals = %v, want a null that asks for the rule of the base branch", body["approvalsRequired"])
	}
}

func TestWatchStartLeavesTheApprovalsOutWhenNobodyTypedThem(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runWatch(t, d, "start", "octo/hello#3"); err != nil {
		t.Fatalf("watch start error = %v", err)
	}
	if _, named := d.starts[0]["approvalsRequired"]; named {
		t.Fatalf("body = %v, want no approvals field at all", d.starts[0])
	}
}

func TestWatchStartLeavesTheAgentAndTheWorktreeToTheChainWhenNobodyTypedThem(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runWatch(t, d, "start", "octo/hello#3"); err != nil {
		t.Fatalf("watch start error = %v", err)
	}
	for _, field := range []string{"provider", "model", "effort", "keepWorktree"} {
		if _, named := d.starts[0][field]; named {
			t.Fatalf("body = %v, want no %s field so the repository, then the daemon, decides", d.starts[0], field)
		}
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

func TestWatchStartSendsTheWorktreeRuleThatWasTyped(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runWatch(t, d, "start", "octo/hello#3", "--keep-worktree", "--provider", "copilot"); err != nil {
		t.Fatalf("watch start error = %v", err)
	}
	if body := d.starts[0]; body["keepWorktree"] != true || body["provider"] != "copilot" {
		t.Fatalf("body = %v, want the worktree kept on copilot", body)
	}
}

func TestWatchStartSendsTheEffortThatWasTyped(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runWatch(t, d, "start", "octo/hello#3", "--model", "opus", "--effort", "max"); err != nil {
		t.Fatalf("watch start error = %v", err)
	}
	if body := d.starts[0]; body["model"] != "opus" || body["effort"] != "max" {
		t.Fatalf("body = %v, want opus at max effort", body)
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
		"nothing to change":  {"set"},
		"only a global flag": {"set", "-o", "json"},
		"approvals nonsense": {"set", "--approvals", "some"},
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

func TestSettingsSetTurnsTheNotificationsOff(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runSettings(t, d, "set", "--notifications=false")
	if err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if len(d.settingsPut) != 1 || d.settingsPut[0]["notificationsEnabled"] != false {
		t.Fatalf("body = %v, want the notifications off", d.settingsPut)
	}
	if !strings.Contains(out, "Show notifications") {
		t.Fatalf("settings set = %q, want the notification settings in the output", out)
	}
}

func TestSettingsSetSilencesTheNotifications(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runSettings(t, d, "set", "--notification-sound=false"); err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if len(d.settingsPut) != 1 || d.settingsPut[0]["notificationSound"] != false {
		t.Fatalf("body = %v, want the sound off", d.settingsPut)
	}
}

func TestSettingsSetPassesOnWhatTheDaemonRefuses(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"between 10s and 24h0m0s":               {"set", "--poll-interval", "2s"},
		"unknown merge method \"fast-forward\"": {"set", "--merge-method", "fast-forward"},
		"unknown notification kind \"rumour\"":  {"set", "--mute-notifications", "rumour"},
	}
	for want, args := range cases {
		t.Run(want, func(t *testing.T) {
			t.Parallel()
			d := newFakeDaemon()
			_, err := runSettings(t, d, args...)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %v, want the one the daemon gave about %q", err, want)
			}
		})
	}
}

func TestSettingsSetMutesTheNotificationKinds(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runSettings(t, d, "set", "--mute-notifications", "review,checks")
	if err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if len(d.settingsPut) != 1 {
		t.Fatalf("the daemon got %d writes, want 1", len(d.settingsPut))
	}
	got, ok := d.settingsPut[0]["mutedNotificationKinds"].([]any)
	if !ok || len(got) != 2 || got[0] != "review" || got[1] != "checks" {
		t.Fatalf("body = %v, want the review and the checks muted", d.settingsPut[0]["mutedNotificationKinds"])
	}
	if !strings.Contains(out, "review, checks") {
		t.Fatalf("settings set = %q, want the muted kinds in the output", out)
	}
}

func TestSettingsSetClearsTheMutedNotificationKinds(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.settings["mutedNotificationKinds"] = []string{"review"}

	if _, err := runSettings(t, d, "set", "--mute-notifications", ""); err != nil {
		t.Fatalf("settings set error = %v", err)
	}
	if got := d.settingsPut[0]["mutedNotificationKinds"].([]any); len(got) != 0 {
		t.Fatalf("body = %v, want every kind shown again", got)
	}
}

func TestSettingsGetPrintsTheMutedNotificationKinds(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.settings["mutedNotificationKinds"] = []string{"review", "merge"}

	out, err := runSettings(t, d, "get")
	if err != nil {
		t.Fatalf("settings get error = %v", err)
	}
	if !strings.Contains(out, "review, merge") {
		t.Fatalf("settings get = %q, want the muted kinds", out)
	}
}

func TestSettingsGetSaysWhenNoNotificationKindIsMuted(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runSettings(t, d, "get")
	if err != nil {
		t.Fatalf("settings get error = %v", err)
	}
	if !strings.Contains(out, "Muted notification kinds") || !strings.Contains(out, "none") {
		t.Fatalf("settings get = %q, want it to say that no kind is muted", out)
	}
}

func TestSettingsSetLeavesTheBoundOfTheApprovalsToTheDaemon(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runSettings(t, d, "set", "--approvals", "-1"); err == nil {
		t.Fatal("settings set took -1 approvals, want the answer of the daemon")
	}
	if len(d.settingsPut) != 1 || d.settingsPut[0]["approvalsRequired"] != float64(-1) {
		t.Fatalf("the daemon got %v, want the -1 the user typed", d.settingsPut)
	}
}
