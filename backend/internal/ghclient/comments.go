package ghclient

import (
	"context"
	"fmt"

	"github.com/google/go-github/v91/github"
)

func ReplyToReviewComment(ctx context.Context, c *github.Client, owner, repo string, number int, commentID int64, body string) (*github.PullRequestComment, *github.Response, error) {
	reply, resp, err := c.PullRequests.CreateCommentInReplyTo(ctx, owner, repo, number, body, commentID)
	if err != nil {
		return nil, resp, fmt.Errorf("reply to review comment %d of %s/%s#%d: %w", commentID, owner, repo, number, err)
	}
	return reply, resp, nil
}

func GetReviewComment(ctx context.Context, c *github.Client, owner, repo string, commentID int64) (*github.PullRequestComment, *github.Response, error) {
	comment, resp, err := c.PullRequests.GetComment(ctx, owner, repo, commentID)
	if err != nil {
		return nil, resp, fmt.Errorf("get review comment %d of %s/%s: %w", commentID, owner, repo, err)
	}
	return comment, resp, nil
}

func GetIssueComment(ctx context.Context, c *github.Client, owner, repo string, commentID int64) (*github.IssueComment, *github.Response, error) {
	comment, resp, err := c.Issues.GetComment(ctx, owner, repo, commentID)
	if err != nil {
		return nil, resp, fmt.Errorf("get comment %d of %s/%s: %w", commentID, owner, repo, err)
	}
	return comment, resp, nil
}

func CommentOnPull(ctx context.Context, c *github.Client, owner, repo string, number int, body string) (*github.IssueComment, *github.Response, error) {
	comment, resp, err := c.Issues.CreateComment(ctx, owner, repo, number, github.IssueCommentRequest{Body: body})
	if err != nil {
		return nil, resp, fmt.Errorf("comment on %s/%s#%d: %w", owner, repo, number, err)
	}
	return comment, resp, nil
}
