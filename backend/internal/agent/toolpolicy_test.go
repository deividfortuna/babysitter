package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

var liveFacts = ToolFacts{WatchID: 7, Exe: "babysitter", Live: true}

func decide(command string) ToolVerdict {
	return DecideTool(EventPreToolUse, ParseToolCall(shellCall(command)), liveFacts)
}

func refusedAsAuthorOnly(v ToolVerdict) bool {
	return v.Deny && strings.HasPrefix(v.Rule, "author-") && v.Reason == AuthorDecisionRefusal
}

func TestDecideRefusesEverySpellingOfAnAuthorDecision(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		"babysitter watch mode 1 auto",
		"babysitter -o json watch reject 1 --reason x",
		"babysitter --data-dir /tmp/x watch stop",
		"/usr/local/bin/babysitter --output json watch approve 1",
		"'/opt/my tools/babysitter' watch merge 1",
		"sh -c 'babysitter -o text watch retry 1'",
		"cd /repo && babysitter watch takeover 1",
		"babysitter watch\thandback 1",
		"babysitter \\\n  watch merge 1",
		"baby''sitter watch reject 1",
		`baby""sitter watch approve 1`,
		`babysitter watch "reject" 1`,
		`babysitter wat'ch' mer""ge 1`,
		`baby\sitter watch st\op 1`,
		"babysitter watch reply 7 done && babysitter watch merge 7",
		`babysitter watch reply 7 "$(babysitter watch merge 7)"`,
		"babysitter watch reply 7 `babysitter watch merge 7`",
		`babysitter -o "a watch reply" watch merge 1`,
		"babysitter --data-dir /tmp/watch watch merge 1",
		"babysitter watch --reason x reject 1",
		"echo $(babysitter watch merge 1)",
		`bash -lc "babysitter watch merge 1"`,
		"env BABYSITTER_WATCH=7 babysitter watch merge 1",
		"babysitter >/tmp/out watch merge 7",
		"babysitter > /tmp/out watch merge 7",
		"babysitter 2>/dev/null watch merge 7",
		"babysitter 2>&1 watch merge 7",
		"babysitter &>/tmp/out watch merge 7",
		"babysitter &>> /tmp/out watch merge 7",
		"babysitter </dev/null watch merge 7",
		"babysitter <<<x watch merge 7",
		"babysitter >|/tmp/out -o json watch merge 7",
		"babysitter {fd}>/tmp/out watch merge 7",
		"babysitter -o json 2>&1 watch merge 7",
		"BABYSITTER watch merge 7",
		"/usr/local/bin/BabySitter -o json watch approve 7",
		`baby$'\x73'itter watch reject 1`,
		`$'\142abysitter' watch merge 1`,
		`$'babysitter' watch $'m\145rge' 1`,
		`babysitter watch $'\x72eject' 1`,
		`$'baby\U73itter' watch stop 1`,
		`baby$"sit"ter watch merge 1`,
		"ls # a comment\nbabysitter watch merge 1",
		"babysitter watch merge 1#2",
		"sh <<EOF\nbabysitter watch merge 1\nEOF",
		"babysitter auth logout",
		"babysitter --data-dir /tmp/x auth login",
		"sh -c 'babysitter auth logout'",
	} {
		if v := decide(command); !refusedAsAuthorOnly(v) {
			t.Errorf("DecideTool(%q) = %+v", command, v)
		}
	}
	for _, command := range []string{
		"babysitter watch reply 1 fixed in the last commit",
		`babysitter watch reply 7 "please watch merge when ready"`,
		"babysitter -o json watch reply 7 'I did not run babysitter watch merge'",
		`babysitter watch reply --to 42 7 "the author runs watch approve"`,
		"babysitter watch reply 7 'merge > reject' 2>&1",
		"babysitter 2>/dev/null watch reply 7 'please watch merge' >/tmp/out",
		`babysitter watch reply 7 $'line one\nwatch merge is for the author'`,
		"go test ./... # do not run babysitter watch merge 1",
		"# babysitter watch merge 1\ngit status",
		"babysitter -o json watch status 1",
		"babysitter watch modes",
		"go test ./internal/prwatch/ -run TestWatchMerge",
		"git commit -m 'watch merge readiness'",
		"",
	} {
		if v := decide(command); v.Deny {
			t.Errorf("DecideTool(%q) = %+v", command, v)
		}
	}
}

