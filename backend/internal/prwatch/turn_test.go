package prwatch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func (fx *fixture) hook(w store.Watch, event, payload string) {
	fx.t.Helper()
	if err := fx.svc.Hook(context.Background(), w.ID, event, []byte(payload)); err != nil {
		fx.t.Fatalf("Hook(%s) error = %v", event, err)
	}
	fx.svc.wg.Wait()
}

func (fx *fixture) comment() {
	fx.update(func() {
		fx.pr.ReviewComments = []ghfake.ReviewComment{{ID: 31, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:05:00Z"), Body: "rename this", URL: "https://c/31", Path: "x.go", Line: 4}}
	})
}

func (fx *fixture) turn(w store.Watch) {
	fx.t.Helper()
	fx.comment()
	fx.poll(w)
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
}

func (fx *fixture) proposals(w store.Watch) []store.Proposal {
	fx.t.Helper()
	ps, err := fx.st.ListProposals(context.Background(), w.ID)
	if err != nil {
		fx.t.Fatal(err)
	}
	return ps
}

func (fx *fixture) proposal(w store.Watch, n int) store.Proposal {
	fx.t.Helper()
	p, err := fx.st.GetProposal(context.Background(), w.ID, n)
	if err != nil {
		fx.t.Fatal(err)
	}
	return p
}

func (fx *fixture) reply(w store.Watch, inReplyTo int64, body string) ReplyOutcome {
	fx.t.Helper()
	out, err := fx.svc.Reply(context.Background(), w.ID, ReplyRequest{InReplyTo: inReplyTo, Body: body})
	if err != nil {
		fx.t.Fatalf("Reply() error = %v", err)
	}
	return out
}

func TestTheDaemonPushesAndPostsWhenTheTurnEnds(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.turn(w)
	told := len(fx.host.last().messages())

	fx.rel.commit("abc", "w1")
	out := fx.reply(w, 31, "renamed it, go test ./... passes")
	if out.Posted != nil || out.Proposal != 1 {
		t.Fatalf("Reply() = %+v, want a reply recorded on proposal 1", out)
	}
	if got := fx.posted(); len(got) != 0 || len(fx.rel.pushed()) != 0 {
		t.Fatalf("something went out before the turn ended: posted %q, pushed %v", got, fx.rel.pushed())
	}
	p := fx.proposal(w, 1)
	if p.Status != store.ProposalOpen || p.HeadSHA != "abc" || p.BaseSHA != "abc" {
		t.Fatalf("open proposal = %+v", p)
	}

	fx.hook(w, agent.EventStop, `{}`)
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "w1", Branch: "fix"}}) {
		t.Fatalf("pushes = %+v, want w1 to fix with no lease", got)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"31:renamed it, go test ./... passes"}) {
		t.Fatalf("posted = %q", got)
	}
	p = fx.proposal(w, 1)
	if p.Status != store.ProposalReleased || p.WorkSHA != "w1" || !p.HasPush || p.ReleasedAt == nil {
		t.Fatalf("released proposal = %+v", p)
	}
	seen, err := fx.st.SeenReviewItems(ctx, w.Key())
	if err != nil || !seen[store.SeenItem{Kind: store.KindReviewComment, ID: fx.newestComment().ID}] {
		t.Fatalf("the posted reply is not seen: %v, %v", seen, err)
	}
	if kinds := fx.kinds(w); kinds[len(kinds)-1] != "replied" {
		t.Fatalf("kinds = %v", kinds)
	}
	if n := len(fx.host.last().messages()); n != told {
		t.Fatalf("the agent heard about the release: %d messages, want %d", n, told)
	}
}

func TestAQuestionToTheAuthorKeepsTheTurnOpen(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")

	fx.hook(w, agent.EventNotification, `{"notification_type":"agent_needs_input"}`)
	if p := fx.proposal(w, 1); p.Status != store.ProposalOpen || len(fx.rel.pushed()) != 0 {
		t.Fatalf("a question ended the turn: %+v, pushes %v", p, fx.rel.pushed())
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)
	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased || len(fx.rel.pushed()) != 1 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
}

func TestATurnThatLeavesNothingLeavesNoProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	if ps := fx.proposals(w); len(ps) != 1 || ps[0].Status != store.ProposalOpen {
		t.Fatalf("the opening message opened no turn: %+v", ps)
	}
	fx.hook(w, agent.EventStop, `{}`)
	if ps := fx.proposals(w); len(ps) != 0 {
		t.Fatalf("an empty turn left %+v", ps)
	}

	fx.turn(w)
	fx.hook(w, agent.EventStop, `{}`)
	if ps := fx.proposals(w); len(ps) != 0 || len(fx.rel.pushed()) != 0 {
		t.Fatalf("a turn with no commit and no reply left %+v and pushed %v", ps, fx.rel.pushed())
	}
}

