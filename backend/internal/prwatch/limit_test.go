package prwatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (fx *fixture) hitLimit(w store.Watch, reset time.Time) {
	fx.t.Helper()
	payload, err := json.Marshal(map[string]string{
		"error":                  "rate_limit",
		"last_assistant_message": fmt.Sprintf("You've hit your session limit · resets %s (UTC)", reset.UTC().Format("3:04pm")),
	})
	if err != nil {
		fx.t.Fatal(err)
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStopFailure, string(payload))
}

func TestALimitedAgentGetsNoMessageUntilTheReset(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	ctx := context.Background()
	reset := fx.clock().UTC().Add(2 * time.Hour).Truncate(time.Minute)

	fx.hitLimit(w, reset)
	got := fx.watch(w)
	if got.AgentLimitedUntil == nil || !got.AgentLimitedUntil.Equal(reset) {
		t.Fatalf("limited until = %v, want %v", got.AgentLimitedUntil, reset)
	}
	if info, _ := fx.svc.Session(ctx, w); info.State != agent.StateIdle {
		t.Fatalf("state after the limit = %q", info.State)
	}
	fx.waitKinds(w, []string{"watch_started", "session_started", "nudged", "agent_failed"})
	if row := fx.activity(w)[3]; !strings.Contains(row.Summary, "usage limit") || row.Ref != "limit@"+reset.Format(time.RFC3339) {
		t.Fatalf("limit row = %+v", row)
	}

	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "rename it", URL: "https://c/11"}}
	})
	fx.poll(w)
	if msgs := h.messages(); len(msgs) != 1 {
		t.Fatalf("a limited agent was told: %q", msgs)
	}
	if _, blockers := fx.svc.Readiness(fx.watch(w), agent.StateIdle); !slices.ContainsFunc(blockers, func(b string) bool { return strings.Contains(b, "usage limit") }) {
		t.Fatalf("blockers = %q", blockers)
	}

	fx.advance(2 * time.Hour)
	fx.poll(w)
	msgs := h.messages()
	if len(msgs) != 2 || msgs[1] != limitContinueMessage {
		t.Fatalf("messages after the reset = %q", msgs)
	}
	if got := fx.watch(w); got.AgentLimitedUntil != nil {
		t.Fatalf("limited until after the resume = %v", got.AgentLimitedUntil)
	}

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.agentIdle(w)
	fx.poll(w)
	if msgs := h.messages(); len(msgs) != 3 || !strings.Contains(msgs[2], "rename it") {
		t.Fatalf("messages after the continued turn = %q", msgs)
	}
}

func TestAStopClearsTheLimit(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.hitLimit(w, fx.clock().Add(time.Hour))
	if fx.watch(w).AgentLimitedUntil == nil {
		t.Fatal("the limit was not stored")
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.agentIdle(w)
	if got := fx.watch(w).AgentLimitedUntil; got != nil {
		t.Fatalf("limited until after a turn that ended well = %v", got)
	}
}

func TestOtherFailuresSetNoLimit(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStopFailure, `{"error":"overloaded","last_assistant_message":"resets 3pm"}`)
	if got := fx.watch(w).AgentLimitedUntil; got != nil {
		t.Fatalf("limited until after an overload = %v", got)
	}
	if info, _ := fx.svc.Session(context.Background(), w); info.State != agent.StateIdle {
		t.Fatalf("state after a failed turn = %q", info.State)
	}
}

