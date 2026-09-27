package prwatch

import (
	"cmp"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

type State struct {
	HeadSHA        string
	PRState        store.PRState
	MergeableState store.MergeableState
	GreenSHA       string
	Checks         map[string]checks.State
}

func stateOf(w store.Watch) State {
	return State{
		HeadSHA:        w.HeadSHA,
		PRState:        w.PRState,
		MergeableState: w.MergeableState,
		GreenSHA:       w.GreenSHA,
		Checks:         w.CheckStates,
	}
}

func Diff(prev State, s *snapshot.Snapshot, now time.Time) ([]store.Activity, State) {
	next := State{
		HeadSHA:        s.PR.HeadSHA,
		PRState:        s.PR.State,
		MergeableState: prev.MergeableState,
		GreenSHA:       prev.GreenSHA,
		Checks:         map[string]checks.State{},
	}
	var out []store.Activity
	add := func(kind store.ActivityKind, ref, actor, summary, url string, payload any) {
		a := store.Activity{Kind: kind, Ref: ref, At: now, Actor: actor, Summary: summary, URL: url}
		if payload != nil {
			if b, err := json.Marshal(payload); err == nil {
				a.Payload = b
			}
		}
		out = append(out, a)
	}

	for _, it := range s.NewReviewItems {
		add(it.Kind.Activity(), fmt.Sprint(it.ID), it.Author, reviewSummary(it), it.URL, reviewPayload(it))
	}

	commit := prev.HeadSHA != "" && s.PR.HeadSHA != "" && s.PR.HeadSHA != prev.HeadSHA
	if commit {
		add(store.ActivityCommit, s.PR.HeadSHA, "", "new commit "+textx.ShortSHA(s.PR.HeadSHA)+" on "+s.PR.HeadBranch, s.PR.URL,
			map[string]any{"sha": s.PR.HeadSHA, "previous": prev.HeadSHA})
		next.GreenSHA = ""
	}
	prevChecks := prev.Checks
	if commit || prevChecks == nil {
		prevChecks = map[string]checks.State{}
	}

	sha := s.PR.HeadSHA
	for _, c := range s.Checks.Items {
		state := classifyCheck(c)
		next.Checks[c.Name] = state
		was := prevChecks[c.Name]
		switch {
		case state == checks.Failed && was != checks.Failed:
			add(store.ActivityCheckFailed, c.Name+"@"+sha, "", c.Name+" failed on "+textx.ShortSHA(sha), c.URL,
				withJob(map[string]any{"check": c.Name, "sha": sha, "conclusion": c.Conclusion, "source": c.Source}, s.FailedJobs, c.Name, 0))
		case state == checks.Passed && was == checks.Failed:
			add(store.ActivityCheckRecovered, c.Name+"@"+sha, "", c.Name+" recovered on "+textx.ShortSHA(sha), c.URL,
				map[string]any{"check": c.Name, "sha": sha})
		}
	}
	for _, r := range s.FailedRunsWithoutCheck() {
		name := r.WorkflowName
		if _, ok := next.Checks[name]; ok {
			continue
		}
		next.Checks[name] = checks.Failed
		if prevChecks[name] != checks.Failed {
			add(store.ActivityCheckFailed, name+"@"+sha, "", name+" failed on "+textx.ShortSHA(sha)+" ("+string(r.Conclusion)+")", r.HTMLURL,
				withJob(map[string]any{"check": name, "sha": sha, "conclusion": r.Conclusion, "run_id": r.RunID, "source": checks.SourceWorkflowRun}, s.FailedJobs, name, r.RunID))
		}
	}

	if sha != "" && next.GreenSHA != sha && allChecksPassed(s) {
		next.GreenSHA = sha
		passed := checks.CountPassed(next.Checks)
		add(store.ActivityChecksGreen, sha, "", fmt.Sprintf("all %s passed on %s", textx.Plural(passed, "check"), textx.ShortSHA(sha)), s.PR.URL,
			map[string]any{"sha": sha, "passed": passed})
	}

	if ms := s.PR.MergeableState; ms.Known() {
		next.MergeableState = ms
		if ms != prev.MergeableState {
			switch ms {
			case store.MergeableBehind:
				add(store.ActivityBehind, "behind@"+sha, "", s.PR.HeadBranch+" is behind "+s.PR.BaseBranch, s.PR.URL,
					map[string]any{"sha": sha, "base": s.PR.BaseBranch})
			case store.MergeableDirty:
				add(store.ActivityConflict, "conflict@"+sha, "", s.PR.HeadBranch+" conflicts with "+s.PR.BaseBranch, s.PR.URL,
					map[string]any{"sha": sha, "base": s.PR.BaseBranch})
			default:
			}
		}
	}

	switch {
	case s.PR.Merged:
		add(store.ActivityMerged, "merged", "", "pull request merged", s.PR.URL, map[string]any{"sha": sha})
	case s.PR.Closed:
		add(store.ActivityClosed, "closed", "", "pull request closed without merge", s.PR.URL, map[string]any{"sha": sha})
	}
	return out, next
}

func withJob(p map[string]any, jobs []snapshot.FailedJob, name string, runID int64) map[string]any {
	j, ok := jobOf(jobs, name, runID)
	if !ok {
		return p
	}
	p["run_id"], p["job_id"], p["job_name"], p["logs_endpoint"] = j.RunID, j.JobID, j.JobName, j.LogsEndpoint
	return p
}

func allChecksPassed(s *snapshot.Snapshot) bool {
	return s.Checks.Status == checks.CISuccess && s.Checks.AllTerminal && s.Checks.FailedCount == 0 && len(s.FailedRuns) == 0
}

func classifyCheck(c snapshot.Check) checks.State {
	if c.Source == checks.SourceStatus {
		return checks.ClassifyLegacy(checks.LegacyState(c.Status))
	}
	return checks.Classify(checks.RunStatus(c.Status), checks.Conclusion(c.Conclusion))
}

func reviewSummary(it snapshot.ReviewItem) string {
	body := firstLine(it.Body)
	switch it.Kind {
	case store.KindReviewComment:
		where := it.Path
		if it.Line != nil {
			where = fmt.Sprintf("%s:%d", it.Path, *it.Line)
		}
		return fmt.Sprintf("%s commented on %s: %s", it.Author, where, body)
	case store.KindReview:
		state := cmp.Or(strings.ToLower(strings.ReplaceAll(string(it.State), "_", " ")), "reviewed")
		if body == "" {
			return fmt.Sprintf("%s %s", it.Author, state)
		}
		return fmt.Sprintf("%s %s: %s", it.Author, state, body)
	default:
		return fmt.Sprintf("%s commented: %s", it.Author, body)
	}
}

func reviewPayload(it snapshot.ReviewItem) map[string]any {
	p := map[string]any{
		"item_id":            it.ID,
		"kind":               it.Kind,
		"author":             it.Author,
		"author_association": it.AuthorAssociation,
		"body":               it.Body,
		"created_at":         it.CreatedAt,
	}
	if it.Path != "" {
		p["path"] = it.Path
	}
	if it.Line != nil {
		p["line"] = *it.Line
	}
	if it.Side != "" {
		p["side"] = it.Side
	}
	if it.CommitID != "" {
		p["commit_id"] = it.CommitID
	}
	if it.State != "" {
		p["state"] = it.State
	}
	return p
}

const summaryWidth = 120

func firstLine(s string) string {
	return textx.FirstLine(s, summaryWidth)
}
