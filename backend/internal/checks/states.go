package checks

import (
	"fmt"
	"slices"
	"strings"
)

type State string

const (
	Pending State = "pending"
	Failed  State = "failed"
	Passed  State = "passed"
	Skipped State = "skipped"
)

var states = []State{Pending, Failed, Passed, Skipped}

func (s State) Valid() bool { return slices.Contains(states, s) }

type CIStatus string

const (
	CISuccess CIStatus = "success"
	CIFailure CIStatus = "failure"
	CIPending CIStatus = "pending"
	CINone    CIStatus = "none"
)

var ciStatuses = []CIStatus{CISuccess, CIFailure, CIPending, CINone}

func (s CIStatus) Valid() bool { return slices.Contains(ciStatuses, s) }

type Source string

const (
	SourceCheckRun    Source = "check_run"
	SourceStatus      Source = "status"
	SourceWorkflowRun Source = "workflow_run"
)

type RunStatus string

const (
	StatusCompleted      RunStatus = "completed"
	StatusWaiting        RunStatus = "waiting"
	StatusActionRequired RunStatus = "action_required"
)

type Conclusion string

const (
	ConclusionSuccess        Conclusion = "success"
	ConclusionFailure        Conclusion = "failure"
	ConclusionTimedOut       Conclusion = "timed_out"
	ConclusionCancelled      Conclusion = "cancelled"
	ConclusionStartupFailure Conclusion = "startup_failure"
	ConclusionActionRequired Conclusion = "action_required"
)

var failedConclusions = []Conclusion{
	ConclusionFailure, ConclusionTimedOut, ConclusionCancelled, ConclusionStartupFailure,
}

func (c Conclusion) Failed() bool { return slices.Contains(failedConclusions, c) }

type LegacyState string

const (
	LegacyPending LegacyState = "pending"
	LegacyFailure LegacyState = "failure"
	LegacyError   LegacyState = "error"
	LegacySuccess LegacyState = "success"
)

func CountPassed(states map[string]State) int {
	n := 0
	for _, state := range states {
		if state == Passed {
			n++
		}
	}
	return n
}

func Summarize(states map[string]State, headSHA, greenSHA string) string {
	var failed []string
	pending := 0
	for name, state := range states {
		switch state {
		case Failed:
			failed = append(failed, name)
		case Pending:
			pending++
		default:
		}
	}
	switch {
	case len(failed) > 0:
		slices.Sort(failed)
		return "failing: " + strings.Join(failed, ", ")
	case pending > 0:
		return fmt.Sprintf("pending (%d)", pending)
	case greenSHA != "" && greenSHA == headSHA:
		return "green"
	case len(states) == 0:
		return "none"
	default:
		return "passed"
	}
}
