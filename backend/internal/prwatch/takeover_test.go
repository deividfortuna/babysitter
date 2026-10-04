package prwatch

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func (fx *fixture) takeover(w store.Watch) Takeover {
	fx.t.Helper()
	tk, err := fx.svc.Takeover(context.Background(), w.ID, TakeoverOptions{PID: 4242})
	if err != nil {
		fx.t.Fatalf("Takeover() error = %v", err)
	}
	return tk
}

func (fx *fixture) settledKinds(w store.Watch) []string {
	fx.t.Helper()
	testutil.Settle(fx.t, func() int { return len(fx.activity(w)) }, "the activity of the watch")
	return fx.kinds(w)
}

func TestATakeoverEndsTheSessionAndGivesTheCommandOfTheAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()

	tk := fx.takeover(w)

	if !h.wasStopped() {
		t.Fatal("the session of the daemon still runs")
	}
	w = fx.watch(w)
	if w.TakenOverAt == nil || !w.TakenOverAt.Equal(fx.clock()) || w.TakenOverPID != 4242 {
		t.Fatalf("watch = %+v", w)
	}
	want := []string{"fake-agent", "--resume", w.AgentSession}
	if !slices.Equal(tk.Argv, want) || tk.NewConversation {
		t.Fatalf("argv = %v, new %v, want %v", tk.Argv, tk.NewConversation, want)
	}
	if tk.WorktreeDir != w.WorktreeDir || tk.WorkBranch != w.WorkBranch || tk.HeadRef != "fix" {
		t.Fatalf("takeover = %+v", tk)
	}
	kinds := fx.settledKinds(w)
	if slices.Contains(kinds, string(store.ActivitySessionExited)) || kinds[len(kinds)-1] != string(store.ActivityTakenOver) {
		t.Fatalf("kinds = %v", kinds)
	}
	row := fx.activity(w)[len(kinds)-1]
	var payload map[string]any
	if err := json.Unmarshal(row.Payload, &payload); err != nil || payload["by"] != "author" {
		t.Fatalf("payload = %s, %v", row.Payload, err)
	}
	if info, err := fx.svc.Session(context.Background(), w); err != nil || info.State != agent.StateNone {
		t.Fatalf("session = %+v, %v", info, err)
	}
}

func TestATakeoverAsksForAPollSoTheBlockerShowsAtOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	for len(fx.svc.kick) > 0 {
		<-fx.svc.kick
	}

	fx.takeover(w)

	if len(fx.svc.kick) != 1 {
		t.Fatal("the takeover asked for no poll")
	}
}

func TestATakeoverInTheMiddleOfATurnDeclinesItsWork(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.reply(w, 31, "renamed it")

	tk := fx.takeover(w)

	if p := fx.proposal(w, 1); p.Status != store.ProposalDeclined {
		t.Fatalf("proposal = %+v", p)
	}
	if !slices.Equal(tk.Declined, []int{1}) {
		t.Fatalf("declined = %v", tk.Declined)
	}
	if len(fx.rel.pushed()) != 0 || len(fx.posted()) != 0 {
		t.Fatalf("pushes %v, posts %v", fx.rel.pushed(), fx.posted())
	}
	rows := fx.activity(w)
	if last := rows[len(rows)-1]; !strings.HasSuffix(last.Summary, "; proposal 1 declined") {
		t.Fatalf("taken_over row = %q", last.Summary)
	}
}

func TestATakeoverDeclinesThePendingProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.propose(w)

	fx.takeover(w)

	if p := fx.proposal(w, 1); p.Status != store.ProposalDeclined {
		t.Fatalf("proposal = %+v", p)
	}
	if _, err := fx.svc.Approve(context.Background(), w.ID, 1, Decision{}); !errors.Is(err, ErrNotPending) {
		t.Fatalf("Approve() error = %v, want ErrNotPending", err)
	}
	if len(fx.rel.pushed()) != 0 {
		t.Fatalf("pushes = %v", fx.rel.pushed())
	}
}

func TestATakeoverOfAWatchWithNoConversationStartsOne(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	if err := fx.st.SetWatchAgentSession(context.Background(), w.ID, ""); err != nil {
		t.Fatal(err)
	}

	tk := fx.takeover(w)

	w = fx.watch(w)
	if w.AgentSession == "" || !tk.NewConversation || !slices.Equal(tk.Argv, []string{"fake-agent", "--session-id", w.AgentSession}) {
		t.Fatalf("argv = %v, new %v, session %q", tk.Argv, tk.NewConversation, w.AgentSession)
	}
}

