package prwatch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (fx *fixture) startManual() store.Watch {
	fx.t.Helper()
	req := fx.startRequest()
	req.ApprovalMode = new(store.ApprovalManual)
	w, err := fx.svc.Start(context.Background(), req)
	if err != nil {
		fx.t.Fatalf("Start() error = %v", err)
	}
	return w
}

func (fx *fixture) propose(w store.Watch) store.Proposal {
	fx.t.Helper()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.reply(w, 31, "renamed it")
	fx.hook(w, agent.EventStop, `{}`)
	return fx.proposal(w, 1)
}

func (fx *fixture) messages() []string {
	return fx.host.last().messages()
}

func (fx *fixture) replyID(p store.Proposal, i int) int64 {
	fx.t.Helper()
	replies, err := fx.st.ProposalReplies(context.Background(), p.ID)
	if err != nil {
		fx.t.Fatal(err)
	}
	return replies[i].ID
}

func TestAManualTurnWaitsForTheAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.startManual()
	if w.ApprovalMode != store.ApprovalManual {
		t.Fatalf("watch = %+v", w)
	}
	p := fx.propose(w)

	if p.Status != store.ProposalPending || len(fx.rel.pushed()) != 0 || len(fx.posted()) != 0 {
		t.Fatalf("proposal = %+v, pushes %v, posted %q", p, fx.rel.pushed(), fx.posted())
	}
	rows := fx.activity(w)
	last := rows[len(rows)-1]
	if last.Kind != store.ActivityProposal || !strings.Contains(last.Summary, "proposal 1 waits on you: 1 commit, 1 file and 1 reply") ||
		!strings.Contains(last.Summary, "babysitter watch proposals 1 1") {
		t.Fatalf("proposal row = %+v", last)
	}
	if kinds := fx.notes.noteKinds(); kinds[len(kinds)-1] != "agent" {
		t.Fatalf("notification kinds = %v", kinds)
	}

	fx.poll(w)
	fx.advance(2 * fx.svc.Interval())
	fx.poll(w)
	got := fx.watch(w)
	if !slices.Contains(got.ReadyBlockers, "proposal 1 waits on your approval") || got.ReadySince != nil {
		t.Fatalf("readiness = %v, %v", got.ReadySince, got.ReadyBlockers)
	}
	if slices.Contains(kindsOf(fx.activity(w)), "merge_ready") {
		t.Fatal("the watch called the pull request ready while work waits on the author")
	}
	if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{}); err == nil || !strings.Contains(err.Error(), "proposal 1 waits on your approval") {
		t.Fatalf("Merge() error = %v", err)
	}
}

func TestApprovingReleasesTheProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.propose(w)

	d, err := fx.svc.Proposal(ctx, w.ID, 1, CodeQuery{})
	if err != nil {
		t.Fatalf("Proposal() error = %v", err)
	}
	if len(d.Commits) != 1 || d.Commits[0].SHA != "w1" || len(d.Files) != 1 || !strings.Contains(d.Diff, "diff --git") ||
		len(d.Replies) != 1 || d.Replies[0].Body != "renamed it" || d.Replies[0].Answers == nil || d.Replies[0].Answers.Actor != "bob" {
		t.Fatalf("detail = %+v", d)
	}

	p, err := fx.svc.Approve(ctx, w.ID, 1, Decision{})
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if p.Status != store.ProposalReleased || p.ApprovedAt == nil {
		t.Fatalf("approved proposal = %+v", p)
	}
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "w1", Branch: "fix"}}) {
		t.Fatalf("pushes = %+v", got)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"31:renamed it"}) {
		t.Fatalf("posted = %q", got)
	}
	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{}); !errors.Is(err, ErrNotPending) {
		t.Fatalf("a second Approve() error = %v", err)
	}
}

func TestTheCodeOfOneCommitOfAProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.turn(w)
	fx.rel.commit("abc", "first-commit")
	fx.rel.commit("first-commit", "second-commit")
	fx.hook(w, agent.EventStop, `{}`)

	for _, tc := range []struct{ ask, commit, base, diff string }{
		{ask: "first-commit", commit: "first-commit", base: "abc", diff: "-abc\n+first-commit\n"},
		{ask: "SECOND-", commit: "second-commit", base: "first-commit", diff: "-first-commit\n+second-commit\n"},
		{ask: "", commit: "", base: "abc", diff: "-abc\n+second-commit\n"},
	} {
		d, err := fx.svc.Proposal(ctx, w.ID, 1, CodeQuery{Commit: tc.ask})
		if err != nil {
			t.Fatalf("Proposal(%q) error = %v", tc.ask, err)
		}
		shows := d.Commit == tc.commit && d.Base == tc.base && strings.HasSuffix(d.Diff, tc.diff)
		if !shows || len(d.Commits) != 2 || d.CodeError != "" {
			t.Fatalf("Proposal(%q) = commit %q, base %q, %d commits, diff %q, code error %q", tc.ask, d.Commit, d.Base, len(d.Commits), d.Diff, d.CodeError)
		}
	}
	for _, ask := range []string{"second", "abc", "nothing"} {
		if _, err := fx.svc.Proposal(ctx, w.ID, 1, CodeQuery{Commit: ask}); !errors.Is(err, ErrUnknownCommit) {
			t.Fatalf("Proposal(%q) error = %v, want ErrUnknownCommit", ask, err)
		}
	}
}

func TestTheCodeOfOneFileOfAProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.turn(w)
	fx.rel.commit("abc", "first-commit")
	fx.hook(w, agent.EventStop, `{}`)

	d, err := fx.svc.Proposal(ctx, w.ID, 1, CodeQuery{Commit: "first-commit", Path: "x.go"})
	if err != nil || len(d.Files) != 1 || d.Files[0].Path != "x.go" || d.Base != "abc" {
		t.Fatalf("Proposal(x.go) = files %+v, base %q, %v", d.Files, d.Base, err)
	}
	fx.rel.mu.Lock()
	asked := fx.rel.diffPaths[len(fx.rel.diffPaths)-1]
	fx.rel.mu.Unlock()
	if !slices.Equal(asked, []string{"x.go"}) {
		t.Fatalf("the diff was read for %q, want x.go only", asked)
	}
	if _, err := fx.svc.Proposal(ctx, w.ID, 1, CodeQuery{Path: "nope.go"}); !errors.Is(err, ErrUnknownFile) {
		t.Fatalf("Proposal(nope.go) error = %v, want ErrUnknownFile", err)
	}
}

func TestAFileOfAProposalWithRepliesOnlyIsChecked(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.turn(w)
	fx.reply(w, 31, "noted")
	fx.hook(w, agent.EventStop, `{}`)

	if _, err := fx.svc.Proposal(ctx, w.ID, 1, CodeQuery{Path: "nope.go"}); !errors.Is(err, ErrUnknownFile) {
		t.Fatalf("Proposal(nope.go) of a proposal with replies only, error = %v, want ErrUnknownFile", err)
	}
}

func TestAnEditedReplyIsPostedAndTheAgentHearsOfIt(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	p := fx.propose(w)
	told := len(fx.messages())

	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{Edits: map[int64]string{fx.replyID(p, 0): "Renamed it, thanks for the catch."}}); err != nil {
		t.Fatal(err)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"31:Renamed it, thanks for the catch."}) {
		t.Fatalf("posted = %q", got)
	}
	msgs := fx.messages()
	if len(msgs) != told+1 || !strings.Contains(msgs[told], "Renamed it, thanks for the catch.") || !strings.Contains(msgs[told], "Nothing to do now") {
		t.Fatalf("messages = %q", msgs[told:])
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)
	if ps := fx.proposals(w); len(ps) != 1 {
		t.Fatalf("the message that asks for nothing left a proposal: %+v", ps)
	}
}

func TestADroppedReplyBringsItsCommentBack(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	p := fx.propose(w)

	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{Drop: []int64{fx.replyID(p, 0)}}); err != nil {
		t.Fatal(err)
	}
	if got := fx.posted(); len(got) != 0 {
		t.Fatalf("posted = %q", got)
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)
	told := len(fx.messages())

	fx.poll(w)
	msgs := fx.messages()
	if len(msgs) != told+1 || !strings.Contains(msgs[told], "rename this") || !strings.Contains(msgs[told], "comment id 31") {
		t.Fatalf("the dropped comment did not come back: %q", msgs[told:])
	}
}

func TestAStaleProposalIsRebasedAndOfferedAgain(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.propose(w)

	fx.update(func() { fx.pr.HeadSHA = "t1" })
	fx.rel.moveRemote("abc", "t1")
	fx.poll(w)
	p := fx.proposal(w, 1)
	if p.Status != store.ProposalPending || p.HeadSHA != "t1" || p.WorkSHA != "w1-on-t1" || p.RebasedFrom != "w1" {
		t.Fatalf("rebased proposal = %+v", p)
	}
	if !slices.Contains(refsOf(fx.activity(w), store.ActivityProposal), "1 rebased w1-on-t1") {
		t.Fatalf("activity = %v", fx.activity(w))
	}
	if len(fx.rel.pushed()) != 0 {
		t.Fatalf("pushes = %v", fx.rel.pushed())
	}

	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{}); err != nil {
		t.Fatal(err)
	}
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "w1-on-t1", Branch: "fix"}}) {
		t.Fatalf("pushes = %+v", got)
	}
}

