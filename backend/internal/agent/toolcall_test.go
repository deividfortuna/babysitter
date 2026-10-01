package agent

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestParseToolCallReadsThePayloadOfEveryAgent(t *testing.T) {
	t.Parallel()
	for payload, want := range map[string]ToolCall{
		`{"toolName":"bash","toolArgs":{"command":"go test ./..."}}`:       {Tool: "bash", Command: "go test ./..."},
		`{"toolName":"bash","toolArgs":"{\"command\":\"go test ./...\"}"}`: {Tool: "bash", Command: "go test ./..."},
		`{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}`:    {Tool: "bash", Command: "go test ./..."},
		`{"tool_name":"Read","tool_input":{"file_path":"/repo/main.go"}}`:  {Tool: "read"},
		`{"toolName":"bash","toolArgs":"not json"}`:                        {Tool: "bash"},
		`not json`: {},
		``:         {},
	} {
		got := ParseToolCall(json.RawMessage(payload))
		if got.Tool != want.Tool || got.Command != want.Command {
			t.Errorf("ParseToolCall(%s) = %q %q, want %q %q", payload, got.Tool, got.Command, want.Tool, want.Command)
		}
	}
}

func programs(command string) []string {
	var out []string
	for _, invocation := range ParseToolCall(shellCall(command)).Invocations {
		out = append(out, invocation.Program)
	}
	return out
}

func shellCall(command string) json.RawMessage {
	payload, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": command}})
	return payload
}

func TestInvocationsFindEveryProgramACommandRuns(t *testing.T) {
	t.Parallel()
	for command, want := range map[string][]string{
		"gh pr view 1 && git status":                 {"gh", "git"},
		"sh -c 'gh pr view 1'":                       {"sh", "gh"},
		"echo $(gh pr view 1)":                       {"echo", "gh"},
		`bash -lc "cd /repo; gh pr view 1"`:          {"bash", "cd", "gh"},
		"env GH_TOKEN=x gh pr view 1":                {"env", "gh"},
		"'/usr/local/bin/gh' pr view 1":              {"/usr/local/bin/gh"},
		`babysitter watch reply 7 "$(gh pr view 1)"`: {"babysitter", "gh"},
	} {
		got := programs(command)
		for _, program := range want {
			if !slices.Contains(got, program) {
				t.Errorf("programs(%q) = %q, lacks %q", command, got, program)
			}
		}
	}
}

func TestInvocationsReadTheArgumentsOfAWatchCommandAsText(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		"babysitter watch reply 7 'run gh pr view'",
		`babysitter -o json watch reply 7 "gh pr view shows the same"`,
	} {
		if got := programs(command); slices.Contains(got, "gh") {
			t.Errorf("programs(%q) = %q, reads the text of a reply as a command", command, got)
		}
	}
}

func TestRunsMatchesThePathAfterTheFlags(t *testing.T) {
	t.Parallel()
	path := []string{"pr", "view"}
	for command, want := range map[string]bool{
		"gh pr view 1":                 true,
		"gh -R octo/hello pr view 1":   true,
		"gh pr --repo octo/hello view": true,
		"gh --help pr view":            true,
		"gh 2>/dev/null pr view":       true,
		"gh pr list":                   false,
		"gh pr":                        false,
		"gh -- pr view":                false,
	} {
		invocation := ParseToolCall(shellCall(command)).Invocations[0]
		if got := invocation.runs(path); got != want {
			t.Errorf("runs(%q, %q) = %v, want %v", command, path, got, want)
		}
	}
}