func TestAShellTakeoverOfAWatchWithNoConversationStartsNone(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	if err := fx.st.SetWatchAgentSession(context.Background(), w.ID, ""); err != nil {
		t.Fatal(err)
	}

	tk, err := fx.svc.Takeover(context.Background(), w.ID, TakeoverOptions{PID: 4242, Shell: true})
	if err != nil {
		t.Fatalf("Takeover() error = %v", err)
	}
	if got := fx.watch(w).AgentSession; got != "" || tk.NewConversation {
		t.Fatalf("session %q, new %v", got, tk.NewConversation)
	}

	fx.handback(w, HandbackOptions{})

	if argv := fx.host.last().spec.Argv; slices.Contains(argv, "--resume") {
		t.Fatalf("the hand-back resumed a conversation that nobody started: %v", argv)
	}
}

func TestATakeoverIsRefusedWhenTheSessionOfTheDaemonDoesNotStop(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.svc.wg.Wait()
	h := fx.host.last()
	h.mu.Lock()
	h.stopErr = context.DeadlineExceeded
	h.mu.Unlock()

	_, err := fx.svc.Takeover(context.Background(), w.ID, TakeoverOptions{PID: 4242})

	if !errors.Is(err, ErrSessionRunning) {
		t.Fatalf("Takeover() error = %v, want ErrSessionRunning", err)
	}
	if fx.watch(w).TakenOverAt != nil {
		t.Fatal("the refused takeover gave the session to the author")
	}
	if fx.svc.sessions.get(w.ID) == nil {
		t.Fatal("the daemon lost the session that still runs")
	}
	if fx.lastKind(w, store.ActivityTakenOver).ID != 0 {
		t.Fatal("the refused takeover recorded a taken_over row")
	}
}

func (fx *fixture) refusedTakeover(w store.Watch) {
	fx.t.Helper()
	if _, err := fx.svc.Takeover(context.Background(), w.ID, TakeoverOptions{PID: 4242}); err == nil {
		fx.t.Fatal("Takeover() gave no error")
	}
	if got := fx.watch(w); got.TakenOverAt != nil {
		fx.t.Fatalf("the failed takeover gave the session to the author: %+v", got)
	}
}

func TestARefusedTakeoverDeclinesNoProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	ctx := context.Background()
	failed, err := fx.st.OpenProposal(ctx, w.ID, "abc", "abc", "abc", fx.clock())
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.st.SetProposalOutcome(ctx, failed.ID, store.ProposalFailed, "push rejected", fx.clock()); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.st.OpenProposal(ctx, w.ID, "abc", "abc", "abc", fx.clock()); err != nil {
		t.Fatal(err)
	}
	fx.refuse("second_decline_refused", "UPDATE OF status ON proposals WHEN NEW.status = 'declined' AND NEW.number = 2")

	fx.refusedTakeover(w)

	if p := fx.proposal(w, 1); p.Status != store.ProposalFailed {
		t.Fatalf("the refused takeover declined proposal 1: %+v", p)
	}
}

func TestATakeoverThatFailsToDeclineLeavesTheSessionOfTheDaemonRunning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		start func(fx *fixture) store.Watch
	}{
		{name: "an open turn", start: func(fx *fixture) store.Watch {
			w := fx.start()
			fx.hook(w, agent.EventUserPromptSubmit, `{}`)
			return w
		}},
		{name: "a proposal that waits for the author", start: func(fx *fixture) store.Watch {
			w := fx.startManual()
			fx.propose(w)
			return w
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			w := c.start(fx)
			fx.refuse("decline_refused", "UPDATE OF status ON proposals WHEN NEW.status = 'declined'")

			fx.refusedTakeover(w)

			if fx.svc.sessions.get(w.ID) == nil || fx.host.last().wasStopped() {
				t.Fatal("the refused takeover stopped the session of the daemon")
			}
		})
	}
}