func TestApprovedWorkGoesOutAfterACleanRebase(t *testing.T) {
	t.Parallel()
	for _, autoRebase := range []bool{true, false} {
		fx := newFixture(t)
		ctx := context.Background()
		req := fx.startRequest()
		req.ApprovalMode, req.AutoApproveRebase = new(store.ApprovalManual), new(autoRebase)
		w, err := fx.svc.Start(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		fx.propose(w)
		fx.update(func() { fx.pr.HeadSHA = "t1" })
		fx.rel.moveRemote("abc", "t1")
		fx.rel.set(func(f *fakeRelease) { f.missing = []string{"t1"} })

		p, err := fx.svc.Approve(ctx, w.ID, 1, Decision{})
		if err != nil {
			t.Fatal(err)
		}
		if autoRebase {
			if p.Status != store.ProposalReleased || !slices.Equal(fx.rel.pushed(), []gitrelease.Push{{SHA: "w1-on-t1", Branch: "fix"}}) {
				t.Fatalf("auto rebase: proposal = %+v, pushes %v", p, fx.rel.pushed())
			}
			continue
		}
		if p.Status != store.ProposalFailed || len(fx.rel.rebases) != 0 {
			t.Fatalf("no auto rebase: approve on a branch that moved = %+v, rebases %v", p, fx.rel.rebases)
		}
		fx.poll(w)
		p = fx.proposal(w, 1)
		if p.Status != store.ProposalPending || p.ApprovedAt != nil || p.WorkSHA != "w1-on-t1" || len(fx.rel.pushed()) != 0 {
			t.Fatalf("no auto rebase: proposal = %+v, pushes %v", p, fx.rel.pushed())
		}
	}
}

func TestAStaleRewriteIsNotRebased(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.propose(w)
	fx.rel.set(func(f *fakeRelease) { f.history["w1"] = []string{"a0"} })

	fx.update(func() { fx.pr.HeadSHA = "t1" })
	fx.rel.moveRemote("abc", "t1")
	fx.poll(w)
	if p := fx.proposal(w, 1); p.Status != store.ProposalPending || p.WorkSHA != "w1" || len(fx.rel.rebases) != 0 {
		t.Fatalf("proposal = %+v, rebases %v", p, fx.rel.rebases)
	}
}

func TestARebaseThatConflictsGoesToTheAgentAndItsResolutionAsks(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	req := fx.startRequest()
	req.ApprovalMode, req.AutoApproveRebase = new(store.ApprovalManual), new(true)
	w, err := fx.svc.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	fx.propose(w)
	told := len(fx.messages())

	fx.update(func() { fx.pr.HeadSHA = "t1" })
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) { f.rebaseErr = &gitrelease.ConflictError{Files: []string{"x.go"}} })
	fx.poll(w)
	if p := fx.proposal(w, 1); p.Status != store.ProposalFailed || !strings.Contains(p.Error, "conflicts in x.go") {
		t.Fatalf("proposal = %+v", p)
	}
	msgs := fx.messages()
	if len(msgs) != told+1 || !strings.Contains(msgs[told], "could not push your work") {
		t.Fatalf("messages = %q", msgs[told:])
	}
	if kinds := fx.notes.noteKinds(); kinds[len(kinds)-1] != "watch" {
		t.Fatalf("notification kinds = %v", kinds)
	}
	fx.conflictNamesNoRetry(w)

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.set(func(f *fakeRelease) {
		f.rebaseErr = nil
		f.history["w2"] = []string{"t1", "abc"}
		f.work = "w2"
	})
	fx.hook(w, agent.EventStop, `{}`)
	if next := fx.proposal(w, 2); next.Status != store.ProposalPending || len(fx.rel.pushed()) != 0 {
		t.Fatalf("the resolution went out without the author: %+v, pushes %v", next, fx.rel.pushed())
	}
}

func TestApproveAndStopAsking(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	fx.settings(store.Settings{ApprovalMode: store.ApprovalManual})
	w := fx.start()
	fx.propose(w)
	other := fx.startNumber(4)

	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{StopAsking: true}); err != nil {
		t.Fatal(err)
	}
	if got := fx.watch(w); got.ApprovalMode != store.ApprovalAuto {
		t.Fatalf("watch = %+v", got)
	}
	if got := fx.watch(other); got.ApprovalMode != store.ApprovalManual {
		t.Fatalf("the other watch = %+v", got)
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit("w1", "w2")
	fx.hook(w, agent.EventStop, `{}`)
	if p := fx.proposal(w, 2); p.Status != store.ProposalReleased {
		t.Fatalf("the next turn asked: %+v", p)
	}
}

func TestSwitchingToAutoReleasesThePendingProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.propose(w)

	auto := store.ApprovalAuto
	if _, err := fx.svc.SetApproval(ctx, w.ID, ApprovalChange{Mode: &auto}); !errors.Is(err, ErrProposalPending) || !strings.Contains(err.Error(), "proposal 1") {
		t.Fatalf("a flip without the release error = %v", err)
	}
	got, err := fx.svc.SetApproval(ctx, w.ID, ApprovalChange{Mode: &auto, Release: true})
	if err != nil || got.ApprovalMode != store.ApprovalAuto {
		t.Fatalf("SetApproval() = %+v, %v", got, err)
	}
	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased || len(fx.rel.pushed()) != 1 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
	manual := store.ApprovalManual
	if got, err := fx.svc.SetApproval(ctx, w.ID, ApprovalChange{Mode: &manual, AutoApproveRebase: new(true)}); err != nil || !got.Asks() || !got.AutoApproveRebase {
		t.Fatalf("back to manual = %+v, %v", got, err)
	}
}

