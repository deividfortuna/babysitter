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
	StateWaiting      State = "waiting"
	StateWaitingInput State = "waiting_input"
	StateBlocked      State = "blocked"
	StateExited       State = "exited"
	StateNone         State = "none"
)

var States = []State{StateNone, StateStarting, StateIdle, StateActive, StateWaiting, StateWaitingInput, StateBlocked, StateExited}

func (s State) Valid() bool { return slices.Contains(States, s) }

func (s State) NeedsInput() bool { return s == StateWaitingInput || s == StateBlocked }

func (s State) EndsTurn() bool { return s == StateIdle || s == StateWaiting || s == StateExited }

func (s State) Working() bool { return s == StateStarting || s == StateActive }

const (
	EventSessionStart      = "session-start"
	EventUserPromptSubmit  = "user-prompt-submit"
	EventPreToolUse        = "pre-tool-use"
	EventPostToolUse       = "post-tool-use"
	EventPostToolUseFailed = "post-tool-use-failure"
	EventPermissionRequest = "permission-request"
	EventStop              = "stop"
	EventStopFailure       = "stop-failure"
	EventNotification      = "notification"
	EventSessionEnd        = "session-end"
)

var Events = []string{
	EventSessionStart, EventUserPromptSubmit, EventPreToolUse, EventPostToolUse, EventPostToolUseFailed,
	EventPermissionRequest, EventStop, EventStopFailure, EventNotification, EventSessionEnd,
}

func ValidEvent(e string) bool { return slices.Contains(Events, e) }

func StateOf(event string, payload json.RawMessage) (State, bool) {
	switch event {
	case EventSessionStart:
		if payloadField(payload, "source") == "compact" {
			return "", false
		}
		return StateIdle, true
	case EventStop:
		if hasBackgroundWork(payload) {
			return StateWaiting, true
		}
		return StateIdle, true
	case EventStopFailure:
		return StateIdle, true
	case EventUserPromptSubmit, EventPreToolUse, EventPostToolUse, EventPostToolUseFailed:
		return StateActive, true
	case EventPermissionRequest:
		return StateBlocked, true
	case EventNotification:
		switch payloadField(payload, "notification_type") {
		case "idle_prompt", "agent_completed":
			return StateIdle, true
		case "agent_needs_input":
			return StateWaitingInput, true
		case "permission_prompt":
			return StateBlocked, true
		}
		return "", false
	case EventSessionEnd:
		switch payloadField(payload, "reason") {
		case "clear", "resume":
			return "", false
		}
		return StateExited, true
	}
	return "", false
}

func (s State) Next(event string, payload json.RawMessage) (State, bool) {
	compaction := event == EventSessionStart && payloadField(payload, "source") == "compact"
	if compaction {
		return s, true
	}
	next, ok := StateOf(event, payload)
	idleNotice := event == EventNotification && next == StateIdle
	if s == StateWaiting && idleNotice {
		return "", false
	}
	return next, ok
}

func payloadField(payload json.RawMessage, name string) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil {
		return ""
	}
	var value string
	if json.Unmarshal(fields[name], &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func hasBackgroundWork(payload json.RawMessage) bool {
	var stop struct {
		BackgroundTasks []json.RawMessage `json:"background_tasks"`
	}
	if json.Unmarshal(payload, &stop) != nil {
		return false
	}
	return len(stop.BackgroundTasks) > 0
}
