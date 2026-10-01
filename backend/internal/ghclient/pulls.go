package ghclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-github/v91/github"
)

func ListOpenPulls(ctx context.Context, c *github.Client, owner, repo string) ([]*github.PullRequest, *github.Response, error) {
	opts := &github.PullRequestListOptions{
		State:   "open",
		PerPage: maxPerPage,
	}
	return paginate(func(page int) ([]*github.PullRequest, *github.Response, error) {
		opts.Page = page
		prs, resp, err := c.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return nil, resp, fmt.Errorf("list pull requests %s/%s: %w", owner, repo, err)
		}
		return prs, resp, nil
	})
}

func GetPull(ctx context.Context, c *github.Client, owner, repo string, number int) (*github.PullRequest, *github.Response, error) {
	pr, resp, err := c.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, resp, fmt.Errorf("get pull request %s/%s#%d: %w", owner, repo, number, err)
	}
	return pr, resp, nil
}

var ErrDiffTooLarge = errors.New("github serves no diff this large: read it with git diff in the checkout")

func PullDiff(ctx context.Context, c *github.Client, owner, repo string, number int) (string, *github.Response, error) {
	diff, resp, err := c.PullRequests.GetRaw(ctx, owner, repo, number, github.RawOptions{Type: github.Diff})
	if err == nil {
		return diff, resp, nil
	}
	tooLarge := resp != nil && resp.StatusCode == http.StatusNotAcceptable
	if tooLarge {
		return "", resp, fmt.Errorf("%w: %s/%s#%d: %w", ErrDiffTooLarge, owner, repo, number, err)
	}
	return "", resp, fmt.Errorf("get the diff of pull request %s/%s#%d: %w", owner, repo, number, err)
}

const comparePageWithoutFiles = 2

func BehindBy(ctx context.Context, c *github.Client, owner, repo, base, head string) (int, *github.Response, error) {
	opts := &github.ListOptions{PerPage: 1, Page: comparePageWithoutFiles}
	comparison, resp, err := c.Repositories.CompareCommits(ctx, owner, repo, base, head, opts)
	if err != nil {
		return 0, resp, fmt.Errorf("compare %s/%s %s...%s: %w", owner, repo, base, head, err)
	}
	return comparison.GetBehindBy(), resp, nil
}

func ListReviews(ctx context.Context, c *github.Client, owner, repo string, number int) ([]*github.PullRequestReview, *github.Response, error) {
	opts := &github.ListOptions{PerPage: maxPerPage}
	return paginate(func(page int) ([]*github.PullRequestReview, *github.Response, error) {
		opts.Page = page
		reviews, resp, err := c.PullRequests.ListReviews(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, resp, fmt.Errorf("list reviews %s/%s#%d: %w", owner, repo, number, err)
		}
		return reviews, resp, nil
	})
}

func ListCheckRuns(ctx context.Context, c *github.Client, owner, repo, ref string) ([]*github.CheckRun, *github.Response, error) {
	opts := &github.ListCheckRunsOptions{
		Filter:  new("latest"),
		PerPage: maxPerPage,
	}
	return paginate(func(page int) ([]*github.CheckRun, *github.Response, error) {
		opts.Page = page
		res, resp, err := c.Checks.ListCheckRunsForRef(ctx, owner, repo, ref, opts)
		if err != nil {
			return nil, resp, fmt.Errorf("list check runs %s/%s@%s: %w", owner, repo, ref, err)
		}
		return res.CheckRuns, resp, nil
	})
}

func GetCombinedStatus(ctx context.Context, c *github.Client, owner, repo, ref string) (*github.CombinedStatus, *github.Response, error) {
	opts := &github.ListOptions{PerPage: maxPerPage}
	status, resp, err := c.Repositories.GetCombinedStatus(ctx, owner, repo, ref, opts)
	if err != nil {
		return nil, resp, fmt.Errorf("get combined status %s/%s@%s: %w", owner, repo, ref, err)
	}
	return status, resp, nil
}

func GetRepo(ctx context.Context, c *github.Client, owner, repo string) (*github.Repository, error) {
	r, _, err := c.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("get repository %s/%s: %w", owner, repo, err)
	}
	return r, nil
}

func IsNotFound(err error) bool {
	return HasStatus(err, http.StatusNotFound)
}

