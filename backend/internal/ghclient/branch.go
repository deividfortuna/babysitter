package ghclient

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-github/v91/github"
)

const pullRequestIDQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) { pullRequest(number: $number) { id } }
}`

const updateBranchMutation = `mutation($id: ID!, $head: GitObjectID!, $method: PullRequestBranchUpdateMethod!) {
  updatePullRequestBranch(input: {pullRequestId: $id, expectedHeadOid: $head, updateMethod: $method}) {
    pullRequest { headRefOid }
  }
}`

var ErrBranchNotUpdated = errors.New("github did not update the pull request branch")

type graphqlErrors []struct {
	Message string `json:"message"`
}

func (e graphqlErrors) err() error {
	if len(e) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(e))
	for _, m := range e {
		msgs = append(msgs, m.Message)
	}
	return errors.New(strings.Join(msgs, "; "))
}

func UpdatePullBranch(ctx context.Context, c *github.Client, owner, repo string, number int, method, expectedHead string) (string, *github.Response, error) {
	id, resp, err := pullRequestID(ctx, c, owner, repo, number)
	if err != nil {
		return "", resp, err
	}
	var out struct {
		Data struct {
			UpdatePullRequestBranch struct {
				PullRequest struct {
					HeadRefOid string `json:"headRefOid"`
				} `json:"pullRequest"`
			} `json:"updatePullRequestBranch"`
		} `json:"data"`
		Errors graphqlErrors `json:"errors"`
	}
	resp, err = postGraphQL(ctx, c, updateBranchMutation, map[string]any{
		"id": id, "head": expectedHead, "method": strings.ToUpper(method),
	}, &out)
	if err != nil {
		return "", resp, fmt.Errorf("update the branch of %s/%s#%d: %w", owner, repo, number, err)
	}
	if refusal := out.Errors.err(); refusal != nil {
		return "", resp, fmt.Errorf("update the branch of %s/%s#%d: %w: %w", owner, repo, number, ErrBranchNotUpdated, refusal)
	}
	return out.Data.UpdatePullRequestBranch.PullRequest.HeadRefOid, resp, nil
}

func pullRequestID(ctx context.Context, c *github.Client, owner, repo string, number int) (string, *github.Response, error) {
	var out struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ID string `json:"id"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
		Errors graphqlErrors `json:"errors"`
	}
	resp, err := postGraphQL(ctx, c, pullRequestIDQuery, map[string]any{"owner": owner, "name": repo, "number": number}, &out)
	if err != nil {
		return "", resp, fmt.Errorf("read the node of %s/%s#%d: %w", owner, repo, number, err)
	}
	if missing := out.Errors.err(); missing != nil {
		return "", resp, fmt.Errorf("read the node of %s/%s#%d: %w", owner, repo, number, missing)
	}
	return out.Data.Repository.PullRequest.ID, resp, nil
}

func postGraphQL(ctx context.Context, c *github.Client, query string, vars map[string]any, out any) (*github.Response, error) {
	req, err := c.NewRequest(ctx, "POST", "graphql", graphqlRequest{Query: query, Variables: vars})
	if err != nil {
		return nil, err
	}
	return c.Do(req, out)
}