func TestWhatArrivesDuringADecisionGoesOutAfterIt(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.turn(w)
	fx.reply(w, 31, "the name is on purpose")
	fx.hook(w, agent.EventStop, `{}`)
	told := len(fx.messages())

	fx.update(func() {
		fx.pr.HeadSHA = "t1"
		fx.pr.IssueComments = []ghfake.Comment{{ID: 41, Author: "carol", CreatedAt: ghfake.At("2026-09-07T13:00:00Z"), Body: "first thought", URL: "https://c/41"}, {ID: 42, Author: "dave", CreatedAt: ghfake.At("2026-09-07T13:01:00Z"), Body: "second thought", URL: "https://c/42"}}
	})
	fx.failBuild(stampedLog("##[error]boom"))
	fx.poll(w)
	fx.poll(w)
	if n := len(fx.messages()); n != told {
		t.Fatalf("the agent heard %d messages while the author decided", n-told)
	}
	if _, err := fx.svc.Send(ctx, w.ID, "hurry up"); !errors.Is(err, ErrProposalPending) {
		t.Fatalf("Send() error = %v, want ErrProposalPending", err)
	}

	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{}); err != nil {
		t.Fatal(err)
	}
	fx.poll(w)
	msgs := fx.messages()
	last := msgs[len(msgs)-1]
	for _, want := range []string{"first thought", "second thought", "Failed: build"} {
		if !strings.Contains(last, want) {
			t.Errorf("the message after the decision lacks %q:\n%s", want, last)
		}
	}
}

func TestRejectingThePushPostsTheRepliesOnly(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.propose(w)
	told := len(fx.messages())

	p, err := fx.svc.Approve(context.Background(), w.ID, 1, Decision{RejectPush: true})
	if err != nil || p.Status != store.ProposalReleased || !p.PushRejected {
		t.Fatalf("Approve() = %+v, %v", p, err)
	}
	if len(fx.rel.pushed()) != 0 || len(fx.posted()) != 1 {
		t.Fatalf("pushes %v, posted %q", fx.rel.pushed(), fx.posted())
	}
	if msgs := fx.messages(); len(msgs) != told+1 || !strings.Contains(msgs[told], "pushed nothing") {
		t.Fatalf("messages = %q", msgs[told:])
	}
}

func TestRejectingTheProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	fx.settings(store.Settings{ApprovalMode: store.ApprovalManual, KeepWorktree: true})
	w := fx.start()
	fx.propose(w)
	told := len(fx.messages())

	p, err := fx.svc.Reject(ctx, w.ID, 1, Rejection{Reason: "use a table test", Discard: true})
	if err != nil || p.Status != store.ProposalRejected || p.Reason != "use a table test" {
		t.Fatalf("Reject() = %+v, %v", p, err)
	}
	if len(fx.rel.pushed()) != 0 || len(fx.posted()) != 0 || !slices.Equal(fx.rel.discards, []string{"abc"}) {
		t.Fatalf("pushes %v, posted %q, discards %v", fx.rel.pushed(), fx.posted(), fx.rel.discards)
	}
	msgs := fx.messages()
	if len(msgs) != told+1 || !strings.Contains(msgs[told], "use a table test") || !strings.Contains(msgs[told], "reset your work branch") {
		t.Fatalf("messages = %q", msgs[told:])
	}
	seen, _ := fx.st.SeenReviewItems(ctx, w.Key())
	if seen[store.SeenItem{Kind: store.KindReviewComment, ID: 31}] {
		t.Fatal("the comment of a rejected reply stays seen")
	}
	if _, err := fx.svc.Reject(ctx, w.ID, 1, Rejection{}); !errors.Is(err, ErrNotPending) {
		t.Fatalf("a second Reject() error = %v", err)
	}
}

func TestStopDeclinesThePendingProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.propose(w)
	if _, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if p := fx.proposal(w, 1); p.Status != store.ProposalDeclined || len(fx.rel.pushed()) != 0 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
}

