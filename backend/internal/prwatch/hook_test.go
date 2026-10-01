package prwatch

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent"
)

func loggedHook(t *testing.T, event, payload string, verdict agent.ToolVerdict) map[string]any {
	t.Helper()
	var out bytes.Buffer
	s := &Service{log: slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	s.logHook(context.Background(), 7, event, agent.ParseToolCall(json.RawMessage(payload)), verdict)
	var line map[string]any
	if err := json.Unmarshal(out.Bytes(), &line); err != nil {
		t.Fatalf("log line %q: %v", out.String(), err)
	}
	return line
}

func TestLogHookWritesEveryCallOfTheHook(t *testing.T) {
	t.Parallel()
	refused := agent.ToolVerdict{Deny: true, Rule: "author-merge", Reason: agent.AuthorDecisionRefusal}
	line := loggedHook(t, agent.EventPreToolUse,
		`{"tool_name":"Bash","tool_input":{"command":"GH_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz1234 \\\n  babysitter watch merge 7"}}`, refused)
	want := map[string]any{
		"level": "INFO", "msg": "the agent called its hook", "watch": float64(7), "event": agent.EventPreToolUse,
		"decision": "deny", "tool": "bash", "rule": "author-merge",
		"command": `GH_TOKEN=[redacted] \ babysitter watch merge 7`,
	}
	for key, value := range want {
		if line[key] != value {
			t.Errorf("%s = %v, want %v", key, line[key], value)
		}
	}

	line = loggedHook(t, agent.EventPreToolUse, `{"toolName":"bash","toolArgs":{"command":"go test ./..."}}`, agent.ToolVerdict{})
	for key, value := range map[string]any{"level": "DEBUG", "decision": "allow", "tool": "bash", "command": "go test ./..."} {
		if line[key] != value {
			t.Errorf("allowed tool line %s = %v, want %v", key, line[key], value)
		}
	}

	line = loggedHook(t, agent.EventStop, `{"session_id":"s"}`, agent.ToolVerdict{})
	for key, value := range map[string]any{"level": "DEBUG", "event": agent.EventStop, "decision": "allow"} {
		if line[key] != value {
			t.Errorf("stop line %s = %v, want %v", key, line[key], value)
		}
	}
	for _, key := range []string{"tool", "command", "rule"} {
		if _, ok := line[key]; ok {
			t.Errorf("stop line has %s: %v", key, line)
		}
	}
}

func TestLoggedCommandRemovesSecretsBeforeTheCut(t *testing.T) {
	t.Parallel()
	key := "cat > k.pem <<EOF\n-----BEGIN PRIVATE KEY-----\n" + strings.Repeat("MIIEvQIBADANBgkqhkiG9w0BAQEFAASC", 10) +
		"\n-----END PRIVATE KEY-----\nEOF"
	token := strings.Repeat("x", hookCommandWidth-12) + " ghp_abcdefghijklmnopqrstuvwxyz1234"
	for command, secret := range map[string]string{key: "MIIEvQIBADAN", token: "ghp_"} {
		if got := loggedCommand(command); strings.Contains(got, secret) {
			t.Errorf("loggedCommand() = %q, keeps %q", got, secret)
		}
	}
}
