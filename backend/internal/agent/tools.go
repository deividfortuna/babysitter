package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var authorDecisions = []string{
	"watch mode", "watch approve", "watch reject", "watch retry", "watch merge", "watch stop",
	"watch takeover", "watch handback",
}

func AuthorCommands(l Launch) []string {
	names := []string{"babysitter"}
	if len(l.Hook) > 0 {
		names = append(names, l.Hook[0], ShellWord(l.Hook[0]))
	}
	slices.Sort(names)
	names = slices.Compact(names)
	out := make([]string, 0, len(names)*len(authorDecisions))
	for _, name := range names {
		for _, decision := range authorDecisions {
			out = append(out, name+" "+decision)
		}
	}
	return out
}

func AuthorPatterns() []string {
	out := make([]string, 0, len(authorDecisions))
	for _, decision := range authorDecisions {
		out = append(out, "*babysitter* "+decision)
	}
	return out
}

const AuthorDecisionRefusal = "babysitter: only the author runs this command; do not run it again, " +
	"and say in your final message what the author has to decide"

var decisionSubcommands = decisionWords()

func decisionWords() []string {
	words := make([]string, 0, len(authorDecisions))
	for _, decision := range authorDecisions {
		words = append(words, strings.TrimPrefix(decision, "watch "))
	}
	return words
}

func IsAuthorDecision(command string) bool {
	return runsAuthorDecision(splitShellWords(command))
}

func runsAuthorDecision(words []shellWord) bool {
	for i := 0; i < len(words); i++ {
		word := words[i]
		if word.separator {
			continue
		}
		if namesBabysitter(word.text) {
			args := commandArgs(words[i+1:])
			subcommands := watchSubcommands(args)
			if slices.ContainsFunc(subcommands, isDecisionSubcommand) {
				return true
			}
			if len(subcommands) > 0 {
				if slices.ContainsFunc(args, substitutesAnAuthorDecision) {
					return true
				}
				i += len(args)
				continue
			}
		}
		if inner := splitShellWords(word.text); len(inner) > 1 && runsAuthorDecision(inner) {
			return true
		}
	}
	return false
}

func namesBabysitter(word string) bool {
	return strings.Contains(strings.ToLower(word), "babysitter")
}

func isDecisionSubcommand(subcommand string) bool {
	return slices.Contains(decisionSubcommands, subcommand)
}

func commandArgs(words []shellWord) []shellWord {
	end := slices.IndexFunc(words, func(w shellWord) bool { return w.separator })
	if end < 0 {
		return words
	}
	return words[:end]
}

func substitutesAnAuthorDecision(word shellWord) bool {
	substitutes := strings.Contains(word.text, "$(") || strings.Contains(word.text, "`")
	return substitutes && runsAuthorDecision(splitShellWords(word.text))
}

func watchSubcommands(args []shellWord) []string {
	var out []string
	for _, flagsTakeValues := range []bool{true, false} {
		positional := positionalWords(args, flagsTakeValues)
		if len(positional) >= 2 && positional[0] == "watch" {
			out = append(out, positional[1])
		}
	}
	return out
}

func positionalWords(args []shellWord, flagsTakeValues bool) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		text := args[i].text
		switch {
		case args[i].redirect:
			i++
		case text == "--":
			return out
		case !strings.HasPrefix(text, "-"):
			if text != "" {
				out = append(out, text)
			}
		case flagsTakeValues && flagTakesNextWord(text):
			i++
		}
	}
	return out
}

func flagTakesNextWord(flag string) bool {
	if strings.Contains(flag, "=") {
		return false
	}
	return strings.HasPrefix(flag, "--") || len(flag) == 2
}

func RefusesToolUse(event string, payload json.RawMessage) bool {
	return event == EventPreToolUse && IsAuthorDecision(toolCommand(payload))
}

func toolCommand(payload json.RawMessage) string {
	var fields map[string]any
	_ = json.Unmarshal(payload, &fields)
	for _, key := range []string{"toolArgs", "tool_input"} {
		if command, ok := toolArgs(fields[key])["command"].(string); ok {
			return command
		}
	}
	return ""
}

func toolArgs(value any) map[string]any {
	if encoded, ok := value.(string); ok {
		var args map[string]any
		_ = json.Unmarshal([]byte(encoded), &args)
		return args
	}
	args, _ := value.(map[string]any)
	return args
}

const prePushHook = `#!/bin/sh
echo "babysitter: the daemon pushes this branch. Commit your work and let the turn end; do not push." >&2
exit 1
`

const CoAuthorTrailer = "Co-authored-by: babysitter <335241182+babysitter-orchestrator@users.noreply.github.com>"

const commitMsgHook = `#!/bin/sh
exec git interpret-trailers --in-place --if-exists addIfDifferent --trailer "` + CoAuthorTrailer + `" "$1"
`

var gitHooks = map[string]string{
	"pre-push":   prePushHook,
	"commit-msg": commitMsgHook,
}

func GitEnv(l Launch) ([]string, error) {
	if l.HooksDir == "" {
		return nil, fmt.Errorf("a hooks directory is required")
	}
	if err := os.MkdirAll(l.HooksDir, 0o750); err != nil {
		return nil, fmt.Errorf("create git hooks dir: %w", err)
	}
	for name, script := range gitHooks {
		if err := os.WriteFile(filepath.Join(l.HooksDir, name), []byte(script), 0o750); err != nil {
			return nil, fmt.Errorf("write %s hook: %w", name, err)
		}
	}
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0=" + l.HooksDir,
	}, nil
}
