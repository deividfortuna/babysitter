package prwatch

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/gitrelease"
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
	if a := fx.activityOf(w, store.ActivityBranchUpdated); !strings.Contains(a.Summary, "GitHub accepted the request to rebase fix onto main at abc") {
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
	if a := fx.activityOf(w, store.ActivityBranchUpdated); !strings.Contains(a.Summary, "GitHub accepted the request to merge main into fix at abc") {
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
	var rebased string
	fx.update(func() { rebased = fx.pr.HeadSHA })
	fx.rel.set(func(f *fakeRelease) { f.history[rebased] = []string{"base"}; f.remote = rebased })
	fx.agentIdle(w)

	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:05:00Z"), Body: "one more thing", URL: "https://c/11"}}
	})
	fx.poll(w)

	if work, _ := fx.rel.Head(context.Background(), ""); work != rebased {
		t.Fatalf("work branch = %s, want it on %s, where GitHub put the pull request branch", work, rebased)
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

func TestAFailedProposalLetsTheAgentUpdateTheBranch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.rel.set(func(f *fakeRelease) { f.pushErr = errors.New("no credential") })
	fx.hook(w, agent.EventStop, `{}`)
	fx.poll(w)
	if p := fx.proposal(w, 1); p.Status != store.ProposalFailed {
		t.Fatalf("proposal = %+v, want it failed", p)
	}

	fx.behind()
	fx.poll(w)

	if got := fx.branchUpdates(); len(got) != 0 {
		t.Fatalf("branch updates = %+v, want none: a rebase on GitHub would fail the retry too", got)
	}
	msgs := h.messages()
	if last := msgs[len(msgs)-1]; !strings.Contains(last, "is behind main") {
		t.Fatalf("last message = %q, want the agent told about the branch", last)
	}
}

func TestATransientFailureIsTriedThreeTimesBeforeTheAgentTakesOver(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	fx.agentIdle(w)
	fx.api.React(ghfake.RouteGraphQL, func(a ghfake.Action) (ghfake.Response, bool) {
		return ghfake.Response{Status: http.StatusBadGateway, Message: "Bad Gateway"}, strings.Contains(string(a.Body), "updatePullRequestBranch")
	})

	fx.behind()
	fx.poll(w)
	fx.poll(w)
	if slices.Contains(fx.kinds(w), string(store.ActivityBranchNotUpdated)) || len(h.messages()) != 1 {
		t.Fatalf("kinds = %v, messages = %d, want no refusal and no message after two transient failures", fx.kinds(w), len(h.messages()))
	}

	fx.poll(w)
	if !slices.Contains(fx.kinds(w), string(store.ActivityBranchNotUpdated)) {
		t.Fatalf("kinds = %v, want the refusal after the third failure", fx.kinds(w))
	}
	msgs := h.messages()
	if len(msgs) != 2 || !strings.Contains(msgs[1], "is behind main") {
		t.Fatalf("messages = %q, want the agent told after the third failure", msgs)
	}
}

func TestAHeadThatIsStillBehindAfterTheUpdateGetsItsOwnTry(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.agentIdle(w)
	fx.behind()
	fx.poll(w)
	var first string
	fx.update(func() { first = fx.pr.HeadSHA })

	fx.poll(w)
	if got := fx.branchUpdates(); len(got) != 1 {
		t.Fatalf("branch updates = %+v, want one while GitHub computes the state of %s", got, first)
	}

	fx.behind()
	fx.poll(w)
	got := fx.branchUpdates()
	if len(got) != 2 || got[1].ExpectedHead != first {
		t.Fatalf("branch updates = %+v, want a second update that expects %s", got, first)
	}
}

func TestAnUpdateThatGitHubAcceptedButNeverAppliedGoesToTheAgent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	fx.agentIdle(w)
	fx.api.React(ghfake.RouteGraphQL, func(a ghfake.Action) (ghfake.Response, bool) {
		body := `{"data":{"updatePullRequestBranch":{"pullRequest":{"headRefOid":"abc"}}}}`
		return ghfake.Response{Status: http.StatusOK, Body: body}, strings.Contains(string(a.Body), "updatePullRequestBranch")
	})

	fx.behind()
	fx.poll(w)
	fx.poll(w)
	if kinds := fx.kinds(w); !slices.Contains(kinds, string(store.ActivityBranchUpdated)) || len(h.messages()) != 1 {
		t.Fatalf("kinds = %v, messages = %d, want the update accepted and no message while GitHub works", kinds, len(h.messages()))
	}

	fx.poll(w)
	fx.poll(w)
	if !slices.Contains(fx.kinds(w), string(store.ActivityBranchNotUpdated)) {
		t.Fatalf("kinds = %v, want the stall recorded after three poll intervals", fx.kinds(w))
	}
	msgs := h.messages()
	if len(msgs) != 2 || !strings.Contains(msgs[1], "is behind main") {
		t.Fatalf("messages = %q, want the agent told once the update stalled", msgs)
	}
}

