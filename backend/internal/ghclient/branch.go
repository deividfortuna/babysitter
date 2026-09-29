package ghclient

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-github/v91/github"
)

const updateBranchMutation = `mutation($id: ID!, $head: GitObjectID!, $method: PullRequestBranchUpdateMethod!) {
  updatePullRequestBranch(input: {pullRequestId: $id, expectedHeadOid: $head, updateMethod: $method}) {
    pullRequest { headRefOid }
  }
}`

var ErrBranchNotUpdated = errors.New("github did not update the pull request branch")

type BranchRefusal struct {
	Reason string
}

func (r *BranchRefusal) Error() string { return ErrBranchNotUpdated.Error() + ": " + r.Reason }

func (r *BranchRefusal) Is(target error) bool { return target == ErrBranchNotUpdated }

func UpdatePullBranch(ctx context.Context, c *github.Client, nodeID, method, expectedHead string) (*github.Response, error) {
	var out struct {
		Errors graphqlErrors `json:"errors"`
	}
	resp, err := postGraphQL(ctx, c, updateBranchMutation, map[string]any{
		"id": nodeID, "head": expectedHead, "method": strings.ToUpper(method),
	}, &out)
	if err != nil {
		return resp, fmt.Errorf("update the branch of %s: %w", nodeID, err)
	}
	if failure := out.Errors.err(); failure != nil {
		if out.Errors.refusal() {
			return resp, &BranchRefusal{Reason: failure.Error()}
		}
		return resp, fmt.Errorf("update the branch of %s: %w", nodeID, failure)
	}
	return resp, nil
}
