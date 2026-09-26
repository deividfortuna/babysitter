package ghclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v91/github"
)

var ErrRulesUnreadable = errors.New("the rules of the branch are not readable with this token")

func RequiredApprovals(ctx context.Context, c *github.Client, owner, repo, branch string) (int, *github.Response, error) {
	fromRules, rulesRead, resp, err := rulesetApprovals(ctx, c, owner, repo, branch)
	if err != nil {
		return 0, resp, err
	}
	fromProtection, protectionRead, resp2, err := protectionApprovals(ctx, c, owner, repo, branch)
	if err != nil {
		return 0, resp2, err
	}
	if resp2 != nil {
		resp = resp2
	}
	n := max(fromRules, fromProtection)
	bothRead := rulesRead && protectionRead
	unknown := n == 0 && !bothRead
	if unknown {
		return 0, resp, fmt.Errorf("rules of %s/%s branch %s: %w", owner, repo, branch, ErrRulesUnreadable)
	}
	return n, resp, nil
}

func rulesetApprovals(ctx context.Context, c *github.Client, owner, repo, branch string) (int, bool, *github.Response, error) {
	rules, resp, err := c.Repositories.ListRulesForBranch(ctx, owner, repo, branch, nil)
	if noRules(err) {
		return 0, true, resp, nil
	}
	if isForbidden(err) {
		return 0, false, resp, nil
	}
	if err != nil {
		return 0, false, resp, fmt.Errorf("rules of %s/%s branch %s: %w", owner, repo, branch, err)
	}
	n := 0
	for _, r := range rules.PullRequest {
		n = max(n, r.Parameters.RequiredApprovingReviewCount)
	}
	return n, true, resp, nil
}

func noRules(err error) bool {
	return IsNotFound(err) || isPlanRestricted(err)
}

func protectionApprovals(ctx context.Context, c *github.Client, owner, repo, branch string) (int, bool, *github.Response, error) {
	p, resp, err := c.Repositories.GetBranchProtection(ctx, owner, repo, branch)
	if unprotected(err) {
		return 0, true, resp, nil
	}
	if isForbidden(err) {
		return 0, false, resp, nil
	}
	if err != nil {
		return 0, false, resp, fmt.Errorf("protection of %s/%s branch %s: %w", owner, repo, branch, err)
	}
	return p.GetRequiredPullRequestReviews().RequiredApprovingReviewCount, true, resp, nil
}

func unprotected(err error) bool {
	return errors.Is(err, github.ErrBranchNotProtected) || noRules(err)
}

func isForbidden(err error) bool {
	var ghErr *github.ErrorResponse
	return errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusForbidden
}

func isPlanRestricted(err error) bool {
	var ghErr *github.ErrorResponse
	if !isForbidden(err) || !errors.As(err, &ghErr) {
		return false
	}
	return strings.Contains(strings.ToLower(ghErr.Message), "upgrade to github")
}
