package prwatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/session"
	"github.com/deividfortuna/babysitter/internal/store"
)

var ErrAgentBusy = errors.New("the agent waits on the author")

var ErrWatchStopped = errors.New("the watch is stopped")

type deliverPolicy int

const (
	deliverRoutine deliverPolicy = iota
	deliverAuthor
)

func (p deliverPolicy) blocks(state agent.State) bool {
	if p == deliverAuthor {
		return state == agent.StateBlocked
	}
	return state.NeedsInput()
}

func (p deliverPolicy) source() string {
	if p == deliverAuthor {
		return "author"
	}
	return "daemon"
}

func (s *Service) waitsOnAuthor(w store.Watch) bool {
	l := s.sessions.get(w.ID)
	return l != nil && deliverRoutine.blocks(l.State())
}

func (s *Service) deliver(ctx context.Context, w store.Watch, text, summary string, policy deliverPolicy, rows []store.Activity) (store.Activity, error) {
	l, err := s.ensureSession(ctx, w)
	if err != nil {
		return store.Activity{}, err
	}
	if policy.blocks(l.State()) {
		return store.Activity{}, ErrAgentBusy
	}
	return s.send(ctx, w, l, text, summary, policy.source(), rows)
}

func (s *Service) send(ctx context.Context, w store.Watch, l *live, text, summary, source string, rows []store.Activity) (store.Activity, error) {
	l.nextTurn()
	if err := l.handle.Send(ctx, text); err != nil {
		s.queueEndTurn(w.ID, l.turnSeq())
		return store.Activity{}, err
	}
	return s.recordMessage(ctx, w, text, summary, source, rows)
}

func (s *Service) recordMessage(ctx context.Context, w store.Watch, text, summary, source string, rows []store.Activity) (store.Activity, error) {
	now := s.now()
	first := int64(0)
	if len(rows) > 0 {
		first = rows[0].ID
	}
	carried := ids(rows)
	row, err := s.recordDelivery(ctx, w, store.Activity{
		Kind: store.ActivityNudged, Ref: fmt.Sprintf("%d@%s", first, now.UTC().Format(time.RFC3339Nano)), At: now,
		Summary: summary,
		Payload: mustJSON(map[string]any{"activity_ids": carried, "message": text, "source": source}),
	})
	if err != nil {
		return store.Activity{}, err
	}
	if err := s.store.MarkActivityNudged(ctx, carried, now); err != nil {
		return store.Activity{}, err
	}
	return row, nil
}

func (s *Service) recordDelivery(ctx context.Context, w store.Watch, a store.Activity) (store.Activity, error) {
	if s.hosted(w) {
		return s.record(ctx, w, a)
	}
	row, _, err := s.insert(ctx, w, a)
	if err != nil {
		return store.Activity{}, err
	}
	s.store.PublishActivity(w.Key())
	return row, nil
}

func (s *Service) Send(ctx context.Context, id int64, text string) (store.Activity, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return store.Activity{}, errors.New("the message is empty")
	}
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return store.Activity{}, err
	}
	if w.Status != store.WatchActive {
		return store.Activity{}, ErrWatchStopped
	}
	if !s.hosted(w) {
		return store.Activity{}, ErrSelfWatch
	}
	if !s.runs(w) {
		return store.Activity{}, fmt.Errorf("%w: %s", ErrNoAgent, w.Provider)
	}
	unlock := s.locks.lock(w.ID)
	defer unlock()
	if w, err = s.store.GetWatch(ctx, id); err != nil {
		return store.Activity{}, err
	}
	if s.withAuthor(w) {
		return store.Activity{}, ErrTakenOver
	}
	if err := s.refusePending(ctx, w); err != nil {
		return store.Activity{}, err
	}
	summary := "you told the agent: " + firstLine(text)
	s.syncWork(ctx, w)
	return s.deliver(ctx, w, text, summary, deliverAuthor, nil)
}

func (s *Service) Hook(ctx context.Context, id int64, event string, payload []byte) error {
	if !agent.ValidEvent(event) {
		return fmt.Errorf("unknown hook event %q", event)
	}
	l := s.sessions.get(id)
	if l == nil {
		return nil
	}
	state, ok := agent.StateOf(event, payload)
	if !ok {
		return nil
	}
	if !l.report(state, s.now()) {
		return nil
	}
	s.store.PublishSession(l.key)
	switch {
	case state == agent.StateActive:
		work := s.workBranch(ctx, id)
		s.spawn(s.turns.queue(id, func() { s.startTurn(id, work) }))
	case state.EndsTurn():
		s.queueEndTurn(id, l.turnSeq())
	}
	return nil
}

type TerminalSize struct {
	Rows, Cols uint16
}

func (s *Service) Resize(ctx context.Context, id int64, size TerminalSize) error {
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return err
	}
	if w.Status != store.WatchActive {
		return ErrWatchStopped
	}
	if !s.hosted(w) {
		return ErrSelfWatch
	}
	unlock := s.sizeLocks.lock(id)
	defer unlock()
	s.sizes.set(id, size)
	l := s.sessions.get(id)
	if l == nil {
		return nil
	}
	if err := l.handle.Resize(size.Rows, size.Cols); err != nil && !errors.Is(err, session.ErrExited) {
		return err
	}
	return nil
}

func (s *Service) Output(ctx context.Context, id int64, n int) (string, error) {
	if l := s.sessions.get(id); l != nil {
		return redact.Text(l.handle.Output(n)), nil
	}
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return "", err
	}
	if !s.hosted(w) {
		return "", ErrSelfWatch
	}
	out, err := session.Tail(s.sessionLogPath(w.ID), n)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return redact.Text(out), nil
}

func (s *Service) Session(_ context.Context, w store.Watch) (SessionInfo, error) {
	if !s.hosted(w) && s.work.working(w.ID, s.now()) {
		return SessionInfo{State: agent.StateActive, AgentSession: w.AgentSession}, nil
	}
	l := s.sessions.get(w.ID)
	if l == nil {
		return SessionInfo{State: agent.StateNone, AgentSession: w.AgentSession}, nil
	}
	info := l.info()
	info.AgentSession = w.AgentSession
	return info, nil
}
