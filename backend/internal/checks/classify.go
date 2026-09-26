package checks

import "github.com/google/go-github/v91/github"

func Classify(status RunStatus, conclusion Conclusion) State {
	switch {
	case status != StatusCompleted, conclusion == ConclusionActionRequired:
		return Pending
	case conclusion.Failed():
		return Failed
	case conclusion == ConclusionSuccess:
		return Passed
	default:
		return Skipped
	}
}

func ClassifyLegacy(state LegacyState) State {
	switch state {
	case LegacyPending:
		return Pending
	case LegacyFailure, LegacyError:
		return Failed
	case LegacySuccess:
		return Passed
	default:
		return Skipped
	}
}

func ClassifyCheckRun(r *github.CheckRun) State {
	return Classify(RunStatus(r.GetStatus()), Conclusion(r.GetConclusion()))
}

func ClassifyStatus(s *github.RepoStatus) State {
	return ClassifyLegacy(LegacyState(s.GetState()))
}

func Overall(runs []*github.CheckRun, combined *github.CombinedStatus) CIStatus {
	var failure, pending, reported bool
	note := func(state State) {
		reported = true
		switch state {
		case Failed:
			failure = true
		case Pending:
			pending = true
		default:
		}
	}
	for _, r := range runs {
		note(ClassifyCheckRun(r))
	}
	if combined != nil {
		for _, s := range combined.Statuses {
			note(ClassifyStatus(s))
		}
	}
	switch {
	case failure:
		return CIFailure
	case pending:
		return CIPending
	case !reported:
		return CINone
	default:
		return CISuccess
	}
}