func TestCommitsTheAuthorHeldBackComeBackInNoProposal(t *testing.T) {
	t.Parallel()
	for name, decide := range map[string]func(fx *fixture, w store.Watch) error{
		"approve without the push": func(fx *fixture, w store.Watch) error {
			_, err := fx.svc.Approve(context.Background(), w.ID, 1, Decision{RejectPush: true})
			return err
		},
		"approve without the push and stop asking": func(fx *fixture, w store.Watch) error {
			_, err := fx.svc.Approve(context.Background(), w.ID, 1, Decision{RejectPush: true, StopAsking: true})
			return err
		},
		"reject and keep the commits": func(fx *fixture, w store.Watch) error {
			_, err := fx.svc.Reject(context.Background(), w.ID, 1, Rejection{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			w := fx.startManual()
			fx.propose(w)
			if err := decide(fx, w); err != nil {
				t.Fatal(err)
			}
			fx.hook(w, agent.EventUserPromptSubmit, `{}`)
			fx.hook(w, agent.EventStop, `{}`)
			if ps := fx.proposals(w); len(ps) != 1 || len(fx.rel.pushed()) != 0 {
				t.Fatalf("the commits the author held back came back: proposals %+v, pushes %v", ps, fx.rel.pushed())
			}
		})
	}
}

func TestACommitTheAuthorHeldBackDoesNotGoOutWithTheNextTurn(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.startManual()
	fx.propose(w)
	if _, err := fx.svc.Approve(ctx, w.ID, 1, Decision{RejectPush: true, StopAsking: true}); err != nil {
		t.Fatal(err)
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit("w1", "w2")
	fx.hook(w, agent.EventStop, `{}`)
	for _, p := range fx.rel.pushed() {
		if heldBack, _ := fx.rel.Contains(ctx, "", p.SHA, "w1"); heldBack {
			t.Fatalf("the push of the next turn carries w1, which the author kept off: pushes %v, proposals %+v", fx.rel.pushed(), fx.proposals(w))
		}
	}
}

func TestACommitMadeWhileTheTurnStartsIsTheWorkOfTheTurn(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.comment()
	fx.poll(w)
	fx.rel.set(func(f *fakeRelease) {
		f.duringFetch = func() { fx.rel.commit("abc", "w1") }
	})
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.reply(w, 31, "Fixed in w1")
	fx.hook(w, agent.EventStop, `{}`)
	if p := fx.proposal(w, 1); !p.Pushes() {
		t.Fatalf("the commit of the turn is not in its proposal: %+v", p)
	}
}

func TestATurnOnAWorkBranchBehindTheHeadOffersNothing(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.hook(w, agent.EventStop, `{}`)
	fx.rel.moveRemote("abc", "t1")

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)
	if ps := fx.proposals(w); len(ps) != 0 {
		t.Fatalf("a turn that committed nothing offered %+v", ps)
	}
}

func TestAnAuthorMessageStartsATurn(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.hook(w, agent.EventStop, `{}`)
	if _, err := fx.svc.Send(ctx, w.ID, "use a table test"); err != nil {
		t.Fatal(err)
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit("abc", "w1")
	fx.hook(w, agent.EventStop, `{}`)
	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased || len(fx.rel.pushed()) != 1 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
}

func TestATurnThatEndsBeforeTheSendReturnsIsClosed(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.hook(w, agent.EventStop, `{}`)
	h := fx.host.last()
	h.mu.Lock()
	h.onSend = func() {
		if err := fx.svc.Hook(ctx, w.ID, agent.EventUserPromptSubmit, []byte(`{}`)); err != nil {
			t.Error(err)
		}
		fx.rel.commit("abc", "w1")
		if err := fx.svc.Hook(ctx, w.ID, agent.EventStop, []byte(`{}`)); err != nil {
			t.Error(err)
		}
	}
	h.mu.Unlock()

	if _, err := fx.svc.Send(ctx, w.ID, "use a table test"); err != nil {
		t.Fatal(err)
	}
	fx.svc.wg.Wait()
	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased || len(fx.rel.pushed()) != 1 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
}

func TestASendThatFailsKeepsTheTurnThatEnded(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.hook(w, agent.EventStop, `{}`)
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit("abc", "w1")
	release := make(chan struct{})
	fx.svc.spawn(fx.svc.turns.queue(w.ID, func() { <-release }))
	if err := fx.svc.Hook(ctx, w.ID, agent.EventStop, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	h := fx.host.last()
	h.mu.Lock()
	h.sendErr = errors.New("terminal gone")
	h.mu.Unlock()

	if _, err := fx.svc.Send(ctx, w.ID, "use a table test"); err == nil {
		t.Fatal("Send() error = nil, want the error of the terminal")
	}
	close(release)
	fx.svc.wg.Wait()
	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased || len(fx.rel.pushed()) != 1 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
}

func TestTheWorkBranchCatchesUpBeforeTheMessage(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.hook(w, agent.EventStop, `{}`)
	fx.rel.moveRemote("abc", "t1")

	fx.turn(w)
	if !slices.Equal(fx.rel.ffs, []string{"t1"}) {
		t.Fatalf("fast-forwards = %v", fx.rel.ffs)
	}
	if p := fx.proposal(w, 1); p.HeadSHA != "t1" || p.BaseSHA != "t1" {
		t.Fatalf("proposal = %+v, want it on t1", p)
	}
	fx.rel.commit("t1", "w1")
	fx.hook(w, agent.EventStop, `{}`)
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "w1", Branch: "fix"}}) {
		t.Fatalf("pushes = %+v", got)
	}
}

