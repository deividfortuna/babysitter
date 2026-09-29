package snapshot

import (
	"fmt"
	"strings"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

const (
	ActionProcessReviewComment = "process_review_comment"
	ActionStopPRClosed         = "stop_pr_closed"
	ActionReadyToMerge         = "ready_to_merge"
	ActionDiagnoseCIFailure    = "diagnose_ci_failure"
	ActionRetryFailedChecks    = "retry_failed_checks"
	ActionStopExhaustedRetries = "stop_exhausted_retries"
	ActionStopActionRequired   = "stop_action_required"
	ActionIdle                 = "idle"
)

func Blockers(s *Snapshot, approvals int) []string {
	if out := endedBlockers(s.PR); out != nil {
		return out
	}
	var out []string
	out = append(out, draftBlockers(s.PR)...)
	out = append(out, checkBlockers(s)...)
	out = append(out, mergeableBlockers(s.PR)...)
	out = append(out, reviewBlockers(s.PR, approvals)...)
	out = append(out, threadBlockers(s)...)
	out = append(out, itemBlockers(s)...)
	return out
}

func BuildGreen(s *Snapshot) bool {
	return endedBlockers(s.PR) == nil && len(checkBlockers(s)) == 0
}

func WaitsOnlyForReview(s *Snapshot, approvals int) bool {
	if endedBlockers(s.PR) != nil {
		return false
	}
	review := reviewerBlockers(s, approvals)
	return len(review) > 0 && len(Blockers(s, approvals)) == len(review)
}

func Rereviewers(s *Snapshot, approvals int) []string {
	var behind []string
	if len(reviewBlockers(s.PR, approvals)) > 0 {
		behind = s.PR.ReviewersBehindHead
	}
	return withRequests(behind, s.Threads.Reviewers)
}

func reviewerBlockers(s *Snapshot, approvals int) []string {
	out := reviewBlockers(s.PR, approvals)
	if s.Threads.waitOnReviewer() {
		out = append(out, threadBlockers(s)...)
	}
	return out
}

func endedBlockers(pr PR) []string {
	switch {
	case pr.Merged:
		return []string{"already merged"}
	case pr.Closed:
		return []string{"closed"}
	}
	return nil
}

func draftBlockers(pr PR) []string {
	if pr.Draft {
		return []string{"still a draft"}
	}
	return nil
}

func checkBlockers(s *Snapshot) []string {
	var out []string
	c := s.Checks
	if c.FailedCount > 0 {
		out = append(out, textx.Plural(c.FailedCount, "check")+" failed")
	}
	if c.PendingCount > 0 || !c.AllTerminal {
		out = append(out, textx.Plural(c.PendingCount, "check")+" pending")
	}
	if runs := s.FailedRunsWithoutCheck(); len(runs) > 0 {
		out = append(out, textx.Plural(len(runs), "workflow run")+" failed")
	}
	if len(s.AwaitingApproval) > 0 {
		out = append(out, textx.Plural(len(s.AwaitingApproval), "workflow run")+" waits for a person")
	}
	if c.Status == checks.CINone && len(out) == 0 {
		out = append(out, "no checks ran")
	}
	return out
}

func mergeableBlockers(pr PR) []string {
	switch {
	case pr.Mergeable == nil || !pr.MergeableState.Known():
		return []string{"GitHub has not computed the mergeable state yet"}
	case !*pr.Mergeable:
		return []string{"GitHub reports it cannot merge"}
	case pr.MergeableState != store.MergeableClean:
		return []string{"GitHub reports " + mergeableWord(pr.MergeableState) + blockedBehindWord(pr)}
	}
	return nil
}

func blockedBehindWord(pr PR) string {
	switch {
	case pr.BehindErr != "":
		return ", and the compare with " + pr.BaseBranch + " failed: " + pr.BehindErr
	case pr.blockedBehind():
		return fmt.Sprintf(", and the branch is %s behind %s", textx.Plural(pr.BehindBy, "commit"), pr.BaseBranch)
	}
	return ""
}

func reviewBlockers(pr PR, approvals int) []string {
	var out []string
	if pr.ChangesRequested > 0 {
		out = append(out, textx.Plural(pr.ChangesRequested, "reviewer")+" requested changes")
	}
	switch {
	case approvals == 0:
	case pr.Approvals == 0:
		out = append(out, "no approval yet")
	case pr.Approvals < approvals:
		out = append(out, fmt.Sprintf("%d of %d approvals", pr.Approvals, approvals))
	}
	if len(pr.RequestedReviewers) > 0 {
		out = append(out, "waiting for a review from "+strings.Join(pr.RequestedReviewers, ", "))
	}
	return out
}

func threadBlockers(s *Snapshot) []string {
	switch {
	case s.Threads.Err != "":
		return []string{"review threads unreadable: " + s.Threads.Err}
	case s.Threads.Unresolved > 0:
		return []string{textx.Plural(s.Threads.Unresolved, "review thread") + " unresolved"}
	}
	return nil
}

func itemBlockers(s *Snapshot) []string {
	if len(s.NewReviewItems) > 0 {
		return []string{textx.Plural(len(s.NewReviewItems), "new review item") + " not looked at yet"}
	}
	return nil
}

func mergeableWord(m store.MergeableState) string {
	switch m {
	case store.MergeableDirty:
		return "conflicts"
	case store.MergeableUnstable:
		return "a failing non required check"
	case store.MergeableHasHooks:
		return "pre receive hooks still running"
	default:
		return string(m)
	}
}

func readyToMerge(s *Snapshot) bool {
	return len(Blockers(s, 1)) == 0
}

func needsAction(s *Snapshot) bool {
	if len(s.AwaitingApproval) > 0 {
		return true
	}
	for _, c := range s.Checks.Items {
		if checks.Conclusion(c.Conclusion) == checks.ConclusionActionRequired || checks.RunStatus(c.Status) == checks.StatusWaiting {
			return true
		}
	}
	return false
}

func recommend(s *Snapshot) []string {
	var actions []string
	if s.PR.Closed || s.PR.Merged {
		if len(s.NewReviewItems) > 0 {
			actions = append(actions, ActionProcessReviewComment)
		}
		return append(actions, ActionStopPRClosed)
	}
	if readyToMerge(s) {
		return []string{ActionReadyToMerge}
	}
	if len(s.NewReviewItems) > 0 {
		actions = append(actions, ActionProcessReviewComment)
	}
	if s.Checks.FailedCount > 0 || len(s.FailedRuns) > 0 || len(s.FailedJobs) > 0 {
		actions = append(actions, ActionDiagnoseCIFailure)
		used, allowed := s.RetryState.CurrentSHARetriesUsed, s.RetryState.MaxFlakyRetries
		switch {
		case !s.Checks.AllTerminal || len(s.FailedRuns) == 0:
		case used < allowed:
			actions = append(actions, ActionRetryFailedChecks)
		default:
			actions = append(actions, ActionStopExhaustedRetries)
		}
	}
	if needsAction(s) {
		actions = append(actions, ActionStopActionRequired)
	}
	if len(actions) == 0 {
		actions = append(actions, ActionIdle)
	}
	return actions
}
