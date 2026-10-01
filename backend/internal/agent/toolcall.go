package agent

import (
	"encoding/json"
	"slices"
	"strings"
)

type ToolCall struct {
	Tool        string
	Command     string
	Invocations []Invocation
}

type Invocation struct {
	Program string
	lowered string
	args    []shellWord
}

func ParseToolCall(payload json.RawMessage) ToolCall {
	var fields map[string]any
	_ = json.Unmarshal(payload, &fields)
	call := ToolCall{Tool: strings.ToLower(firstString(fields, "toolName", "tool_name"))}
	for _, key := range []string{"toolArgs", "tool_input"} {
		if command, ok := toolArgs(fields[key])["command"].(string); ok {
			call.Command = command
			break
		}
	}
	call.Invocations = invocations(splitShellWords(call.Command))
	return call
}

func firstString(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := fields[key].(string); ok {
			return value
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

func invocations(words []shellWord) []Invocation {
	var out []Invocation
	for i := 0; i < len(words); i++ {
		word := words[i]
		if word.separator {
			continue
		}
		args := commandArgs(words[i+1:])
		invocation := Invocation{Program: word.text, lowered: strings.ToLower(word.text), args: args}
		out = append(out, invocation)
		if invocation.takesText() {
			out = append(out, substitutedInvocations(args)...)
			i += len(args)
			continue
		}
		if inner := splitShellWords(word.text); len(inner) > 1 {
			out = append(out, invocations(inner)...)
		}
	}
	return out
}

func commandArgs(words []shellWord) []shellWord {
	end := slices.IndexFunc(words, func(w shellWord) bool { return w.separator })
	if end < 0 {
		return words
	}
	return words[:end]
}

func substitutedInvocations(args []shellWord) []Invocation {
	var out []Invocation
	for _, arg := range args {
		if strings.Contains(arg.text, "$(") || strings.Contains(arg.text, "`") {
			out = append(out, invocations(splitShellWords(arg.text))...)
		}
	}
	return out
}

func (inv Invocation) takesText() bool {
	return namesBabysitter(inv.lowered) && slices.ContainsFunc(inv.readings(), isWatchSubcommand)
}

func isWatchSubcommand(positional []string) bool {
	return len(positional) >= 2 && positional[0] == "watch"
}

func namesBabysitter(lowered string) bool {
	return strings.Contains(lowered, "babysitter")
}

func (inv Invocation) runs(path []string) bool {
	return slices.ContainsFunc(inv.readings(), func(positional []string) bool {
		return len(positional) >= len(path) && slices.Equal(positional[:len(path)], path)
	})
}

func (inv Invocation) readings() [][]string {
	return [][]string{positionalWords(inv.args, true), positionalWords(inv.args, false)}
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