func TestARewriteGoesOutWithTheLeasePinnedToTheHeadOfTheTurn(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.set(func(f *fakeRelease) {
		f.history["r1"] = []string{"base"}
		f.work = "r1"
	})
	fx.hook(w, agent.EventStop, `{}`)
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "r1", Branch: "fix", Lease: "abc"}}) {
		t.Fatalf("pushes = %+v, want r1 with the lease on abc", got)
	}
}

func TestARewriteLeasesTheHeadItWasCheckedAgainst(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) {
		f.history["r1"] = []string{"base"}
		f.work = "r1"
	})
	fx.hook(w, agent.EventStop, `{}`)
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "r1", Branch: "fix", Lease: "t1"}}) {
		t.Fatalf("pushes = %+v, want r1 with the lease on t1", got)
	}
	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased {
		t.Fatalf("proposal = %+v", p)
	}
}

func TestATurnThatOnlyAddedCommitsIsRebasedOntoTheHeadThatMoved(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.reply(w, 31, "renamed it")
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) { f.missing = []string{"t1"} })

	fx.hook(w, agent.EventStop, `{}`)
	if !slices.Equal(fx.rel.rebases, []string{"t1"}) {
		t.Fatalf("rebases = %v", fx.rel.rebases)
	}
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "w1-on-t1", Branch: "fix"}}) {
		t.Fatalf("pushes = %+v", got)
	}
	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased || p.HeadSHA != "t1" || p.WorkSHA != "w1-on-t1" {
		t.Fatalf("proposal = %+v", p)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"31:renamed it"}) {
		t.Fatalf("posted = %q", got)
	}
}

func TestARebaseOfTheReleaseThatConflictsHandsTheWorkToTheAgent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) {
		f.missing = []string{"t1"}
		f.rebaseErr = &gitrelease.ConflictError{Files: []string{"x.go"}}
	})
	told := len(fx.host.last().messages())

	fx.hook(w, agent.EventStop, `{}`)
	if p := fx.proposal(w, 1); p.Status != store.ProposalFailed || len(fx.rel.pushed()) != 0 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
	msgs := fx.host.last().messages()
	if len(msgs) != told+1 || !strings.Contains(msgs[told], "x.go") {
		t.Fatalf("messages = %q", msgs[told:])
	}
}

func TestARetryOfARewriteHandsTheWorkToTheAgent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) {
		f.history["r1"] = []string{"main2", "base"}
		f.work = "r1"
		f.missing = []string{"t1"}
	})
	fx.hook(w, agent.EventStop, `{}`)
	told := len(fx.host.last().messages())
	if len(fx.rel.rebases) != 0 {
		t.Fatalf("the release rebased a rewrite: %v", fx.rel.rebases)
	}

	p, err := fx.svc.Retry(context.Background(), w.ID, 1)
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if p.Status != store.ProposalFailed || len(fx.rel.rebases) != 0 || len(fx.rel.pushed()) != 0 {
		t.Fatalf("proposal = %+v, rebases %v, pushes %v", p, fx.rel.rebases, fx.rel.pushed())
	}
	msgs := fx.host.last().messages()
	if len(msgs) != told+1 || !strings.Contains(msgs[told], "t1") || !strings.Contains(msgs[told], "lacks") {
		t.Fatalf("messages = %q", msgs[told:])
	}
}

