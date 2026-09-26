package ghclient

import (
	"context"
	"fmt"

	"github.com/google/go-github/v91/github"
)

func ListWorkflowRuns(ctx context.Context, c *github.Client, owner, repo, headSHA string) ([]*github.WorkflowRun, *github.Response, error) {
	opts := &github.ListWorkflowRunsOptions{
		HeadSHA: headSHA,
		PerPage: maxPerPage,
	}
	return paginate(func(page int) ([]*github.WorkflowRun, *github.Response, error) {
		opts.Page = page
		res, resp, err := c.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
		if err != nil {
			return nil, resp, fmt.Errorf("list workflow runs %s/%s@%s: %w", owner, repo, headSHA, err)
		}
		return res.WorkflowRuns, resp, nil
	})
}

func ListWorkflowJobs(ctx context.Context, c *github.Client, owner, repo string, runID int64) ([]*github.WorkflowJob, *github.Response, error) {
	opts := &github.ListWorkflowJobsOptions{
		Filter:  "latest",
		PerPage: maxPerPage,
	}
	return paginate(func(page int) ([]*github.WorkflowJob, *github.Response, error) {
		opts.Page = page
		res, resp, err := c.Actions.ListWorkflowJobs(ctx, owner, repo, runID, opts)
		if err != nil {
			return nil, resp, fmt.Errorf("list jobs of run %s/%s/%d: %w", owner, repo, runID, err)
		}
		return res.Jobs, resp, nil
	})
}