func ListPullsByHead(ctx context.Context, c *github.Client, owner, repo, head, state string) ([]*github.PullRequest, *github.Response, error) {
	opts := &github.PullRequestListOptions{
		State:     state,
		Head:      head,
		Sort:      "created",
		Direction: "desc",
		PerPage:   maxPerPage,
	}
	prs, resp, err := c.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, resp, fmt.Errorf("list pull requests %s/%s with head %s: %w", owner, repo, head, err)
	}
	return prs, resp, nil
}

func ListIssueComments(ctx context.Context, c *github.Client, owner, repo string, number int) ([]*github.IssueComment, *github.Response, error) {
	opts := &github.IssueListCommentsOptions{PerPage: maxPerPage}
	return paginate(func(page int) ([]*github.IssueComment, *github.Response, error) {
		opts.Page = page
		comments, resp, err := c.Issues.ListComments(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, resp, fmt.Errorf("list comments %s/%s#%d: %w", owner, repo, number, err)
		}
		return comments, resp, nil
	})
}

func ListReviewComments(ctx context.Context, c *github.Client, owner, repo string, number int) ([]*github.PullRequestComment, *github.Response, error) {
	opts := &github.PullRequestListCommentsOptions{PerPage: maxPerPage}
	return paginate(func(page int) ([]*github.PullRequestComment, *github.Response, error) {
		opts.Page = page
		comments, resp, err := c.PullRequests.ListComments(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, resp, fmt.Errorf("list review comments %s/%s#%d: %w", owner, repo, number, err)
		}
		return comments, resp, nil
	})
}

const (
	MergeSquash = "squash"
	MergeMerge  = "merge"
	MergeRebase = "rebase"
)

var MergeMethodsKnown = []string{MergeSquash, MergeMerge, MergeRebase}

func MergeMethods(r *github.Repository) []string {
	var out []string
	if r.GetAllowSquashMerge() {
		out = append(out, MergeSquash)
	}
	if r.GetAllowMergeCommit() {
		out = append(out, MergeMerge)
	}
	if r.GetAllowRebaseMerge() {
		out = append(out, MergeRebase)
	}
	return out
}

var ErrNotMerged = errors.New("github did not merge the pull request")

func MergePull(ctx context.Context, c *github.Client, owner, repo string, number int, method, headSHA string) (*github.PullRequestMergeResult, *github.Response, error) {
	opts := &github.PullRequestOptions{MergeMethod: method, SHA: headSHA}
	res, resp, err := c.PullRequests.Merge(ctx, owner, repo, number, "", opts)
	if err != nil {
		return nil, resp, fmt.Errorf("merge pull request %s/%s#%d: %w", owner, repo, number, err)
	}
	if !res.GetMerged() {
		reason := res.GetMessage()
		if reason == "" {
			reason = "no reason given"
		}
		return res, resp, fmt.Errorf("merge pull request %s/%s#%d: %w: %s", owner, repo, number, ErrNotMerged, reason)
	}
	return res, resp, nil
}

func IsRefused(err error) bool {
	return errors.Is(err, ErrNotMerged) ||
		HasStatus(err, http.StatusMethodNotAllowed, http.StatusConflict, http.StatusUnprocessableEntity)
}

func IsNoReplyTarget(err error) bool {
	var ghErr *github.ErrorResponse
	if !HasStatus(err, http.StatusUnprocessableEntity) || !errors.As(err, &ghErr) {
		return false
	}
	for _, e := range ghErr.Errors {
		if e.Field == "in_reply_to" {
			return true
		}
	}
	return false
}

func ApprovePull(ctx context.Context, c *github.Client, owner, repo string, number int, headSHA, body string) (*github.PullRequestReview, *github.Response, error) {
	review, resp, err := c.PullRequests.CreateReview(ctx, owner, repo, number, &github.PullRequestReviewRequest{
		CommitID: &headSHA, Body: &body, Event: new("APPROVE"),
	})
	if err != nil {
		return nil, resp, fmt.Errorf("approve pull request %s/%s#%d: %w", owner, repo, number, err)
	}
	return review, resp, nil
}

func RequestReviewers(ctx context.Context, c *github.Client, owner, repo string, number int, logins []string) (*github.Response, error) {
	_, resp, err := c.PullRequests.RequestReviewers(ctx, owner, repo, number, github.ReviewersRequest{Reviewers: logins})
	if err != nil {
		return resp, fmt.Errorf("ask for a review of %s/%s#%d: %w", owner, repo, number, err)
	}
	return resp, nil
}