func TestARewriteThatDropsACommitOfTheHeadFails(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.set(func(f *fakeRelease) {
		f.history["r1"] = []string{"base"}
		f.work = "r1"
		f.missing = []string{"t1"}
	})
	fx.reply(w, 31, "renamed it")
	fx.hook(w, agent.EventStop, `{}`)

	if len(fx.rel.pushed()) != 0 || len(fx.posted()) != 0 {
		t.Fatalf("pushed %v and posted %q", fx.rel.pushed(), fx.posted())
	}
	p := fx.proposal(w, 1)
	if p.Status != store.ProposalFailed || !strings.Contains(p.Error, "t1") {
		t.Fatalf("proposal = %+v", p)
	}
	rows := fx.activity(w)
	last := rows[len(rows)-1]
	if last.Kind != store.ActivityAgentFailed || !strings.Contains(last.Summary, "push proposal 1") ||
		!strings.Contains(last.Summary, "babysitter watch retry 1 1") || !strings.Contains(string(last.Payload), `"proposal":1`) {
		t.Fatalf("failure row = %+v", last)
	}
	if kinds := fx.notes.noteKinds(); kinds[len(kinds)-1] != "watch" {
		t.Fatalf("notification kinds = %v", kinds)
	}
}

func TestARetryRebasesOntoTheHeadThatMoved(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.reply(w, 31, "renamed it")
	fx.rel.set(func(f *fakeRelease) { f.pushErr = gitrelease.ErrLeaseRefused })
	fx.hook(w, agent.EventStop, `{}`)
	if p := fx.proposal(w, 1); p.Status != store.ProposalFailed || len(fx.posted()) != 0 {
		t.Fatalf("proposal = %+v, posted %q", p, fx.posted())
	}

	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) {
		f.pushErr = nil
		f.missing = []string{"t1"}
	})
	p, err := fx.svc.Retry(ctx, w.ID, 1)
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if p.Status != store.ProposalReleased || p.HeadSHA != "t1" || p.WorkSHA != "w1-on-t1" {
		t.Fatalf("retried proposal = %+v", p)
	}
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "w1-on-t1", Branch: "fix"}}) {
		t.Fatalf("pushes = %+v", got)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"31:renamed it"}) {
		t.Fatalf("posted = %q", got)
	}
	if _, err := fx.svc.Retry(ctx, w.ID, 1); !errors.Is(err, ErrNothingToRetry) {
		t.Fatalf("a second Retry() error = %v, want ErrNothingToRetry", err)
	}
	if _, err := fx.svc.Retry(ctx, w.ID, 9); !errors.Is(err, store.ErrProposalNotFound) {
		t.Fatalf("Retry() of a missing proposal error = %v", err)
	}
}

func TestARetryPushesAfterACredentialFailure(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.rel.set(func(f *fakeRelease) {
		f.pushErr = errors.New("could not read Username for 'https://github.com': terminal prompts disabled")
	})
	fx.hook(w, agent.EventStop, `{}`)
	if p := fx.proposal(w, 1); p.Status != store.ProposalFailed || !strings.Contains(p.Error, "terminal prompts disabled") {
		t.Fatalf("proposal = %+v", p)
	}
	fx.rel.set(func(f *fakeRelease) { f.pushErr = nil })
	p, err := fx.svc.Retry(context.Background(), w.ID, 1)
	if err != nil || p.Status != store.ProposalReleased || len(fx.rel.rebases) != 0 {
		t.Fatalf("Retry() = %+v, %v, rebases %v", p, err, fx.rel.rebases)
	}
}