func TestASelfWatchHasNoGate(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	fx.settings(store.Settings{ApprovalMode: store.ApprovalManual})
	w := fx.startSelf()
	if w.ApprovalMode != store.ApprovalAuto {
		t.Fatalf("self watch = %+v", w)
	}
	if _, err := fx.svc.Proposals(ctx, w.ID); !errors.Is(err, ErrSelfWatch) {
		t.Fatalf("Proposals() error = %v", err)
	}
	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{}); !errors.Is(err, ErrSelfWatch) {
		t.Fatalf("Approve() error = %v", err)
	}
	if _, err := fx.svc.Reject(ctx, w.ID, 1, Rejection{}); !errors.Is(err, ErrSelfWatch) {
		t.Fatalf("Reject() error = %v", err)
	}
	manual := store.ApprovalManual
	if _, err := fx.svc.SetApproval(ctx, w.ID, ApprovalChange{Mode: &manual}); !errors.Is(err, ErrSelfWatch) {
		t.Fatalf("SetApproval() error = %v", err)
	}
}

func TestStartTakesTheApprovalModeOfTheSettings(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.settings(store.Settings{ApprovalMode: store.ApprovalManual, AutoApproveRebase: true})
	w := fx.start()
	if w.ApprovalMode != store.ApprovalManual || !w.AutoApproveRebase {
		t.Fatalf("watch = %+v", w)
	}
	fx.openPR(4)
	req := StartRequest{Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 4}, SourceDir: fx.dir, ApprovalMode: new(store.ApprovalAuto)}
	other, err := fx.svc.Start(context.Background(), req)
	if err != nil || other.ApprovalMode != store.ApprovalAuto {
		t.Fatalf("a watch that asks for auto = %+v, %v", other, err)
	}
}

func TestAFailedReleaseBlocksTheMerge(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.rel.set(func(f *fakeRelease) { f.pushErr = errors.New("no credential") })
	fx.hook(w, agent.EventStop, `{}`)
	fx.poll(w)
	want := "proposal 1 of the agent did not go out; retry with `/opt/babysitter watch retry 1 1`"
	if got := fx.watch(w); !slices.Contains(got.ReadyBlockers, want) {
		t.Fatalf("blockers = %v", got.ReadyBlockers)
	}
}

func TestTheBlockerOfAConflictNobodyApprovedNamesNoRetry(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.propose(w)
	fx.update(func() { fx.pr.HeadSHA = "t1" })
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) { f.rebaseErr = &gitrelease.ConflictError{Files: []string{"x.go"}} })
	fx.poll(w)
	fx.poll(w)
	want := "proposal 1 of the agent did not go out; the agent resolves it in its next turn"
	if got := fx.watch(w); !slices.Contains(got.ReadyBlockers, want) {
		t.Fatalf("blockers = %v", got.ReadyBlockers)
	}
}

func TestAResolvedConflictPostsOneReplyPerComment(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.propose(w)

	fx.update(func() { fx.pr.HeadSHA = "t1" })
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) { f.rebaseErr = &gitrelease.ConflictError{Files: []string{"x.go"}} })
	fx.poll(w)

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.set(func(f *fakeRelease) {
		f.rebaseErr = nil
		f.history["w2"] = []string{"t1", "abc"}
		f.work = "w2"
	})
	fx.reply(w, 31, "renamed it in w2, on top of t1")
	fx.hook(w, agent.EventStop, `{}`)

	if _, err := fx.svc.Approve(ctx, w.ID, 2, Decision{}); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"31:renamed it in w2, on top of t1"}) {
		t.Fatalf("posted = %q, want one answer to comment 31", got)
	}
}

func TestAReplyAfterACleanRebaseNamesNoCommitThatNeverLanded(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	req := fx.startRequest()
	req.ApprovalMode, req.AutoApproveRebase = new(store.ApprovalManual), new(true)
	w, err := fx.svc.Start(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.reply(w, 31, "Fixed in w1.")
	fx.hook(w, agent.EventStop, `{}`)

	fx.update(func() { fx.pr.HeadSHA = "t1" })
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) { f.missing = []string{"t1"} })
	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{}); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	fx.poll(w)

	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "w1-on-t1", Branch: "fix"}}) {
		t.Fatalf("pushes = %+v, want the rebased work", got)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"31:Fixed in w1-on-t1."}) {
		t.Fatalf("posted = %q: the reply must name w1-on-t1, the commit the pull request got", got)
	}
}

func TestARebasedProposalNamesItsNewCommits(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.reply(w, 31, "Fixed in w1.")
	fx.reply(w, 0, "The test is in w1 too.")
	fx.hook(w, agent.EventStop, `{}`)

	fx.update(func() { fx.pr.HeadSHA = "t1" })
	fx.rel.moveRemote("abc", "t1")
	fx.poll(w)

	d, err := fx.svc.Proposal(ctx, w.ID, 1, CodeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if d.RebasedFrom != "w1" || d.Replies[0].Text() != "Fixed in w1-on-t1." || d.Replies[1].Text() != "The test is in w1-on-t1 too." {
		t.Fatalf("rebased proposal = %+v, replies %+v", d.Proposal, d.Replies)
	}
}

func TestAReleaseThatRebasesNamesThePushedCommits(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.reply(w, 31, "Fixed in w1.")
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) { f.missing = []string{"t1"} })
	fx.hook(w, agent.EventStop, `{}`)

	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased || p.WorkSHA != "w1-on-t1" {
		t.Fatalf("proposal = %+v", p)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"31:Fixed in w1-on-t1."}) {
		t.Fatalf("posted = %q", got)
	}
}

