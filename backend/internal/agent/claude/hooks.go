package claude

import (
	"encoding/json"

	"github.com/deividfortuna/babysitter/internal/agent"
)

var hooks = []struct {
	key, event, matcher string
}{
	{"SessionStart", agent.EventSessionStart, "startup|resume|clear|compact"},
	{"UserPromptSubmit", agent.EventUserPromptSubmit, ""},
	{"PreToolUse", agent.EventPreToolUse, ""},
	{"PostToolUse", agent.EventPostToolUse, ""},
	{"PostToolUseFailure", agent.EventPostToolUseFailed, ""},
	{"PermissionRequest", agent.EventPermissionRequest, ""},
	{"Stop", agent.EventStop, ""},
	{"Notification", agent.EventNotification, ""},
	{"SessionEnd", agent.EventSessionEnd, ""},
}

const hookTimeout = 10

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type hookEntry struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

type hookSettingsFile struct {
	Hooks map[string][]hookEntry `json:"hooks"`
}

func hookSettings(hook []string) string {
	settings := hookSettingsFile{Hooks: map[string][]hookEntry{}}
	for _, h := range hooks {
		cmd := agent.ShellJoin(append(append([]string(nil), hook...), h.event))
		settings.Hooks[h.key] = []hookEntry{{Matcher: h.matcher, Hooks: []hookCommand{{Type: "command", Command: cmd, Timeout: hookTimeout}}}}
	}
	b, _ := json.Marshal(settings)
	return string(b)
}