func TestARetryThatConflictsHandsTheWorkToTheAgent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.reply(w, 31, "renamed it")
	fx.rel.set(func(f *fakeRelease) { f.pushErr = gitrelease.ErrLeaseRefused })
	fx.hook(w, agent.EventStop, `{}`)
	told := len(fx.host.last().messages())

	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) {
		f.pushErr = nil
		f.missing = []string{"t1"}
		f.rebaseErr = &gitrelease.ConflictError{Files: []string{"x.go"}}
	})
	fx.advance(time.Minute)
	p, err := fx.svc.Retry(ctx, w.ID, 1)
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if p.Status != store.ProposalFailed || len(fx.rel.pushed()) != 0 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
	msgs := fx.host.last().messages()
	if len(msgs) != told+1 || !strings.Contains(msgs[told], "could not push your work") || !strings.Contains(msgs[told], "x.go") {
		t.Fatalf("messages = %q", msgs[told:])
	}
	fx.conflictNamesNoRetry(w)

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.set(func(f *fakeRelease) {
		f.rebaseErr = nil
		f.missing = nil
		f.history["w2"] = []string{"t1", "abc"}
		f.work = "w2"
	})
	fx.hook(w, agent.EventStop, `{}`)
	if old := fx.proposal(w, 1); old.Status != store.ProposalSuperseded {
		t.Fatalf("old proposal = %+v", old)
	}
	if _, err := fx.svc.Retry(ctx, w.ID, 1); !errors.Is(err, ErrNothingToRetry) || !strings.Contains(err.Error(), "the next turn of the agent took its work over") {
		t.Fatalf("a retry of the superseded proposal error = %v", err)
	}
	next := fx.proposal(w, 2)
	if next.Status != store.ProposalReleased || next.HeadSHA != "t1" {
		t.Fatalf("next proposal = %+v", next)
	}
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "w2", Branch: "fix"}}) {
		t.Fatalf("pushes = %+v", got)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"31:renamed it"}) {
		t.Fatalf("posted = %q", got)
	}
}

func TestADependabotTurnReleasesRepliesOnly(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.Author = "dependabot[bot]" })
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.reply(w, 0, "@dependabot rebase")
	fx.hook(w, agent.EventStop, `{}`)

	if len(fx.rel.pushed()) != 0 || len(fx.rel.rebases) != 0 {
		t.Fatalf("the daemon touched the branch of the bot: pushes %v, rebases %v", fx.rel.pushed(), fx.rel.rebases)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"0:@dependabot rebase"}) {
		t.Fatalf("posted = %q", got)
	}
	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased || p.HasPush {
		t.Fatalf("proposal = %+v", p)
	}
}

func TestADependabotWorktreeFollowsTheBranchOfTheBot(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.Author = "dependabot[bot]" })
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.hook(w, agent.EventStop, `{}`)
	fx.rel.moveRemote("base", "b2")

	if _, err := fx.svc.Send(context.Background(), w.ID, "check the build again"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fx.rel.resets, []string{"b2"}) || fx.rel.work != "b2" {
		t.Fatalf("resets = %v, work = %s, want the worktree on b2", fx.rel.resets, fx.rel.work)
	}
	if len(fx.rel.pushed()) != 0 {
		t.Fatalf("pushes = %v", fx.rel.pushed())
	}
}

func TestStopDeclinesTheOpenProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	if _, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if p := fx.proposal(w, 1); p.Status != store.ProposalDeclined || len(fx.rel.pushed()) != 0 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
}

func TestRecoverClosesATurnTheShutdownCutShort(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")

	fx.svc = fx.newService()
	fx.svc.recover(context.Background())
	fx.svc.wg.Wait()
	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased || len(fx.rel.pushed()) != 1 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
}

func TestAnIdleSignalOlderThanTheLastMessageEndsNoTurn(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	l := fx.svc.sessions.get(w.ID)
	stale := l.turnSeq() - 1
	l.report(agent.StateIdle, fx.clock())

	fx.svc.endTurn(w.ID, stale)
	if p := fx.proposal(w, 1); p.Status != store.ProposalOpen || len(fx.rel.pushed()) != 0 {
		t.Fatalf("proposal = %+v, pushes %v", p, fx.rel.pushed())
	}
	fx.svc.endTurn(w.ID, l.turnSeq())
	if p := fx.proposal(w, 1); p.Status != store.ProposalReleased {
		t.Fatalf("proposal = %+v", p)
	}
}

func TestASessionThatExitsEndsTheTurn(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.host.last().exit(errors.New("killed"))
	if testutil.Within(testutil.Timeout, func() bool { return fx.proposal(w, 1).Status == store.ProposalReleased }) {
		return
	}
	t.Fatalf("proposal = %+v, pushes %v", fx.proposal(w, 1), fx.rel.pushed())
}

