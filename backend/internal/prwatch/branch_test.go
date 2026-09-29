package prwatch

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (fx *fixture) behind() {
	fx.update(func() { fx.pr.MergeableState = "behind" })
}

func (fx *fixture) branchUpdates() []ghfake.BranchUpdate {
	var out []ghfake.BranchUpdate
	fx.update(func() { out = append(out, fx.pr.BranchUpdates...) })
	return out
}

func (fx *fixture) startWith(edit func(*StartRequest)) store.Watch {
	fx.t.Helper()
	req := fx.startRequest()
	edit(&req)
	w, err := fx.svc.Start(context.Background(), req)
	if err != nil {
		fx.t.Fatalf("Start() error = %v", err)
	}
	return w
}

func (fx *fixture) activityOf(w store.Watch, kind store.ActivityKind) store.Activity {
	fx.t.Helper()
	for _, a := range fx.activity(w) {
		if a.Kind == kind {
			return a
		}
	}
	fx.t.Fatalf("no %s activity in %v", kind, fx.kinds(w))
	return store.Activity{}
}

func TestABranchBehindItsBaseIsRebasedOnGitHubAndTheAgentHearsNothing(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	fx.agentIdle(w)

	fx.behind()
	fx.poll(w)
	fx.poll(w)

	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged", "behind", "branch_updated", "commit", "checks_green"})
	if got := fx.branchUpdates(); !slices.Equal(got, []ghfake.BranchUpdate{{Method: "REBASE", ExpectedHead: "abc"}}) {
		t.Fatalf("branch updates = %+v, want one rebase that expects abc", got)
	}
	if a := fx.activityOf(w, store.ActivityBranchUpdated); !strings.Contains(a.Summary, "GitHub rebased fix onto main") {
		t.Fatalf("summary = %q", a.Summary)
	}
	if msgs := h.messages(); len(msgs) != 1 {
		t.Fatalf("messages = %q, want only the opening message", msgs)
	}
}

func TestAWatchThatMergesTheBaseAsksGitHubForAMerge(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startWith(func(r *StartRequest) { r.BranchUpdate = new(store.BranchMerge) })
	fx.agentIdle(w)

	fx.behind()
	fx.poll(w)

	if got := fx.branchUpdates(); !slices.Equal(got, []ghfake.BranchUpdate{{Method: "MERGE", ExpectedHead: "abc"}}) {
		t.Fatalf("branch updates = %+v, want one merge that expects abc", got)
	}
	if a := fx.activityOf(w, store.ActivityBranchUpdated); !strings.Contains(a.Summary, "GitHub merged main into fix") {
		t.Fatalf("summary = %q", a.Summary)
	}
}

func TestABranchBehindAtTheStartIsUpdatedOnGitHub(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.behind()

	fx.start()

	if got := fx.branchUpdates(); len(got) != 1 {
		t.Fatalf("branch updates = %+v, want one", got)
	}
	if msgs := fx.host.last().messages(); len(msgs) != 1 {
		t.Fatalf("messages = %q, want only the opening message", msgs)
	}
}

func TestAnUpdateGitHubRefusesGoesToTheAgentOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	fx.update(func() { fx.pr.RefuseBranchUpdate = "merge conflict between base and head" })

	fx.behind()
	fx.poll(w)
	fx.agentIdle(w)
	fx.poll(w)

	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged", "behind", "branch_update_failed", "nudged"})
	if got := fx.branchUpdates(); len(got) != 1 {
		t.Fatalf("branch updates = %+v, want one attempt for the head", got)
	}
	if a := fx.activityOf(w, store.ActivityBranchNotUpdated); !strings.Contains(a.Summary, "merge conflict between base and head") {
		t.Fatalf("summary = %q, want the reason of GitHub", a.Summary)
	}
	msgs := h.messages()
	if len(msgs) != 2 || !strings.Contains(msgs[1], "is behind main") || !strings.Contains(msgs[1], "rebase onto it") {
		t.Fatalf("messages = %q, want the agent to rebase", msgs)
	}
}

func TestTheAgentMergesTheBaseWhenTheWatchSaysMerge(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startWith(func(r *StartRequest) { r.BranchUpdate, r.UpdateOnGitHub = new(store.BranchMerge), new(false) })
	h := fx.host.last()

	fx.behind()
	fx.poll(w)

	if got := fx.branchUpdates(); len(got) != 0 {
		t.Fatalf("branch updates = %+v, want none with GitHub off", got)
	}
	msgs := h.messages()
	if len(msgs) != 2 || !strings.Contains(msgs[1], "merge it into the branch") || !strings.Contains(msgs[1], "Do not rebase") || strings.Contains(msgs[1], "rebase onto it") {
		t.Fatalf("messages = %q, want the agent to merge", msgs)
	}
}

func TestTheAgentMergesTheBaseToSolveAConflictWhenTheWatchSaysMerge(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startWith(func(r *StartRequest) { r.BranchUpdate = new(store.BranchMerge) })
	h := fx.host.last()

	fx.update(func() { fx.pr.MergeableState = "dirty" })
	fx.poll(w)

	if got := fx.branchUpdates(); len(got) != 0 {
		t.Fatalf("branch updates = %+v, want none: GitHub cannot solve a conflict", got)
	}
	msgs := h.messages()
	if len(msgs) != 2 || !strings.Contains(msgs[1], "merge conflicts") || !strings.Contains(msgs[1], "merge it into the branch") {
		t.Fatalf("messages = %q, want the agent to merge", msgs)
	}
}