func TestWorkThatLandedIsNotOfferedAgainAsARebase(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.propose(w)
	fx.failReplies(31)
	p, err := fx.svc.Approve(ctx, w.ID, 1, Decision{})
	if err != nil || p.Status != store.ProposalFailed || !slices.Equal(fx.rel.pushed(), []gitrelease.Push{{SHA: "w1", Branch: "fix"}}) {
		t.Fatalf("Approve() = %+v, %v, pushes %v", p, err, fx.rel.pushed())
	}

	fx.update(func() { fx.pr.HeadSHA = "w1" })
	fx.poll(w)
	p = fx.proposal(w, 1)
	if p.Status != store.ProposalFailed || p.ApprovedAt == nil || p.RebasedFrom != "" {
		t.Fatalf("the work that landed came back as a rebase: %+v", p)
	}
	for _, ref := range refsOf(fx.activity(w), store.ActivityProposal) {
		if strings.Contains(ref, "rebased") {
			t.Fatalf("the author was asked again: proposal rows %v", refsOf(fx.activity(w), store.ActivityProposal))
		}
	}
}

func (fx *fixture) takeOverALandedPush(w store.Watch) store.Proposal {
	fx.t.Helper()
	fx.propose(w)
	fx.failReplies(31)
	p, err := fx.svc.Approve(context.Background(), w.ID, 1, Decision{})
	if err != nil || p.Status != store.ProposalFailed || !slices.Equal(fx.rel.pushed(), []gitrelease.Push{{SHA: "w1", Branch: "fix"}}) {
		fx.t.Fatalf("Approve() = %+v, %v, pushes %v", p, err, fx.rel.pushed())
	}
	fx.update(func() { fx.pr.HeadSHA = "w1" })
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit("w1", "w2")
	fx.hook(w, agent.EventStop, `{}`)
	return fx.proposal(w, 2)
}

func TestATurnThatTakesOverALandedPushStartsOnIt(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	if p := fx.takeOverALandedPush(w); p.HeadSHA != "w1" {
		t.Fatalf("the proposal starts below the push that landed: %+v", p)
	}
	fx.poll(w)
	if p := fx.proposal(w, 2); p.Status != store.ProposalPending || p.RebasedFrom != "" {
		t.Fatalf("the push that landed came back as a rebase: %+v", p)
	}
}

func TestATurnThatTakesOverAHeadThatLeftTheBranchStartsOnTheBranch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.propose(w)
	fx.rel.set(func(f *fakeRelease) { f.pushErr = errors.New("Could not resolve host: github.com") })
	if p, err := fx.svc.Approve(context.Background(), w.ID, 1, Decision{}); err != nil || p.Status != store.ProposalFailed {
		t.Fatalf("Approve() = %+v, %v", p, err)
	}
	fx.rel.set(func(f *fakeRelease) { f.pushErr = nil })
	fx.rel.moveRemote("r0", "t1")
	fx.update(func() { fx.pr.HeadSHA = "t1" })

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit("w1", "w2")
	fx.hook(w, agent.EventStop, `{}`)
	if p := fx.proposal(w, 2); p.HeadSHA != "t1" {
		t.Fatalf("the proposal starts on a head the branch no longer has: %+v", p)
	}
}

func TestARejectAfterALandedPushKeepsThatPush(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.takeOverALandedPush(w)
	if _, err := fx.svc.Reject(context.Background(), w.ID, 2, Rejection{Discard: true}); err != nil {
		t.Fatalf("Reject() error = %v", err)
	}
	if got := fx.rel.discards; !slices.Equal(got, []string{"w1"}) {
		t.Fatalf("the reject reset the work branch below the push that landed: discards %v", got)
	}
}

func TestAReplyToADeletedCommentSaysTheCommentIsGone(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.propose(w)
	fx.update(func() { fx.repo.Deleted = []int64{31} })
	p, err := fx.svc.Approve(ctx, w.ID, 1, Decision{})
	if err != nil || p.Status != store.ProposalReleased {
		t.Fatalf("Approve() = %+v, %v", p, err)
	}
	replies, err := fx.st.ProposalReplies(ctx, p.ID)
	if err != nil || len(replies) != 1 || replies[0].DroppedAt == nil {
		t.Fatalf("replies = %+v, %v", replies, err)
	}
	if reason := replies[0].Error; strings.Contains(reason, "comment on the pull request") || !strings.Contains(reason, "deleted") {
		t.Fatalf("the drop names the wrong cause: %q", reason)
	}
}

