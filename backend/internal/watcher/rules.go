package watcher

import (
	"slices"
	"strings"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/store"
)

type ReviewState string

const (
	ReviewStateApproved         ReviewState = "APPROVED"
	ReviewStateChangesRequested ReviewState = "CHANGES_REQUESTED"
	ReviewStateCommented        ReviewState = "COMMENTED"
	ReviewStatePending          ReviewState = "PENDING"
)

func StateOf(r *github.PullRequestReview) ReviewState {
	return ReviewState(strings.ToUpper(r.GetState()))
}

var countingStates = []ReviewState{ReviewStateApproved, ReviewStateChangesRequested}

func (s ReviewState) Counts() bool { return slices.Contains(countingStates, s) }

func ReviewDecision(reviews []*github.PullRequestReview, author string, requestedReviewers int) (decision store.ReviewDecision, approvals, changesRequested int) {
	sorted := slices.Clone(reviews)
	slices.SortStableFunc(sorted, func(a, b *github.PullRequestReview) int {
		return a.GetSubmittedAt().Compare(b.GetSubmittedAt().Time)
	})

	latest := make(map[string]ReviewState)
	for _, r := range sorted {
		login := r.GetUser().GetLogin()
		if login == "" || strings.EqualFold(login, author) {
			continue
		}
		if state := StateOf(r); state.Counts() {
			latest[login] = state
		}
	}
	for _, state := range latest {
		switch state {
		case ReviewStateApproved:
			approvals++
		case ReviewStateChangesRequested:
			changesRequested++
		default:
		}
	}
	switch {
	case changesRequested > 0:
		decision = store.ReviewChangesRequested
	case approvals > 0:
		decision = store.ReviewApproved
	case requestedReviewers > 0:
		decision = store.ReviewRequired
	default:
		decision = store.ReviewNone
	}
	return decision, approvals, changesRequested
}