func TestADependabotBranchIsNotUpdatedOnGitHub(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.Author = "dependabot[bot]" })
	w := fx.start()
	fx.agentIdle(w)

	fx.behind()
	fx.poll(w)

	if got := fx.branchUpdates(); len(got) != 0 {
		t.Fatalf("branch updates = %+v, want none: Dependabot owns the branch", got)
	}
}

func TestASelfWatchIsNotUpdatedOnGitHub(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.checkout("fix")
	w := fx.startSelf()

	fx.behind()
	fx.poll(w)

	if got := fx.branchUpdates(); len(got) != 0 {
		t.Fatalf("branch updates = %+v, want none: the session of the author pushes the branch", got)
	}
}

func TestTheWorkBranchFollowsTheBranchGitHubRebased(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.agentIdle(w)
	fx.behind()
	fx.poll(w)
	head := fx.activityOf(w, store.ActivityBranchUpdated)
	var to struct {
		To string `json:"to"`
	}
	if err := json.Unmarshal(head.Payload, &to); err != nil {
		t.Fatal(err)
	}
	fx.rel.set(func(f *fakeRelease) { f.remote = to.To })
	fx.agentIdle(w)

	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:05:00Z"), Body: "one more thing", URL: "https://c/11"}}
	})
	fx.poll(w)

	if work, _ := fx.rel.Head(context.Background(), ""); work != to.To {
		t.Fatalf("work branch = %s, want it on %s, where GitHub put the pull request branch", work, to.To)
	}
}

func TestGitHubWaitsWhileTheAgentWorks(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()

	fx.behind()
	fx.poll(w)

	if got := fx.branchUpdates(); len(got) != 0 {
		t.Fatalf("branch updates = %+v, want none while the agent works", got)
	}
	if msgs := h.messages(); len(msgs) != 1 {
		t.Fatalf("messages = %q, want only the opening message: the branch waits for GitHub", msgs)
	}

	fx.agentIdle(w)
	fx.poll(w)

	if got := fx.branchUpdates(); len(got) != 1 {
		t.Fatalf("branch updates = %+v, want one once the agent is idle", got)
	}
	if msgs := h.messages(); len(msgs) != 1 {
		t.Fatalf("messages = %q, want no message about the branch", msgs)
	}
}

func TestGitHubWaitsUntilTheProposalIsDecided(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.propose(w)

	fx.behind()
	fx.poll(w)

	if got := fx.branchUpdates(); len(got) != 0 {
		t.Fatalf("branch updates = %+v, want none while proposal 1 waits", got)
	}
	if p := fx.proposal(w, 1); p.Status != store.ProposalPending {
		t.Fatalf("proposal = %+v, want it still pending", p)
	}

	if _, err := fx.svc.Approve(context.Background(), w.ID, 1, Decision{}); err != nil {
		t.Fatal(err)
	}
	fx.update(func() { fx.pr.HeadSHA = "w1" })
	fx.agentIdle(w)
	fx.poll(w)

	if got := fx.branchUpdates(); !slices.Equal(got, []ghfake.BranchUpdate{{Method: "REBASE", ExpectedHead: "w1"}}) {
		t.Fatalf("branch updates = %+v, want one rebase of the head that the proposal pushed", got)
	}
}

func TestAWatchChangesHowItUpdatesItsBranch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()

	got, err := fx.svc.SetMergeRules(context.Background(), w.ID, MergeRulesChange{BranchUpdate: new(store.BranchUpdate("Merge")), UpdateOnGitHub: new(false)})
	if err != nil || got.BranchUpdate != store.BranchMerge || got.UpdateOnGitHub {
		t.Fatalf("SetMergeRules() = %+v, %v, want a merge by the agent", got, err)
	}
	if _, err := fx.svc.SetMergeRules(context.Background(), w.ID, MergeRulesChange{BranchUpdate: new(store.BranchUpdate("squash"))}); !errors.Is(err, ErrBadBranchUpdate) {
		t.Fatalf("SetMergeRules(squash) error = %v, want ErrBadBranchUpdate", err)
	}
}

func TestStartTakesTheBranchUpdateOfTheChain(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.settings(store.Settings{BranchUpdate: store.BranchMerge, UpdateOnGitHub: false})

	if w := fx.start(); w.BranchUpdate != store.BranchMerge || w.UpdateOnGitHub {
		t.Fatalf("watch = %+v, want the settings", w)
	}

	fx.repoOverrides(store.WatchOverrides{BranchUpdate: store.BranchRebase, UpdateOnGitHub: new(true)})
	fx.openPR(4)
	w := fx.startWith(func(r *StartRequest) { r.Target.Number = 4 })
	if w.BranchUpdate != store.BranchRebase || !w.UpdateOnGitHub {
		t.Fatalf("watch = %+v, want the repository", w)
	}

	fx.openPR(5)
	w = fx.startWith(func(r *StartRequest) {
		r.Target.Number = 5
		r.BranchUpdate, r.UpdateOnGitHub = new(store.BranchMerge), new(false)
	})
	if w.BranchUpdate != store.BranchMerge || w.UpdateOnGitHub {
		t.Fatalf("watch = %+v, want the request", w)
	}
}

func TestStartRefusesAnUnknownBranchUpdate(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	_, err := fx.svc.Start(context.Background(), StartRequest{
		Target: fx.startRequest().Target, SourceDir: fx.dir, BranchUpdate: new(store.BranchUpdate("squash")),
	})
	if !errors.Is(err, ErrBadBranchUpdate) {
		t.Fatalf("Start() error = %v, want ErrBadBranchUpdate", err)
	}
}