func TestTheEndOfATurnWaitsForItsStart(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.hook(w, agent.EventStop, `{}`)
	for i := range 20 {
		unlock := fx.svc.locks.lock(w.ID)
		if err := fx.svc.Hook(ctx, w.ID, agent.EventUserPromptSubmit, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
		fx.rel.commit(fx.rel.work, fmt.Sprintf("w%d", i))
		if err := fx.svc.Hook(ctx, w.ID, agent.EventStop, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
		unlock()
		fx.svc.wg.Wait()
		if p := fx.proposal(w, i+1); p.Status != store.ProposalReleased {
			t.Fatalf("turn %d left %+v", i+1, p)
		}
	}
	fx.svc.turns.mu.Lock()
	defer fx.svc.turns.mu.Unlock()
	if n := len(fx.svc.turns.tail); n != 0 {
		t.Fatalf("%d queues outlived their jobs", n)
	}
}

func TestTheNextTurnRebasesTheWorkOfAFailedTurn(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.rel.set(func(f *fakeRelease) { f.pushErr = errors.New("Could not resolve host: github.com") })
	fx.hook(w, agent.EventStop, `{}`)
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) {
		f.pushErr = nil
		f.missing = []string{"t1"}
	})

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)
	if got := fx.rel.pushed(); !slices.Equal(got, []gitrelease.Push{{SHA: "w1-on-t1", Branch: "fix"}}) {
		t.Fatalf("pushes = %+v, rebases %v", got, fx.rel.rebases)
	}
	if p := fx.proposal(w, 2); p.Status != store.ProposalReleased {
		t.Fatalf("proposal = %+v", p)
	}
}

func TestWorkHandedToTheAgentIsHandedOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) {
		f.missing = []string{"t1"}
		f.rebaseErr = &gitrelease.ConflictError{Files: []string{"x.go"}}
	})
	fx.hook(w, agent.EventStop, `{}`)
	told := len(fx.host.last().messages())

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)
	if msgs := fx.host.last().messages(); len(msgs) != told {
		t.Fatalf("the work went back again: %q", msgs[told:])
	}
	if p := fx.proposal(w, 2); p.Status != store.ProposalFailed || p.HeadSHA != "t1" {
		t.Fatalf("proposal = %+v", p)
	}
}

func TestATurnWithAMergeIsNotRebased(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "m1")
	fx.rel.commit("m1", "w1")
	fx.rel.moveRemote("abc", "t1")
	fx.rel.set(func(f *fakeRelease) {
		f.merges = []string{"m1"}
		f.missing = []string{"t1"}
	})
	fx.hook(w, agent.EventStop, `{}`)
	if len(fx.rel.rebases) != 0 || len(fx.rel.pushed()) != 0 {
		t.Fatalf("rebases %v, pushes %v", fx.rel.rebases, fx.rel.pushed())
	}
	told := len(fx.host.last().messages())
	if _, err := fx.svc.Retry(context.Background(), w.ID, 1); err != nil {
		t.Fatal(err)
	}
	if len(fx.rel.rebases) != 0 {
		t.Fatalf("the retry rebased a merge: %v", fx.rel.rebases)
	}
	if msgs := fx.host.last().messages(); len(msgs) != told+1 || !strings.Contains(msgs[told], "lacks") {
		t.Fatalf("messages = %q", msgs[told:])
	}
}

func TestARetryThatRacesAStopSaysTheWatchStopped(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.rel.set(func(f *fakeRelease) { f.pushErr = errors.New("Could not resolve host: github.com") })
	fx.hook(w, agent.EventStop, `{}`)
	fx.rel.set(func(f *fakeRelease) { f.pushErr = nil })

	unlock := fx.svc.locks.lock(w.ID)
	done := make(chan error)
	go func() {
		_, err := fx.svc.Retry(ctx, w.ID, 1)
		done <- err
	}()
	testutil.Eventually(t, func() bool { return fx.lockUsers(w.ID) >= 2 }, "Retry to wait on the lock of the watch")
	if _, err := fx.svc.stop(ctx, w.ID, store.StopUser, "", StopOptions{KeepWorktree: new(true)}); err != nil {
		t.Fatal(err)
	}
	unlock()

	if err := <-done; !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("Retry() error = %v, want ErrWatchStopped", err)
	}
	if len(fx.rel.pushed()) != 0 {
		t.Fatalf("pushes = %v", fx.rel.pushed())
	}
}

func (fx *fixture) lockUsers(id int64) int {
	fx.svc.locks.mu.Lock()
	defer fx.svc.locks.mu.Unlock()
	if l, ok := fx.svc.locks.m[id]; ok {
		return l.users
	}
	return 0
}