func TestARetryDoesNotReleaseWorkTheAuthorNeverApproved(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.propose(w)
	fx.update(func() { fx.pr.HeadSHA = "t1" })
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) { f.rebaseErr = &gitrelease.ConflictError{Files: []string{"x.go"}} })
	fx.poll(w)
	if p := fx.proposal(w, 1); p.Status != store.ProposalFailed || p.ApprovedAt != nil {
		t.Fatalf("proposal after the conflict = %+v", p)
	}

	fx.rel.set(func(f *fakeRelease) {
		f.rebaseErr = nil
		f.remote = "abc"
	})
	if _, err := fx.svc.Retry(ctx, w.ID, 1); !errors.Is(err, ErrNothingToRetry) {
		t.Fatalf("Retry() error = %v, want ErrNothingToRetry", err)
	}
	if len(fx.rel.pushed()) != 0 || len(fx.posted()) != 0 {
		t.Fatalf("a retry released work nobody approved: pushes %v, posted %q", fx.rel.pushed(), fx.posted())
	}
}

func TestARejectOfWorkThatWentOutIsRefused(t *testing.T) {
	t.Parallel()
	for name, turn := range map[string]func(fx *fixture, w store.Watch){
		"the push landed": func(fx *fixture, w store.Watch) {
			fx.rel.commit("abc", "w1")
			fx.reply(w, 31, "renamed it")
		},
		"a reply is posted": func(fx *fixture, w store.Watch) {
			fx.reply(w, 0, "the name is on purpose")
			fx.reply(w, 31, "kept it")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			w := fx.start()
			fx.turn(w)
			turn(fx, w)
			fx.failReplies(31)
			fx.hook(w, agent.EventStop, `{}`)
			if p := fx.proposal(w, 1); p.Status != store.ProposalFailed {
				t.Fatalf("proposal = %+v", p)
			}
			told := len(fx.messages())

			if _, err := fx.svc.Reject(context.Background(), w.ID, 1, Rejection{Discard: true}); !errors.Is(err, ErrNotPending) {
				t.Fatalf("Reject() error = %v, want ErrNotPending", err)
			}
			if p := fx.proposal(w, 1); p.Status != store.ProposalFailed || len(fx.rel.resets) != 0 || len(fx.messages()) != told {
				t.Fatalf("proposal %+v, resets %v, messages %q", p, fx.rel.resets, fx.messages()[told:])
			}
		})
	}
}

func (fx *fixture) conflictNamesNoRetry(w store.Watch) {
	fx.t.Helper()
	var row store.Activity
	for _, a := range fx.activity(w) {
		if a.Kind == store.ActivityAgentFailed {
			row = a
		}
	}
	notes := fx.notes.messages()
	note := notes[len(notes)-1]
	if !strings.Contains(row.Summary, "conflicts in x.go") || !strings.Contains(row.Summary, "the agent resolves it in its next turn") {
		fx.t.Fatalf("conflict row = %q", row.Summary)
	}
	if strings.Contains(row.Summary, "watch retry") || strings.Contains(string(row.Payload), `"retry"`) || strings.Contains(note, "watch retry") {
		fx.t.Fatalf("the conflict names a retry: row %q, payload %s, notification %q", row.Summary, row.Payload, note)
	}
}

func kindsOf(rows []store.Activity) []string {
	out := make([]string, 0, len(rows))
	for _, a := range rows {
		out = append(out, string(a.Kind))
	}
	return out
}

func refsOf(rows []store.Activity, kind store.ActivityKind) []string {
	var out []string
	for _, a := range rows {
		if a.Kind == kind {
			out = append(out, a.Ref)
		}
	}
	return out
}

func (fx *fixture) conversation() {
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 422, Author: "carol", CreatedAt: ghfake.At("2026-09-07T12:06:00Z"), Body: "add a README section", URL: "https://c/422"}}
	})
}

func (fx *fixture) answerConversation(w store.Watch, parent, child, body string) {
	fx.t.Helper()
	fx.conversation()
	fx.poll(w)
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit(parent, child)
	fx.reply(w, 422, body)
	fx.hook(w, agent.EventStop, `{}`)
}

func TestADroppedAnswerToAConversationCommentBringsItBack(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.answerConversation(w, "abc", "w1", "added a README section")
	p := fx.proposal(w, 1)
	d, err := fx.svc.Proposal(ctx, w.ID, 1, CodeQuery{})
	if err != nil || len(d.Replies) != 1 || d.Replies[0].Answers == nil || d.Replies[0].Answers.Actor != "carol" {
		t.Fatalf("the reply does not name the comment it answers: %+v, %v", d.Replies, err)
	}

	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{Drop: []int64{fx.replyID(p, 0)}}); err != nil {
		t.Fatal(err)
	}
	if got := fx.posted(); len(got) != 0 {
		t.Fatalf("posted = %q", got)
	}
	msgs := fx.messages()
	if last := msgs[len(msgs)-1]; !strings.Contains(last, "comes back to you") {
		t.Fatalf("the decision does not say the comment comes back: %q", last)
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)
	told := len(fx.messages())

	fx.poll(w)
	msgs = fx.messages()
	if len(msgs) != told+1 || !strings.Contains(msgs[told], "add a README section") || !strings.Contains(msgs[told], "--to 422") {
		t.Fatalf("the conversation comment did not come back: %q", msgs[told:])
	}
}