func TestTheWorkBranchFollowsABranchSomeoneElseRewrote(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.agentIdle(w)
	fx.update(func() { fx.pr.HeadSHA = "x1" })
	fx.rel.set(func(f *fakeRelease) { f.history["x1"] = []string{"base"}; f.remote = "x1" })
	fx.poll(w)

	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:05:00Z"), Body: "one more thing", URL: "https://c/11"}}
	})
	fx.poll(w)

	if work, _ := fx.rel.Head(context.Background(), ""); work != "x1" {
		t.Fatalf("work branch = %s, want x1: it had nothing that the rewritten branch lacks", work)
	}
}

func TestAWatchThatMergesMergesTheBranchThatMovedUnderItsWork(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	req := fx.startRequest()
	req.ApprovalMode = new(store.ApprovalManual)
	req.BranchUpdate = new(store.BranchMerge)
	w, err := fx.svc.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	fx.propose(w)
	fx.rel.set(func(f *fakeRelease) { f.merges = append(f.merges, "w1") })
	fx.update(func() { fx.pr.HeadSHA = "t1" })
	fx.rel.moveRemote("abc", "t1")

	fx.poll(w)
	p := fx.proposal(w, 1)
	if p.Status != store.ProposalPending || p.WorkSHA != "w1-merge-t1" || len(fx.rel.rebases) != 0 {
		t.Fatalf("proposal = %+v, rebases %v, want the work merged with t1 and offered again", p, fx.rel.rebases)
	}

	if _, err := fx.svc.Approve(context.Background(), w.ID, 1, Decision{}); err != nil {
		t.Fatal(err)
	}
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "w1-merge-t1", Branch: "fix"}}) {
		t.Fatalf("pushes = %+v, want the merged work pushed without force", got)
	}
}

func TestGitHubWaitsForThePollAfterTheDaemonPushed(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startWith(func(r *StartRequest) {
		r.ApprovalMode, r.AutoApproveRebase = new(store.ApprovalManual), new(true)
	})
	fx.propose(w)
	fx.rel.set(func(f *fakeRelease) { f.pushErr = errors.New("the lease refused the push") })
	if p, err := fx.svc.Approve(context.Background(), w.ID, 1, Decision{}); err != nil || p.Status != store.ProposalFailed {
		t.Fatalf("Approve() = %+v, %v, want the push to fail", p, err)
	}
	fx.rel.set(func(f *fakeRelease) { f.pushErr = nil })
	fx.update(func() { fx.pr.HeadSHA, fx.pr.MergeableState = "t1", "behind" })
	fx.rel.moveRemote("abc", "t1")
	fx.agentIdle(w)

	fx.poll(w)

	if got := fx.rel.pushed(); len(got) == 0 || got[len(got)-1].SHA != "w1-on-t1" {
		t.Fatalf("pushes = %+v, want the approved work pushed on t1", got)
	}
	if got := fx.branchUpdates(); len(got) != 0 {
		t.Fatalf("branch updates = %+v, want none in the poll that pushed: t1 is no longer the head", got)
	}

	fx.update(func() { fx.pr.HeadSHA = "w1-on-t1" })
	fx.poll(w)
	if got := fx.branchUpdates(); !slices.Equal(got, []ghfake.BranchUpdate{{Method: "REBASE", ExpectedHead: "w1-on-t1"}}) {
		t.Fatalf("branch updates = %+v, want one update of the pushed head", got)
	}
}

func TestAWatchChangesHowItUpdatesItsBranch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()

	got, err := fx.svc.SetMergeRules(context.Background(), w.ID, MergeRulesChange{BranchUpdate: new(store.BranchMerge), UpdateOnGitHub: new(false)})
	if err != nil || got.BranchUpdate != store.BranchMerge || got.UpdateOnGitHub {
		t.Fatalf("SetMergeRules() = %+v, %v, want a merge by the agent", got, err)
	}
	for _, bad := range []store.BranchUpdate{"squash", "Merge", ""} {
		if _, err := fx.svc.SetMergeRules(context.Background(), w.ID, MergeRulesChange{BranchUpdate: new(bad)}); !errors.Is(err, ErrBadBranchUpdate) {
			t.Fatalf("SetMergeRules(%q) error = %v, want ErrBadBranchUpdate", bad, err)
		}
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
