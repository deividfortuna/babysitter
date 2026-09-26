package ghclient

import (
	"context"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestGetReviewCommentAndIssueComment(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	pr := g.PR("o/r", 5)
	pr.ReviewComments = []ghfake.ReviewComment{{ID: 31, Body: "rename this"}}
	pr.IssueComments = []ghfake.Comment{{ID: 11, Body: "please add a test"}}
	c := g.Client(t)
	ctx := context.Background()

	review, _, err := GetReviewComment(ctx, c, "o", "r", 31)
	if err != nil || review.GetBody() != "rename this" {
		t.Fatalf("GetReviewComment() = %v, %v", review, err)
	}
	if _, _, err := GetReviewComment(ctx, c, "o", "r", 11); !IsNotFound(err) {
		t.Fatalf("GetReviewComment() of an issue comment error = %v, want not found", err)
	}
	issue, _, err := GetIssueComment(ctx, c, "o", "r", 11)
	if err != nil || issue.GetBody() != "please add a test" {
		t.Fatalf("GetIssueComment() = %v, %v", issue, err)
	}
	if _, _, err := GetIssueComment(ctx, c, "o", "r", 12); !IsNotFound(err) {
		t.Fatalf("GetIssueComment() of a missing comment error = %v, want not found", err)
	}
}
