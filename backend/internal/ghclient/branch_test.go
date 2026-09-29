package ghclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestUpdatePullBranchRebasesTheHeadItExpects(t *testing.T) {
	t.Parallel()
	gh := ghfake.New()
	p := gh.PR("o/r", 5)
	c := gh.Client(t)

	head, _, err := UpdatePullBranch(context.Background(), c, "o", "r", 5, "rebase", "abc")
	if err != nil {
		t.Fatalf("UpdatePullBranch() error = %v", err)
	}
	var updates []ghfake.BranchUpdate
	var now string
	gh.Update(func() { updates, now = p.BranchUpdates, p.HeadSHA })
	if len(updates) != 1 || updates[0] != (ghfake.BranchUpdate{Method: "REBASE", ExpectedHead: "abc"}) {
		t.Fatalf("BranchUpdates = %+v, want one rebase that expects abc", updates)
	}
	if head == "" || head != now {
		t.Fatalf("UpdatePullBranch() = %q, want the new head %q", head, now)
	}
}

func TestUpdatePullBranchReportsTheRefusalOfGitHub(t *testing.T) {
	t.Parallel()
	gh := ghfake.New()
	p := gh.PR("o/r", 5)
	gh.Update(func() { p.RefuseBranchUpdate = "merge conflict between base and head" })
	c := gh.Client(t)

	_, _, err := UpdatePullBranch(context.Background(), c, "o", "r", 5, "merge", "abc")
	if !errors.Is(err, ErrBranchNotUpdated) || !strings.Contains(err.Error(), "merge conflict") {
		t.Fatalf("UpdatePullBranch() error = %v, want the refusal with the reason of GitHub", err)
	}
}

func TestUpdatePullBranchRefusesAHeadThatMoved(t *testing.T) {
	t.Parallel()
	gh := ghfake.New()
	gh.PR("o/r", 5)
	c := gh.Client(t)

	_, _, err := UpdatePullBranch(context.Background(), c, "o", "r", 5, "merge", "old")
	if !errors.Is(err, ErrBranchNotUpdated) {
		t.Fatalf("UpdatePullBranch() error = %v, want a refusal", err)
	}
}