func TestATakeoverThatFailsToBuildTheCommandChangesNothing(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.svc.wg.Wait()
	if err := fx.st.SetWatchAgentSession(context.Background(), w.ID, ""); err != nil {
		t.Fatal(err)
	}
	fx.svc.agents[ProviderClaude].(*fakeRunner).authorErr = errors.New("the trust of the worktree cannot be written")

	fx.refusedTakeover(w)

	if got := fx.watch(w).AgentSession; got != "" {
		t.Fatalf("the failed takeover stored the conversation %q that nobody started", got)
	}
}

func TestATakeoverThatFailsToStoreTheConversationIsUndone(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	if err := fx.st.SetWatchAgentSession(context.Background(), w.ID, ""); err != nil {
		t.Fatal(err)
	}
	fx.refuse("agent_session_refused", "UPDATE OF agent_session ON watches WHEN NEW.agent_session != ''")

	fx.refusedTakeover(w)
}

func TestATakeoverWhoseSessionDoesNotStopDeclinesNoProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.startManual()
	fx.propose(w)
	h := fx.host.last()
	h.mu.Lock()
	h.stopErr = context.DeadlineExceeded
	h.mu.Unlock()

	if _, err := fx.svc.Takeover(context.Background(), w.ID, TakeoverOptions{PID: 4242}); !errors.Is(err, ErrSessionRunning) {
		t.Fatalf("Takeover() error = %v, want ErrSessionRunning", err)
	}
	if p := fx.proposal(w, 1); p.Status != store.ProposalPending {
		t.Fatalf("the refused takeover declined proposal 1: %+v", p)
	}
}

func TestAShellTakeoverNeedsNoAgent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.svc.wg.Wait()
	delete(fx.svc.agents, ProviderClaude)

	if _, err := fx.svc.Takeover(context.Background(), w.ID, TakeoverOptions{PID: 4242, Shell: true}); err != nil {
		t.Fatalf("Takeover() error = %v", err)
	}
	if fx.watch(w).TakenOverAt == nil {
		t.Fatal("the watch is not taken over")
	}
}

func TestATurnQueuedBeforeATakeoverOpensNoProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)

	fx.svc.startTurn(w.ID, "abc")

	if p, open, err := fx.st.ActiveProposal(context.Background(), w.ID); err != nil || open {
		t.Fatalf("a proposal opened while the session is with the author: %+v, err %v", p, err)
	}
}

func TestATakeoverIsRefusedWithAReason(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	self := newFixture(t)
	if _, err := self.svc.Takeover(ctx, self.startSelf().ID, TakeoverOptions{PID: 1}); !errors.Is(err, ErrSelfWatch) {
		t.Fatalf("Takeover() of a self watch = %v, want ErrSelfWatch", err)
	}

	stopped := newFixture(t)
	w := stopped.start()
	if _, err := stopped.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := stopped.svc.Takeover(ctx, w.ID, TakeoverOptions{PID: 1}); !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("Takeover() of a stopped watch = %v, want ErrWatchStopped", err)
	}

	twice := newFixture(t)
	w = twice.start()
	twice.takeover(w)
	if _, err := twice.svc.Takeover(ctx, w.ID, TakeoverOptions{PID: 1}); !errors.Is(err, ErrTakenOver) {
		t.Fatalf("a second Takeover() = %v, want ErrTakenOver", err)
	}
	if got := twice.watch(w).TakenOverPID; got != 4242 {
		t.Fatalf("the refused takeover changed the pid to %d", got)
	}
}

func TestATakeoverWaitsForTheLockOfTheWatch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()

	unlock := fx.svc.locks.Lock(w.ID)
	done := make(chan error)
	go func() {
		_, err := fx.svc.Takeover(context.Background(), w.ID, TakeoverOptions{PID: 4242})
		done <- err
	}()
	testutil.Eventually(t, func() bool { return fx.lockUsers(w.ID) >= 2 }, "Takeover to wait on the lock of the watch")
	if fx.watch(w).TakenOverAt != nil || fx.host.last().wasStopped() {
		t.Fatal("the takeover went on without the lock")
	}
	unlock()

	if err := <-done; err != nil {
		t.Fatalf("Takeover() error = %v", err)
	}
	if fx.watch(w).TakenOverAt == nil {
		t.Fatal("the watch is not taken over")
	}
}

