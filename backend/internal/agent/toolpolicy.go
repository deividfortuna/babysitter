package agent

import (
	"path/filepath"
	"slices"
	"strings"
)

type ToolFacts struct {
	WatchID int64
	Exe     string
	Live    bool
}

type ToolVerdict struct {
	Deny   bool
	Rule   string
	Reason string
}

const (
	ToolAllow = "allow"
	ToolDeny  = "deny"
)

func (v ToolVerdict) Decision() string {
	if v.Deny {
		return ToolDeny
	}
	return ToolAllow
}

func DecideTool(event string, call ToolCall, f ToolFacts) ToolVerdict {
	if event != EventPreToolUse {
		return ToolVerdict{}
	}
	return evaluateTool(call, f, toolRules)
}

type toolRule interface {
	id() string
	check(call ToolCall, f ToolFacts) (reason string, denied bool)
}

var toolRules = slices.Concat(stateRules, authorRules())

func evaluateTool(call ToolCall, f ToolFacts, rules []toolRule) ToolVerdict {
	for _, r := range rules {
		if reason, denied := r.check(call, f); denied {
			return ToolVerdict{Deny: true, Rule: r.id(), Reason: reason}
		}
	}
	return ToolVerdict{}
}

const NoSessionRefusal = "babysitter: the daemon runs no agent session for this watch, so it refuses every tool; " +
	"stop and say in your final message that the session of the watch is gone"

var stateRules = []toolRule{noSession{}}

type noSession struct{}

func (noSession) id() string { return "no-session" }

func (noSession) check(_ ToolCall, f ToolFacts) (string, bool) {
	return NoSessionRefusal, !f.Live
}

type commandRule struct {
	name    string
	program string
	names   func(lowered string, f ToolFacts) bool
	path    []string
	reason  func(ToolFacts) string
}

func (r commandRule) id() string { return r.name }

func (r commandRule) check(call ToolCall, f ToolFacts) (string, bool) {
	for _, invocation := range call.Invocations {
		if r.names(invocation.lowered, f) && invocation.runs(r.path) {
			return r.reason(f), true
		}
	}
	return "", false
}

const AuthorDecisionRefusal = "babysitter: only the author runs this command; do not run it again, " +
	"and say in your final message what the author has to decide"

func authorOnly(ToolFacts) string { return AuthorDecisionRefusal }

func namesDaemon(lowered string, f ToolFacts) bool {
	return namesBabysitter(lowered) || namesExecutable(lowered, f.Exe)
}

func namesExecutable(lowered, exe string) bool {
	if exe == "" {
		return false
	}
	return filepath.Base(lowered) == strings.ToLower(filepath.Base(exe))
}

func authorRules() []toolRule {
	out := make([]toolRule, 0, len(authorDecisions))
	for _, decision := range authorDecisions {
		out = append(out, commandRule{
			name:    "author-" + decision[len(decision)-1],
			program: "babysitter",
			names:   namesDaemon,
			path:    decision,
			reason:  authorOnly,
		})
	}
	return out
}
