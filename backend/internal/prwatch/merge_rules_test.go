package prwatch

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestAWatchDropsTheApprovalItCopied(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)
	if got := fx.watch(w); !slices.Contains(got.ReadyBlockers, "no approval yet") {
		t.Fatalf("blockers = %v, want the approval the watch copied", got.ReadyBlockers)
	}

	got, err := fx.svc.SetMergeRules(ctx, w.ID, MergeRulesChange{ApprovalsRequired: ApprovalsOf(0)})
	if err != nil || got.ApprovalsRequired != 0 {
		t.Fatalf("SetMergeRules() = %+v, %v", got, err)
	}
	fx.poll(w)
	if got := fx.watch(w); got.ReadySince == nil || len(got.ReadyBlockers) != 0 {
		t.Fatalf("watch = since %v, blockers %v", got.ReadySince, got.ReadyBlockers)
	}
}

func TestAWatchTakesTheRuleOfTheBaseBranchAgain(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	req := fx.startRequest()
	req.ApprovalsRequired = ApprovalsOf(5)
	w, err := fx.svc.Start(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	fx.update(func() { fx.repo.Approvals["main"] = 2 })

	got, err := fx.svc.SetMergeRules(ctx, w.ID, MergeRulesChange{ApprovalsRequired: ApprovalsFromBranch()})
	if err != nil || got.ApprovalsRequired != 2 {
		t.Fatalf("SetMergeRules() = %+v, %v; want the 2 the base branch asks for", got, err)
	}
}

func TestAWatchChangesItsMergeMethod(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	req := fx.startRequest()
	req.ApprovalsRequired, req.MergeMethod = ApprovalsOf(3), new("squash")
	w, err := fx.svc.Start(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	got, err := fx.svc.SetMergeRules(ctx, w.ID, MergeRulesChange{MergeMethod: new("Rebase")})
	if err != nil || got.MergeMethod != "rebase" || got.ApprovalsRequired != 3 {
		t.Fatalf("SetMergeRules() = %+v, %v; want rebase and the 3 approvals it had", got, err)
	}
	got, err = fx.svc.SetMergeRules(ctx, w.ID, MergeRulesChange{MergeMethod: new("")})
	if err != nil || got.MergeMethod != "" {
		t.Fatalf("SetMergeRules() = %+v, %v; want the repository default", got, err)
	}
	if _, err := fx.svc.SetMergeRules(ctx, w.ID, MergeRulesChange{MergeMethod: new("fast-forward")}); !errors.Is(err, ErrBadMergeMethod) {
		t.Fatalf("SetMergeRules() error = %v, want ErrBadMergeMethod", err)
	}
	if _, err := fx.svc.SetMergeRules(ctx, w.ID, MergeRulesChange{ApprovalsRequired: ApprovalsOf(-1)}); !errors.Is(err, ErrBadApprovals) {
		t.Fatalf("SetMergeRules() error = %v, want ErrBadApprovals", err)
	}
	if got := fx.watch(w); got.MergeMethod != "" || got.ApprovalsRequired != 3 {
		t.Fatalf("a refused change wrote the watch: %+v", got)
	}
}

func TestAStoppedWatchKeepsItsMergeRules(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	if _, err := fx.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}

	if _, err := fx.svc.SetMergeRules(ctx, w.ID, MergeRulesChange{ApprovalsRequired: ApprovalsOf(0)}); !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("SetMergeRules() error = %v, want ErrWatchStopped", err)
	}
}

func TestASelfWatchChangesItsMergeRules(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startSelf()

	got, err := fx.svc.SetMergeRules(context.Background(), w.ID, MergeRulesChange{ApprovalsRequired: ApprovalsOf(2), MergeMethod: new("merge")})
	if err != nil || got.ApprovalsRequired != 2 || got.MergeMethod != "merge" {
		t.Fatalf("SetMergeRules() = %+v, %v", got, err)
	}
}