func spellings(program string, path []string) []string {
	run := program + " " + strings.Join(path, " ")
	flagged := program + " -o json " + path[0] + " --reason x " + strings.Join(path[1:], " ")
	return []string{
		run,
		run + " 1",
		"/usr/local/bin/" + run,
		"'" + program + "' " + strings.Join(path, " "),
		program[:1] + "''" + program[1:] + " " + strings.Join(path, " "),
		strings.ToUpper(program) + " " + strings.Join(path, " "),
		flagged,
		program + " 2>/dev/null " + strings.Join(path, " "),
		"env X=1 " + run,
		"cd /repo && " + run,
		"true; " + run,
		"ls # a comment\n" + run,
		"sh -c '" + run + "'",
		`bash -lc "` + run + `"`,
		"echo $(" + run + ")",
		"echo `" + run + "`",
	}
}

func TestEveryCommandRuleRefusesEverySpelling(t *testing.T) {
	t.Parallel()
	for _, r := range toolRules {
		command, ok := r.(commandRule)
		if !ok {
			continue
		}
		want := ToolVerdict{Deny: true, Rule: command.name, Reason: command.reason(liveFacts)}
		for _, spelling := range spellings(command.program, command.path) {
			if v := decide(spelling); v != want {
				t.Errorf("%s: DecideTool(%q) = %+v", command.name, spelling, v)
			}
		}
		near := command.program + " " + strings.Join(command.path, " ") + "s"
		if v := decide(near); v.Deny {
			t.Errorf("%s: DecideTool(%q) = %+v", command.name, near, v)
		}
	}
}

func TestDecideRefusesADecisionOfTheDaemonExecutable(t *testing.T) {
	t.Parallel()
	renamed := ToolFacts{WatchID: 7, Exe: "/opt/tools/watchd", Live: true}
	for _, command := range []string{
		"/opt/tools/watchd -o json watch merge 7",
		"watchd watch approve 7",
		"WATCHD watch reject 7",
		"sh -c 'wat''chd --data-dir /tmp/x watch stop 7'",
		"echo $(/opt/tools/watchd watch merge 7)",
	} {
		if v := DecideTool(EventPreToolUse, ParseToolCall(shellCall(command)), renamed); !refusedAsAuthorOnly(v) {
			t.Errorf("DecideTool(%q) = %+v", command, v)
		}
	}
	for _, command := range []string{
		"/opt/tools/watchd watch reply 7 done",
		"/opt/tools/watchd -o json watch status 7",
		"watchdog watch merge 7",
	} {
		if v := DecideTool(EventPreToolUse, ParseToolCall(shellCall(command)), renamed); v.Deny {
			t.Errorf("DecideTool(%q) = %+v", command, v)
		}
	}
}

func TestDecideReadsTheCommandOfEveryAgent(t *testing.T) {
	t.Parallel()
	const decision = "babysitter -o json watch reject 1 --reason x"
	refused := []string{
		`{"toolName":"bash","toolArgs":{"command":"` + decision + `"}}`,
		`{"toolName":"bash","toolArgs":"{\"command\":\"` + decision + `\"}"}`,
		`{"tool_name":"Bash","tool_input":{"command":"` + decision + `"}}`,
	}
	for _, payload := range refused {
		if v := DecideTool(EventPreToolUse, ParseToolCall(json.RawMessage(payload)), liveFacts); !v.Deny {
			t.Errorf("DecideTool(%s) = %+v", payload, v)
		}
		if v := DecideTool(EventPostToolUse, ParseToolCall(json.RawMessage(payload)), liveFacts); v.Deny {
			t.Errorf("DecideTool() refuses a tool that already ran: %s", payload)
		}
	}
	allowed := []string{
		`{"toolName":"bash","toolArgs":{"command":"babysitter watch reply 1 done"}}`,
		`{"tool_name":"Read","tool_input":{"file_path":"` + decision + `"}}`,
		`{"toolName":"bash","toolArgs":"not json"}`,
		`{}`,
		`not json`,
		``,
	}
	for _, payload := range allowed {
		if v := DecideTool(EventPreToolUse, ParseToolCall(json.RawMessage(payload)), liveFacts); v.Deny {
			t.Errorf("DecideTool(%s) = %+v", payload, v)
		}
	}
}

func TestDecideRefusesEveryToolWithoutASession(t *testing.T) {
	t.Parallel()
	gone := ToolFacts{WatchID: 7, Exe: "babysitter"}
	v := DecideTool(EventPreToolUse, ParseToolCall(shellCall("ls")), gone)
	if want := (ToolVerdict{Deny: true, Rule: "no-session", Reason: NoSessionRefusal}); v != want {
		t.Fatalf("DecideTool() without a session = %+v, want %+v", v, want)
	}
	if v := DecideTool(EventStop, ToolCall{}, gone); v.Deny {
		t.Fatalf("DecideTool() refuses a stop without a session: %+v", v)
	}
}