func TestBackgroundWorkKeepsTheWatchFromMerging(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"go test ./..."}]}`)
	info, _ := fx.svc.Session(context.Background(), w)
	if info.State != agent.StateWaiting {
		t.Fatalf("state = %q", info.State)
	}
	if _, blockers := fx.svc.Readiness(fx.watch(w), info.State); !slices.Contains(blockers, "the background work of the agent still runs") {
		t.Fatalf("blockers = %q", blockers)
	}
}

func TestIdleNoticeKeepsTheAgentWaitingOnBackgroundWork(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	state := func() agent.State {
		info, _ := fx.svc.Session(context.Background(), w)
		return info.State
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"sleep 150"}]}`)
	fx.hook(w, agent.EventNotification, `{"notification_type":"idle_prompt"}`)
	if got := state(); got != agent.StateWaiting {
		t.Fatalf("state after the idle notice = %q, want %q", got, agent.StateWaiting)
	}
	if _, blockers := fx.svc.Readiness(fx.watch(w), state()); !slices.Contains(blockers, "the background work of the agent still runs") {
		t.Fatalf("blockers = %q", blockers)
	}
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{"background_tasks":[]}`)
	if got := state(); got != agent.StateIdle {
		t.Fatalf("state after the background work ends = %q, want %q", got, agent.StateIdle)
	}
}

func TestATurnThatEndsAfterTheResetWakesThePollToContinue(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	fx.hitLimit(w, fx.clock().UTC().Add(time.Hour).Truncate(time.Minute))

	fx.advance(time.Hour)
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.poll(w)
	if msgs := h.messages(); len(msgs) != 1 {
		t.Fatalf("a working agent was told to continue: %q", msgs)
	}
	fx.svc.schedule.polled(w.ID, fx.clock(), slowDown, fx.svc.cadence())
	fx.hook(w, agent.EventStopFailure, `{"error":"overloaded"}`)
	if !fx.due(w) {
		t.Fatal("the end of the turn did not wake the watch to continue after the reset")
	}
}

func TestALimitTakesTheReadinessAwayAtOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)
	fx.poll(w)
	readiness := func() (*time.Time, []string) {
		info, _ := fx.svc.Session(context.Background(), w)
		return fx.svc.Readiness(fx.watch(w), info.State)
	}
	if since, blockers := readiness(); since == nil {
		t.Fatalf("a good watch is not ready: %q", blockers)
	}
	limitBlocks := func(b string) bool { return strings.Contains(b, "usage limit") }

	fx.hitLimit(w, fx.clock().Add(time.Hour))
	if since, blockers := readiness(); since != nil || !slices.ContainsFunc(blockers, limitBlocks) {
		t.Fatalf("readiness after the limit, before a poll = %v, %q", since, blockers)
	}
	fx.poll(w)
	if since, blockers := readiness(); since != nil || len(slices.DeleteFunc(slices.Clone(blockers), limitBlocks)) != 0 || !slices.ContainsFunc(blockers, limitBlocks) {
		t.Fatalf("readiness after a poll = %v, %q, want the limit once", since, blockers)
	}

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.agentIdle(w)
	if _, blockers := readiness(); slices.ContainsFunc(blockers, limitBlocks) {
		t.Fatalf("blockers after the limit was cleared = %q", blockers)
	}
}

func TestALimitThatComesDuringThePollKeepsTheMessage(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	ctx := context.Background()
	reset := fx.clock().UTC().Add(time.Hour).Truncate(time.Minute)
	payload, err := json.Marshal(map[string]string{
		"error":                  "rate_limit",
		"last_assistant_message": fmt.Sprintf("You've hit your session limit · resets %s (UTC)", reset.Format("3:04pm")),
	})
	if err != nil {
		t.Fatal(err)
	}
	fx.rel.duringFetch = func() {
		for _, e := range []struct {
			event   string
			payload []byte
		}{{agent.EventUserPromptSubmit, []byte(`{}`)}, {agent.EventStopFailure, payload}} {
			if _, err := fx.svc.Hook(ctx, w.ID, e.event, e.payload); err != nil {
				t.Errorf("Hook(%s) error = %v", e.event, err)
			}
		}
	}
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "rename it", URL: "https://c/11"}}
	})
	fx.poll(w)
	fx.svc.wg.Wait()
	if fx.watch(w).AgentLimitedUntil == nil {
		t.Fatal("the limit that came during the poll was not stored")
	}
	if msgs := h.messages(); len(msgs) != 1 {
		t.Fatalf("an agent that hit its limit during the poll was told: %q", msgs)
	}
}

func TestThePollKeepsTheMessageWhenItCannotReadTheLimit(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	db, err := sql.Open("sqlite3", "file:"+fx.dbPath+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	fx.rel.duringFetch = func() {
		if _, err := db.Exec("ALTER TABLE watches RENAME COLUMN agent_limited_until TO limit_unreadable"); err != nil {
			t.Errorf("break the read of the limit: %v", err)
		}
	}
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "rename it", URL: "https://c/11"}}
	})
	fx.advance(time.Minute)
	_ = fx.svc.Poll(context.Background(), w.ID)
	fx.svc.wg.Wait()
	if _, err := db.Exec("ALTER TABLE watches RENAME COLUMN limit_unreadable TO agent_limited_until"); err != nil {
		t.Fatal(err)
	}
	if msgs := h.messages(); len(msgs) != 1 {
		t.Fatalf("the poll typed a message without knowing the limit: %q", msgs)
	}
}

func TestALimitThatComesAfterTheContinueMessageStays(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	ctx := context.Background()
	fx.hitLimit(w, fx.clock().UTC().Add(time.Hour).Truncate(time.Minute))
	fx.advance(time.Hour)

	later := fx.clock().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	payload, err := json.Marshal(map[string]string{
		"error":                  "rate_limit",
		"last_assistant_message": fmt.Sprintf("You've hit your session limit · resets %s (UTC)", later.Format("3:04pm")),
	})
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	h.mu.Lock()
	h.onSend = func() {
		once.Do(func() {
			for _, e := range []struct {
				event   string
				payload []byte
			}{{agent.EventUserPromptSubmit, []byte(`{}`)}, {agent.EventStopFailure, payload}} {
				if _, err := fx.svc.Hook(ctx, w.ID, e.event, e.payload); err != nil {
					t.Errorf("Hook(%s) error = %v", e.event, err)
				}
			}
		})
	}
	h.mu.Unlock()
	fx.poll(w)
	fx.svc.wg.Wait()

	if msgs := h.messages(); len(msgs) != 2 || msgs[1] != limitContinueMessage {
		t.Fatalf("messages = %q", msgs)
	}
	if got := fx.watch(w).AgentLimitedUntil; got == nil || !got.Equal(later) {
		t.Fatalf("limited until = %v, want the newer limit %v", got, later)
	}
}
