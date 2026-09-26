package watcher

import (
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/store"
)

func review(login, state string, at int) *github.PullRequestReview {
	return &github.PullRequestReview{
		User:        &github.User{Login: new(login)},
		State:       new(state),
		SubmittedAt: &github.Timestamp{Time: time.Date(2026, 9, 7, 0, 0, at, 0, time.UTC)},
	}
}

func TestReviewDecision(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		reviews   []*github.PullRequestReview
		requested int
		want      store.ReviewDecision
		approvals int
		changes   int
	}{
		{name: "empty", want: store.ReviewNone},
		{name: "requested only", requested: 2, want: store.ReviewRequired},
		{name: "approved", reviews: []*github.PullRequestReview{review("bob", "APPROVED", 1)}, want: store.ReviewApproved, approvals: 1},
		{name: "author self approval ignored", reviews: []*github.PullRequestReview{review("alice", "APPROVED", 1)}, want: store.ReviewNone},
		{name: "changes then approve by same user", reviews: []*github.PullRequestReview{
			review("bob", "CHANGES_REQUESTED", 1), review("bob", "APPROVED", 2),
		}, want: store.ReviewApproved, approvals: 1},
		{name: "approve then changes by same user, out of order", reviews: []*github.PullRequestReview{
			review("bob", "CHANGES_REQUESTED", 2), review("bob", "APPROVED", 1),
		}, want: store.ReviewChangesRequested, changes: 1},
		{name: "changes wins over approval", reviews: []*github.PullRequestReview{
			review("bob", "APPROVED", 1), review("carol", "CHANGES_REQUESTED", 2),
		}, want: store.ReviewChangesRequested, approvals: 1, changes: 1},
		{name: "dismissed does not count", reviews: []*github.PullRequestReview{review("bob", "DISMISSED", 1)}, requested: 1, want: store.ReviewRequired},
		{name: "comment does not count", reviews: []*github.PullRequestReview{review("bob", "COMMENTED", 1)}, want: store.ReviewNone},
		{name: "approval satisfies pending request", reviews: []*github.PullRequestReview{review("bob", "APPROVED", 1)}, requested: 1, want: store.ReviewApproved, approvals: 1},
		{name: "later comment keeps the approval", reviews: []*github.PullRequestReview{
			review("bob", "APPROVED", 1), review("bob", "COMMENTED", 2),
		}, want: store.ReviewApproved, approvals: 1},
		{name: "later comment keeps the change request", reviews: []*github.PullRequestReview{
			review("bob", "CHANGES_REQUESTED", 1), review("bob", "COMMENTED", 2),
		}, want: store.ReviewChangesRequested, changes: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, approvals, changes := ReviewDecision(tc.reviews, "alice", tc.requested)
			if got != tc.want || approvals != tc.approvals || changes != tc.changes {
				t.Fatalf("got (%s, %d, %d), want (%s, %d, %d)", got, approvals, changes, tc.want, tc.approvals, tc.changes)
			}
		})
	}
}
