package prwatch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

func (s *Service) blockers(w store.Watch, snap *snapshot.Snapshot, state agentStatus) []string {
	out := snapshot.Blockers(snap, w.ApprovalsRequired)
	for i := range out {
		out[i] = redact.Text(out[i])
	}
	for _, own := range []string{state.untold, state.proposal, state.author} {
		if own != "" {
			out = append(out, own)
		}
	}
	return out
}

func untold(pending []store.Activity) string {
	if len(pending) == 0 {
		return ""
	}
	return fmt.Sprintf("the agent was not told about %s yet", agent.Summarize(toItems(pending)))
}

func Readiness(w store.Watch, state agent.State, now time.Time, interval time.Duration) (*time.Time, []string) {
	blockers := append([]string{}, w.ReadyBlockers...)
	if word, busy := agentBusyWord(state); busy {
		return nil, append(blockers, word)
	}
	if limit := limitBlocker(w); limit != "" {
		return nil, append(blockers, limit)
	}
	if !settled(w.ReadySince, now, interval) {
		return nil, blockers
	}
	return w.ReadySince, blockers
}

func settled(since *time.Time, now time.Time, interval time.Duration) bool {
	return since != nil && !now.Before(since.Add(interval))
}

func (s *Service) Readiness(w store.Watch, state agent.State) (*time.Time, []string) {
	return Readiness(w, state, s.now(), s.Interval())
}

func agentBusyWord(state agent.State) (string, bool) {
	switch state {
	case agent.StateStarting:
		return "the agent is starting", true
	case agent.StateActive:
		return "the agent is still working", true
	case agent.StateWaiting:
		return "the background work of the agent still runs", true
	case agent.StateWaitingInput:
		return "the agent asks you a question", true
	case agent.StateBlocked:
		return "the agent waits on a permission decision", true
	default:
		return "", false
	}
}

func (s *Service) assess(ctx context.Context, w store.Watch, snap *snapshot.Snapshot, state agentStatus, newHead bool) error {
	now := s.now()
	blockers := s.blockers(w, snap, state)
	if _, busy := agentBusyWord(state.session.State); busy || state.limit != "" || len(blockers) > 0 {
		return s.store.SetWatchReadiness(ctx, w.ID, nil, blockers)
	}
	since := w.ReadySince
	if since == nil || newHead {
		since = &now
	}
	if err := s.store.SetWatchReadiness(ctx, w.ID, since, nil); err != nil {
		return err
	}
	if !settled(since, now, s.Interval()) {
		return nil
	}
	_, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityMergeReady, Ref: "ready@" + snap.PR.HeadSHA, At: now,
		Summary: readySummary(w, snap),
		Payload: mustJSON(map[string]any{"sha": snap.PR.HeadSHA, "approvals": snap.PR.Approvals, "checks": checks.CountPassed(w.CheckStates)}),
	})
	return err
}

func readySummary(w store.Watch, snap *snapshot.Snapshot) string {
	parts := []string{textx.Plural(snap.PR.Approvals, "approval")}
	if passed := checks.CountPassed(w.CheckStates); passed > 0 {
		parts = append(parts, textx.Plural(passed, "check")+" green")
	}
	return fmt.Sprintf("ready to merge: %s; %s", strings.Join(parts, ", "), mergeHint(w))
}

func mergeHint(w store.Watch) string {
	if w.MergeWhenReady {
		return "merge when ready merges it now"
	}
	return fmt.Sprintf("merge from the app or `babysitter watch merge %d`", w.ID)
}
