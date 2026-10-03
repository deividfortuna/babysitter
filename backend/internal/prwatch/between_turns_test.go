package prwatch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestAnItemWaitsForTheOpeningTurnAndGoesWhenItEnds(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.failBuild(stampedLog("##[group]Run go test ./...", "##[endgroup]", "--- FAIL: TestThing", "##[error]Process completed with exit code 1."))
	w := fx.startWith(func(*StartRequest) {})
	h := fx.host.last()
	if msgs := h.messages(); len(msgs) != 1 || !strings.Contains(msgs[0], "babysitting") {
		t.Fatalf("messages while the opening turn runs = %q", msgs)
	}
	todo, _ := fx.st.UnnudgedActionable(context.Background(), w.ID)
	if len(todo) != 1 {
		t.Fatalf("untold = %+v, want the failed check to wait", todo)
	}

	fx.svc.schedule.polled(w.ID, fx.clock(), slowDown, fx.svc.cadence())
	if fx.due(w) {
		t.Fatal("the watch is due before the turn ends")
	}
	fx.hook(w, agent.EventStop, `{}`)
	if !fx.due(w) {
		t.Fatal("the end of the turn did not wake the watch for the failed check that waits")
	}
	if err := fx.svc.Poll(context.Background(), w.ID); err != nil {
		t.Fatal(err)
	}
	if msgs := h.messages(); len(msgs) != 2 || !strings.Contains(msgs[1], "Failed: build") {
		t.Fatalf("messages after the opening turn = %q", msgs)
	}
}

func TestAnAgentWithSilentHooksGetsTheMessage(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "rename it", URL: "https://c/11"}}
	})
	fx.poll(w)
	if msgs := h.messages(); len(msgs) != 1 {
		t.Fatalf("a working agent was told: %q", msgs)
	}

	fx.advance(silentTurn)
	fx.poll(w)
	if msgs := h.messages(); len(msgs) != 2 || !strings.Contains(msgs[1], "rename it") {
		t.Fatalf("messages after the hooks went silent = %q", msgs)
	}
}

func TestTheAuthorTypesIntoAWorkingAgent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	if _, err := fx.svc.Send(context.Background(), w.ID, "also update the docs"); err != nil {
		t.Fatalf("Send() to a working agent error = %v", err)
	}
	if msgs := fx.host.last().messages(); msgs[len(msgs)-1] != "also update the docs" {
		t.Fatalf("messages = %q", msgs)
	}
}

func TestTheEndOfATurnWakesThePollOnlyAfterTheTurnIsClosed(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.failBuild(stampedLog("##[group]Run go test ./...", "##[endgroup]", "--- FAIL: TestThing", "##[error]Process completed with exit code 1."))
	w := fx.startWith(func(*StartRequest) {})
	fx.svc.schedule.polled(w.ID, fx.clock(), slowDown, fx.svc.cadence())

	release := make(chan struct{})
	fx.svc.spawn(fx.svc.turns.queue(w.ID, func() { <-release }))
	if _, err := fx.svc.Hook(context.Background(), w.ID, agent.EventStop, []byte(`{}`)); err != nil {
		close(release)
		t.Fatalf("Hook(stop) error = %v", err)
	}
	dueBeforeClose := fx.due(w)
	close(release)
	fx.svc.wg.Wait()
	if dueBeforeClose {
		t.Fatal("the end of the turn woke the poll before the turn was closed")
	}
	if !fx.due(w) {
		t.Fatal("the closed turn did not wake the watch for the failed check that waits")
	}
}

func TestACompactionInTheTurnKeepsTheMessageWaiting(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.advance(silentTurn - time.Minute)
	fx.hook(w, agent.EventSessionStart, `{"source":"compact"}`)
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "rename it", URL: "https://c/11"}}
	})
	fx.advance(time.Minute)
	fx.poll(w)
	if msgs := h.messages(); len(msgs) != 1 {
		t.Fatalf("an agent that compacted its context in the turn was told: %q", msgs)
	}
}
