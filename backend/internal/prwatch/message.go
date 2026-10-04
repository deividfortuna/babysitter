package prwatch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/session"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

var ErrAgentBusy = errors.New("the agent waits on the author")

var ErrAgentWorking = errors.New("the agent is still working")

var ErrWatchStopped = errors.New("the watch is stopped")

type deliverPolicy int

const (
	deliverRoutine deliverPolicy = iota
	deliverBetweenTurns
	deliverAuthor
)

func (p deliverPolicy) refusal(l *live, now time.Time) error {
	state := l.State()
	if p == deliverAuthor {
		return authorRefusal(state)
	}
	if state.NeedsInput() {
		return ErrAgentBusy
	}
	turnRuns := p == deliverBetweenTurns && l.holdsMessages(now)
	if turnRuns {
		return ErrAgentWorking
	}
	return nil
}

func authorRefusal(state agent.State) error {
	if state == agent.StateBlocked {
		return ErrAgentBusy
	}
	return nil
}

func (p deliverPolicy) source() string {
	if p == deliverAuthor {
		return "author"
	}
	return "daemon"
}

func (s *Service) waitsOnAuthor(w store.Watch) bool {
	l := s.sessions.get(w.ID)
	return l != nil && l.State().NeedsInput()
}

func (s *Service) agentWorks(w store.Watch) bool {
	l := s.sessions.get(w.ID)
	return l != nil && l.holdsMessages(s.now())
}

func (s *Service) deliver(ctx context.Context, w store.Watch, text, summary string, policy deliverPolicy, rows []store.Activity) (store.Activity, error) {
	l, err := s.ensureSession(ctx, w)
	if err != nil {
		return store.Activity{}, err
	}
	if err := policy.refusal(l, s.now()); err != nil {
		return store.Activity{}, err
	}
	return s.send(ctx, w, l, text, summary, policy.source(), rows)
}

func (s *Service) send(ctx context.Context, w store.Watch, l *live, text, summary, source string, rows []store.Activity) (store.Activity, error) {
	l.nextTurn(s.now())
	if err := l.handle.Send(ctx, text); err != nil {
		l.dropPrompt()
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
	unlock := s.locks.Lock(w.ID)
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
	if s.waitsForGitHub(ctx, w) {
		return store.Activity{}, ErrBranchUpdating
	}
	summary := "you told the agent: " + firstLine(text)
	s.syncWork(ctx, w)
	return s.deliver(ctx, w, text, summary, deliverAuthor, nil)
}

func (s *Service) Hook(ctx context.Context, id int64, event string, payload []byte) (agent.ToolVerdict, error) {
	if !agent.ValidEvent(event) {
		return agent.ToolVerdict{}, fmt.Errorf("unknown hook event %q", event)
	}
	l := s.sessions.get(id)
	var call agent.ToolCall
	if event == agent.EventPreToolUse {
		call = agent.ParseToolCall(payload)
	}
	running := l != nil && l.State() != agent.StateExited
	verdict := agent.DecideTool(event, call, agent.ToolFacts{WatchID: id, Exe: cmp.Or(s.exe, "babysitter"), Live: running})
	s.logHook(ctx, id, event, call, verdict)
	if l != nil {
		s.reportState(context.WithoutCancel(ctx), id, l, event, payload)
	}
	return verdict, nil
}

const hookCommandWidth = 200

func (s *Service) logHook(ctx context.Context, id int64, event string, call agent.ToolCall, verdict agent.ToolVerdict) {
	level := slog.LevelDebug
	if verdict.Deny {
		level = slog.LevelInfo
	}
	if !s.log.Enabled(ctx, level) {
		return
	}
	attrs := []any{"watch", id, "event", event, "decision", verdict.Decision()}
	if call.Tool != "" {
		attrs = append(attrs, "tool", call.Tool)
	}
	if call.Command != "" {
		attrs = append(attrs, "command", loggedCommand(call.Command))
	}
	if verdict.Deny {
		attrs = append(attrs, "rule", verdict.Rule)
	}
	s.log.Log(ctx, level, "the agent called its hook", attrs...)
}

func loggedCommand(command string) string {
	oneLine := strings.Join(strings.Fields(redact.Text(command)), " ")
	return textx.FirstLine(oneLine, hookCommandWidth)
}

func (s *Service) reportState(ctx context.Context, id int64, l *live, event string, payload []byte) {
	s.noteLimit(ctx, id, event, payload)
	state, changed := l.reportEvent(event, payload, s.now())
	if !changed {
		return
	}
	s.store.PublishSession(l.key)
	switch {
	case state == agent.StateActive:
		work := s.workBranch(ctx, id)
		s.spawn(s.turns.queue(id, func() { s.startTurn(id, work) }))
	case state.EndsTurn():
		s.queueEndTurn(id, l.turnSeq())
		s.spawn(s.turns.queue(id, func() { s.kickForWaiting(id) }))
	}
}

func (s *Service) kickForWaiting(id int64) {
	ctx := context.WithoutCancel(s.background())
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		s.log.Error("read the watch at the end of the turn", "watch", id, "err", err)
		return
	}
	if s.limitIsOver(w) {
		s.Kick(id)
		return
	}
	todo, err := s.store.UnnudgedActionable(ctx, id)
	if err != nil {
		s.log.Error("read the messages that wait for the end of the turn", "watch", id, "err", err)
		return
	}
	if len(todo) > 0 {
		s.Kick(id)
	}
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
	unlock := s.sizeLocks.Lock(id)
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
