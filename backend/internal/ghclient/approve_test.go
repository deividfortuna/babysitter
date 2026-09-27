package ghclient

import (
	"context"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestApprovePullSubmitsAnApprovalOnTheHead(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	pr := g.PR("o/r", 5)
	c := g.Client(t)

	review, _, err := ApprovePull(context.Background(), c, "o", "r", 5, "abc", "the policy approved it")
	if err != nil {
		t.Fatalf("ApprovePull() error = %v", err)
	}
	if review.GetState() != "APPROVED" {
		t.Fatalf("review state = %q, want APPROVED", review.GetState())
	}
	g.Update(func() {
		if len(pr.Reviews) != 1 || pr.Reviews[0].CommitID != "abc" || pr.Reviews[0].Body != "the policy approved it" || pr.Reviews[0].Author != "alice" {
			t.Errorf("reviews = %+v, want one approval by alice on abc", pr.Reviews)
		}
	})
}
