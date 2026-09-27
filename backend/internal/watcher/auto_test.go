package watcher

import (
	"context"
	"testing"

	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
)

func TestSyncKeepsTheAssigneesTheForkAndTheUpdateType(t *testing.T) {
	fx := newFixture(t)
	pr := fx.seedOpenPR()
	fx.update(pr, func(pr *ghfake.PR) {
		pr.Assignees = []string{"alice"}
		pr.HeadRepo = "teammate/r"
	})
	bump := fx.gh.PR("o/r", 2)
	fx.update(bump, func(pr *ghfake.PR) {
		pr.Author = "dependabot[bot]"
		pr.Title = "Bump the go group with 2 updates"
		pr.Body = "Updates `a` from 1.2.0 to 1.2.1\nUpdates `b` from 1.2.0 to 1.3.0\n"
	})
	ctx := context.Background()
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	prs, err := fx.store.OpenPRs(ctx, fx.repo.ID)
	if err != nil || len(prs) != 2 {
		t.Fatalf("OpenPRs() = %v, %v", prs, err)
	}
	mine, update := prs[0], prs[1]
	if !mine.AssignedTo("alice") || !mine.Fork || mine.UpdateType != "" {
		t.Errorf("#1 = assignees %v, fork %v, update %q, want alice, a fork and no update type", mine.Assignees, mine.Fork, mine.UpdateType)
	}
	if update.Fork || update.UpdateType != dependabot.Minor {
		t.Errorf("#2 = fork %v, update %q, want no fork and minor", update.Fork, update.UpdateType)
	}
}

func TestTheStepAfterThePassRunsAfterTheSync(t *testing.T) {
	fx := newFixture(t)
	fx.seedOpenPR()
	var seen []store.PullRequest
	WithAfterPass(func(ctx context.Context) {
		seen, _ = fx.store.OpenPRs(ctx, fx.repo.ID)
	})(fx.w)
	fx.w.runPass(context.Background())
	if len(seen) != 1 {
		t.Fatalf("the step after the pass saw %d pull requests, want the 1 of the sync", len(seen))
	}
}