func TestAReviewWhileTheAuthorHasTheSessionStaysUntold(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	sessions := fx.host.count()

	fx.comment()
	fx.poll(w)

	if fx.host.count() != sessions {
		t.Fatalf("the poll started %d sessions", fx.host.count()-sessions)
	}
	var row store.Activity
	for _, a := range fx.activity(w) {
		if a.Kind == store.ActivityReviewComment {
			row = a
		}
	}
	if row.ID == 0 || row.NudgedAt != nil {
		t.Fatalf("review comment row = %+v", row)
	}
	if blockers := fx.watch(w).ReadyBlockers; !slices.Contains(blockers, "the session is with you") {
		t.Fatalf("blockers = %v", blockers)
	}
}

func TestAWatchTakenOverIsNotReadyToMerge(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.takeover(w)

	fx.readyToMerge(w)
	fx.poll(w)

	if slices.Contains(fx.kinds(w), string(store.ActivityMergeReady)) {
		t.Fatalf("kinds = %v", fx.kinds(w))
	}
	if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{}); !errors.Is(err, ErrNotReady) || !strings.Contains(err.Error(), "the session is with you") {
		t.Fatalf("Merge() error = %v, want ErrNotReady that names the session", err)
	}
	if len(fx.merges()) != 0 {
		t.Fatalf("merges = %v", fx.merges())
	}
}

func TestAWatchTakenOverAsksForNoReReview(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.answered()
	w := fx.start()
	fx.takeover(w)

	fx.poll(w)

	if asked := fx.asked(); len(asked) != 0 {
		t.Fatalf("asked = %v", asked)
	}
}

func TestASendWhileTheAuthorHasTheSessionIsRefused(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	sessions := fx.host.count()

	_, err := fx.svc.Send(context.Background(), w.ID, "rename it")
	if !errors.Is(err, ErrTakenOver) || !strings.Contains(err.Error(), "babysitter watch handback") {
		t.Fatalf("Send() error = %v, want ErrTakenOver", err)
	}
	if fx.host.count() != sessions {
		t.Fatal("the send started a session")
	}
}

func TestAHookOfTheSessionOfTheAuthorChangesNothing(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	before := fx.watch(w)
	rows := len(fx.settledKinds(w))

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)

	if ps := fx.proposals(w); len(ps) != 0 {
		t.Fatalf("proposals = %+v", ps)
	}
	if got := len(fx.settledKinds(w)); got != rows {
		t.Fatalf("the hooks recorded %d rows", got-rows)
	}
	if info, _ := fx.svc.Session(context.Background(), w); info.State != agent.StateNone {
		t.Fatalf("session = %+v", info)
	}
	if after := fx.watch(w); after.TakenOverAt == nil || after.AgentSession != before.AgentSession {
		t.Fatalf("watch = %+v", after)
	}
}

func TestAMergeWhileTheAuthorHasTheSessionKeepsTheWorktree(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	removed := len(fx.git.removedDirs())

	fx.update(func() { fx.pr.Merged, fx.pr.State = true, "closed" })
	fx.poll(w)

	got := fx.watch(w)
	if got.Status != store.WatchStopped || got.StopReason != store.StopMerged {
		t.Fatalf("watch = %s (%s)", got.Status, got.StopReason)
	}
	if len(fx.git.removedDirs()) != removed {
		t.Fatalf("removed = %v", fx.git.removedDirs())
	}
	rows := fx.activity(w)
	if last := rows[len(rows)-1]; last.Kind != store.ActivityWatchStopped || !strings.Contains(last.Summary, "worktree kept: the session was with you") {
		t.Fatalf("stop row = %s %q", last.Kind, last.Summary)
	}
}

func TestAStopOfTheUserKeepsTheWorktreeOfTheAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	removed := len(fx.git.removedDirs())

	if _, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{KeepWorktree: new(false)}); err != nil {
		t.Fatal(err)
	}
	if len(fx.git.removedDirs()) != removed {
		t.Fatalf("removed = %v", fx.git.removedDirs())
	}
}

func TestARestartOfTheDaemonLeavesTheSessionWithTheAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	sessions := fx.host.count()

	fx.svc = fx.newService()
	fx.svc.recover(context.Background())
	fx.svc.wg.Wait()

	if fx.host.count() != sessions {
		t.Fatal("the restart started the session of the daemon again")
	}
}