func TestARejectBringsTheConversationCommentBack(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.answerConversation(w, "abc", "w1", "added a README section")

	if _, err := fx.svc.Reject(ctx, w.ID, 1, Rejection{}); err != nil {
		t.Fatal(err)
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)
	told := len(fx.messages())

	fx.poll(w)
	msgs := fx.messages()
	if len(msgs) != told+1 || !strings.Contains(msgs[told], "add a README section") {
		t.Fatalf("the conversation comment did not come back: %q", msgs[told:])
	}
}

func TestATurnThatAnswersAConversationCommentAgainPostsOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.rel.set(func(f *fakeRelease) { f.pushErr = errors.New("no credential") })
	fx.answerConversation(w, "abc", "w1", "added a README section")
	if p := fx.proposal(w, 1); p.Status != store.ProposalFailed {
		t.Fatalf("proposal 1 = %+v", p)
	}
	fx.rel.set(func(f *fakeRelease) { f.pushErr = nil })

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit("w1", "w2")
	fx.reply(w, 422, "put the example in a JSDoc comment instead")
	fx.hook(w, agent.EventStop, `{}`)

	if got := fx.posted(); !slices.Equal(got, []string{"0:put the example in a JSDoc comment instead"}) {
		t.Fatalf("posted = %q", got)
	}
}

func TestTheRowOfAnEditedReplyNamesTheAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	p := fx.propose(w)

	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{Edits: map[int64]string{fx.replyID(p, 0): "Renamed it, thanks."}}); err != nil {
		t.Fatal(err)
	}
	row, ok, err := fx.st.ActivityByRef(ctx, w.ID, store.ActivityReplied, fmt.Sprint(fx.newestComment().ID))
	if err != nil || !ok || row.Summary != "you replied in the thread of comment 31: Renamed it, thanks." {
		t.Fatalf("replied row = %+v, %v, %v", row, ok, err)
	}
}

func TestTheStopRowNamesTheProposalItDeclined(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.propose(w)

	if _, err := fx.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if p := fx.proposal(w, 1); p.Status != store.ProposalDeclined {
		t.Fatalf("proposal = %+v", p)
	}
	rows := fx.activity(w)
	last := rows[len(rows)-1]
	if last.Kind != store.ActivityWatchStopped || !strings.HasSuffix(last.Summary, "; proposal 1 declined") {
		t.Fatalf("stop row = %+v", last)
	}
}

func TestTheNextProposalMarksTheCommitTheAuthorKeptOff(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.propose(w)
	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{RejectPush: true}); err != nil {
		t.Fatal(err)
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit("w1", "w2")
	fx.hook(w, agent.EventStop, `{}`)

	d, err := fx.svc.Proposal(ctx, w.ID, 2, CodeQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Commits) != 2 || !slices.Equal(d.HeldBack, []string{"w1"}) {
		t.Fatalf("proposal 2: commits %+v, held back %v", d.Commits, d.HeldBack)
	}
}

func TestAProposalWhoseCodeCannotBeReadSaysSo(t *testing.T) {
	t.Parallel()
	breaks := map[string]func(f *fakeRelease, err error){
		"history": func(f *fakeRelease, err error) { f.logErr = err },
		"files":   func(f *fakeRelease, err error) { f.filesErr = err },
		"diff":    func(f *fakeRelease, err error) { f.diffErr = err },
	}
	for name, set := range breaks {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			ctx := context.Background()
			w := fx.startManual()
			fx.propose(w)
			fx.rel.set(func(f *fakeRelease) { set(f, errors.New("git: exit status 128")) })

			d, err := fx.svc.Proposal(ctx, w.ID, 1, CodeQuery{})
			if err != nil {
				t.Fatalf("Proposal() error = %v", err)
			}
			if !strings.Contains(d.CodeError, "exit status 128") || len(d.Commits) != 0 || len(d.Files) != 0 || d.Diff != "" {
				t.Fatalf("detail with unreadable code = %+v", d)
			}
			if d.Status != store.ProposalPending || len(d.Replies) != 1 {
				t.Fatalf("the metadata went missing with the code: %+v", d)
			}

			fx.rel.set(func(f *fakeRelease) { set(f, nil) })
			d, err = fx.svc.Proposal(ctx, w.ID, 1, CodeQuery{})
			if err != nil {
				t.Fatalf("Proposal() error = %v", err)
			}
			if d.CodeError != "" || len(d.Commits) != 1 || len(d.Files) != 1 || d.Diff == "" {
				t.Fatalf("detail after the recovery = %+v", d)
			}
		})
	}
}
