package agent

import (
	"encoding/json"
	"slices"
	"strings"
)

type State string

const (
	StateStarting     State = "starting"
	StateIdle         State = "idle"
	StateActive       State = "active"
	StateWaitingInput State = "waiting_input"
	StateBlocked      State = "blocked"
	StateExited       State = "exited"
	StateNone         State = "none"
)

var States = []State{StateNone, StateStarting, StateIdle, StateActive, StateWaitingInput, StateBlocked, StateExited}

func (s State) Valid() bool { return slices.Contains(States, s) }

func (s State) NeedsInput() bool { return s == StateWaitingInput || s == StateBlocked }

func (s State) EndsTurn() bool { return s == StateIdle || s == StateExited }

const (
	EventSessionStart      = "session-start"
	EventUserPromptSubmit  = "user-prompt-submit"
	EventPreToolUse        = "pre-tool-use"
	EventPostToolUse       = "post-tool-use"
	EventPostToolUseFailed = "post-tool-use-failure"
	EventPermissionRequest = "permission-request"
	EventStop              = "stop"
	EventNotification      = "notification"
	EventSessionEnd        = "session-end"
)

var Events = []string{
	EventSessionStart, EventUserPromptSubmit, EventPreToolUse, EventPostToolUse, EventPostToolUseFailed,
	EventPermissionRequest, EventStop, EventNotification, EventSessionEnd,
}

func ValidEvent(e string) bool { return slices.Contains(Events, e) }

func StateOf(event string, payload json.RawMessage) (State, bool) {
	switch event {
	case EventSessionStart, EventStop:
		return StateIdle, true
	case EventUserPromptSubmit, EventPreToolUse, EventPostToolUse, EventPostToolUseFailed:
		return StateActive, true
	case EventPermissionRequest:
		return StateBlocked, true
	case EventNotification:
		var p struct {
			Type string `json:"notification_type"`
		}
		_ = json.Unmarshal(payload, &p)
		switch strings.TrimSpace(p.Type) {
		case "idle_prompt", "agent_completed":
			return StateIdle, true
		case "agent_needs_input":
			return StateWaitingInput, true
		case "permission_prompt":
			return StateBlocked, true
		}
		return "", false
	case EventSessionEnd:
		var p struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(payload, &p)
		switch strings.TrimSpace(p.Reason) {
		case "clear", "resume":
			return "", false
		}
		return StateExited, true
	}
	return "", false
}
