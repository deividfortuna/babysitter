package claude

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent"
)

func TestHookSettings(t *testing.T) {
	t.Parallel()
	settings := hookSettings([]string{"/usr/local/bin/babysitter", "watch", "hook", "--watch", "7"})
	var parsed hookSettingsFile
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		t.Fatalf("settings %q: %v", settings, err)
	}
	if len(parsed.Hooks) != len(hooks) {
		t.Fatalf("hooks = %d, want %d", len(parsed.Hooks), len(hooks))
	}
	for _, h := range hooks {
		entries := parsed.Hooks[h.key]
		if len(entries) != 1 || len(entries[0].Hooks) != 1 {
			t.Fatalf("%s = %+v", h.key, entries)
		}
		cmd := entries[0].Hooks[0]
		want := "'/usr/local/bin/babysitter' 'watch' 'hook' '--watch' '7' '" + h.event + "'"
		if cmd.Type != "command" || cmd.Command != want || cmd.Timeout != hookTimeout {
			t.Errorf("%s command = %+v, want %q", h.key, cmd, want)
		}
		if entries[0].Matcher != h.matcher {
			t.Errorf("%s matcher = %q, want %q", h.key, entries[0].Matcher, h.matcher)
		}
		if !agent.ValidEvent(h.event) {
			t.Errorf("%s reports %q, which the daemon does not know", h.key, h.event)
		}
	}
	if start := parsed.Hooks["SessionStart"]; start[0].Matcher != "startup|resume|clear|compact" {
		t.Fatalf("session start matcher = %q", start[0].Matcher)
	}
	if strings.Contains(settings, `"matcher":""`) {
		t.Fatalf("an empty matcher is written: %s", settings)
	}
}

func TestHookSettingsQuotesTheCommand(t *testing.T) {
	t.Parallel()
	settings := hookSettings([]string{"/opt/my tools/babysitter", "watch", "hook", "--data-dir", "/data/it's"})
	var parsed hookSettingsFile
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		t.Fatalf("settings %q: %v", settings, err)
	}
	got := parsed.Hooks["Stop"][0].Hooks[0].Command
	if want := `'/opt/my tools/babysitter' 'watch' 'hook' '--data-dir' '/data/it'\''s' 'stop'`; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
}
