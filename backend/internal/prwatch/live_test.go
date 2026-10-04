package prwatch

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/session"
)

const aloneFor = 2 * time.Second

type slowHandle struct {
	fakeHandle
	entered chan struct{}
	open    <-chan struct{}
}

func newSlowHandle(open <-chan struct{}, pid int) *slowHandle {
	return &slowHandle{done: make(chan struct{}), pid: pid, entered: make(chan struct{}), open: open}
}

func (h *slowHandle) Stop(ctx context.Context) error {
	close(h.entered)
	select {
	case <-h.open:
		return h.fakeHandle.Stop(ctx)
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(aloneFor):
		return errors.New("this session stopped on its own, the others were not stopping yet")
	}
}

type ctxHandle struct {
	fakeHandle
}

func newCtxHandle() *ctxHandle {
	return &ctxHandle{done: make(chan struct{}), pid: 3000}
}

func (h *ctxHandle) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return h.fakeHandle.Stop(ctx)
}

func liveOf(h session.Handle) *live {
	return &live{handle: h, state: agent.StateIdle}
}

func TestShutdownStopsEverySessionTogether(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	open := make(chan struct{})
	handles := make([]*slowHandle, 3)
	for i := range handles {
		handles[i] = newSlowHandle(open, 2000+i)
		fx.svc.sessions.set(int64(i+1), liveOf(handles[i]))
	}
	go func() {
		for _, h := range handles {
			<-h.entered
		}
		close(open)
	}()

	done := make(chan struct{})
	go func() {
		fx.svc.stopSessions()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * aloneFor):
		t.Fatal("stopSessions did not return")
	}
	for i, h := range handles {
		if !h.wasStopped() {
			t.Errorf("session %d was left running", i+1)
		}
	}
}

func TestEndSessionStopsTheAgentAfterTheClientWentAway(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := newCtxHandle()
	fx.svc.sessions.set(w.ID, liveOf(h))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fx.svc.endSession(ctx, w)
	if !h.wasStopped() {
		t.Fatal("the agent session of the stopped watch is still running")
	}
}

func TestAContinuedSessionWaitsAtItsPromptAsIdle(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	ctx := context.Background()

	fx.svc = fx.newService()
	fx.svc.recover(ctx)
	fx.svc.wg.Wait()

	h := fx.host.last()
	if fx.host.count() != 2 || !strings.Contains(strings.Join(h.spec.Argv, " "), "--resume") {
		t.Fatalf("the new daemon did not continue the session: %d sessions, %v", fx.host.count(), h.spec.Argv)
	}
	info, err := fx.svc.Session(ctx, fx.watch(w))
	if err != nil || info.State != agent.StateIdle {
		t.Fatalf("state = %q, %v, want idle", info.State, err)
	}
	if blockers := fx.watch(w).ReadyBlockers; slices.Contains(blockers, "the agent is starting") {
		t.Fatalf("blockers = %v", blockers)
	}
}
