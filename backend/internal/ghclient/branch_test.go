package ghclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func nodeIDOf(t *testing.T, c *github.Client, number int) string {
	t.Helper()
	pr, _, err := c.PullRequests.Get(context.Background(), "o", "r", number)
	if err != nil {
		t.Fatal(err)
	}
	if pr.GetNodeID() == "" {
		t.Fatal("the pull request has no node ID")
	}
	return pr.GetNodeID()
}

func TestUpdatePullBranchRebasesTheHeadItExpects(t *testing.T) {
	t.Parallel()
	gh := ghfake.New()
	p := gh.PR("o/r", 5)
	c := gh.Client(t)

	if _, err := UpdatePullBranch(context.Background(), c, nodeIDOf(t, c, 5), "rebase", "abc"); err != nil {
		t.Fatalf("UpdatePullBranch() error = %v", err)
	}
	var updates []ghfake.BranchUpdate
	var now string
	gh.Update(func() { updates, now = p.BranchUpdates, p.HeadSHA })
	if len(updates) != 1 || updates[0] != (ghfake.BranchUpdate{Method: "REBASE", ExpectedHead: "abc"}) {
		t.Fatalf("BranchUpdates = %+v, want one rebase that expects abc", updates)
	}
	if now == "abc" {
		t.Fatal("the head did not move")
	}
}

func TestUpdatePullBranchReportsTheRefusalOfGitHub(t *testing.T) {
	t.Parallel()
	gh := ghfake.New()
	p := gh.PR("o/r", 5)
	gh.Update(func() { p.RefuseBranchUpdate = "merge conflict between base and head" })
	c := gh.Client(t)

	_, err := UpdatePullBranch(context.Background(), c, nodeIDOf(t, c, 5), "merge", "abc")
	if !errors.Is(err, ErrBranchNotUpdated) || !strings.Contains(err.Error(), "merge conflict") {
		t.Fatalf("UpdatePullBranch() error = %v, want the refusal with the reason of GitHub", err)
	}
}

func TestUpdatePullBranchRefusesAHeadThatMoved(t *testing.T) {
	t.Parallel()
	gh := ghfake.New()
	gh.PR("o/r", 5)
	c := gh.Client(t)

	_, err := UpdatePullBranch(context.Background(), c, nodeIDOf(t, c, 5), "merge", "old")
	if !errors.Is(err, ErrBranchNotUpdated) {
		t.Fatalf("UpdatePullBranch() error = %v, want a refusal", err)
	}
}

func TestUpdatePullBranchTakesARateLimitForATransientFailure(t *testing.T) {
	t.Parallel()
	gh := ghfake.New()
	gh.PR("o/r", 5)
	c := gh.Client(t)
	id := nodeIDOf(t, c, 5)
	gh.React(ghfake.RouteGraphQL, func(ghfake.Action) (ghfake.Response, bool) {
		return ghfake.Response{Status: 200, Body: `{"data":null,"errors":[{"type":"RATE_LIMITED","message":"API rate limit exceeded"}]}`}, true
	})

	_, err := UpdatePullBranch(context.Background(), c, id, "rebase", "abc")
	if err == nil || errors.Is(err, ErrBranchNotUpdated) {
		t.Fatalf("UpdatePullBranch() error = %v, want a failure that is not a refusal", err)
	}
}

func TestUpdatePullBranchKeepsATransportFailureApartFromARefusal(t *testing.T) {
	t.Parallel()
	gh := ghfake.New()
	gh.PR("o/r", 5)
	c := gh.Client(t)
	id := nodeIDOf(t, c, 5)
	gh.Fail(ghfake.RouteGraphQL, 502, "Bad Gateway")

	_, err := UpdatePullBranch(context.Background(), c, id, "rebase", "abc")
	if err == nil || errors.Is(err, ErrBranchNotUpdated) {
		t.Fatalf("UpdatePullBranch() error = %v, want a failure that is not a refusal", err)
	}
}
