package prwatch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/session"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

const (
	readyPoll    = 100 * time.Millisecond
	startupGrace = 15 * time.Second
	stopTimeout  = 10 * time.Second
)

type live struct {
	handle    session.Handle
	startedAt time.Time
	resumed   bool
	signals   bool
	logPath   string
	key       store.WatchKey

	mu       sync.Mutex
	state    agent.State
	signalAt time.Time
	stopping bool
	turn     uint64
}

func (l *live) nextTurn() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.turn++
}

func (l *live) turnSeq() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.turn
}

func (l *live) State() agent.State {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state
}

func (l *live) report(state agent.State, at time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	changed := l.state != state && l.state != agent.StateExited
	if changed {
		l.state = state
	}
	l.signalAt = at
	return changed
}

func (l *live) markExited(at time.Time) (stopping bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state = agent.StateExited
	l.signalAt = at
	return l.stopping
}

func (l *live) beginStop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopping = true
}

func (l *live) cancelStop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopping = false
}

func (l *live) info() SessionInfo {
	l.mu.Lock()
	defer l.mu.Unlock()
	started := l.startedAt
	out := SessionInfo{State: l.state, PID: l.handle.PID(), StartedAt: &started, LogPath: l.logPath}
	if !l.signalAt.IsZero() {
		signal := l.signalAt
		out.SignalAt = &signal
	}
	return out
}

type sessions struct {
	registry[*live]
}

func (r *sessions) remove(id int64) *live {
	l := r.take(id)
	if l != nil {
		l.beginStop()
	}
	return l
}

func (r *sessions) removeAll() []*live {
	out := r.takeAll()
	for _, l := range out {
		l.beginStop()
	}
	return out
}

type SessionInfo struct {
	State        agent.State
	PID          int
	StartedAt    *time.Time
	SignalAt     *time.Time
	AgentSession string
	LogPath      string
}

func (s *Service) hook(watchID int64) []string {
	if s.exe == "" {
		return nil
	}
	return []string{s.exe, "watch", "hook", "--data-dir", s.dataDir, "--watch", strconv.FormatInt(watchID, 10)}
}

func (s *Service) sessionLogPath(watchID int64) string {
	return filepath.Join(s.dataDir, "sessions", fmt.Sprintf("%d.log", watchID))
}

func (s *Service) ensureSession(ctx context.Context, w store.Watch) (*live, error) {
	if l := s.sessions.get(w.ID); l != nil && l.State() != agent.StateExited {
		return l, nil
	}
	runner := s.agentFor(w.Provider)
	if runner == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoAgent, w.Provider)
	}
	w, err := s.store.GetWatch(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	set, err := s.store.Settings(ctx)
	if err != nil {
		return nil, err
	}
	sessionID, resume := w.AgentSession, w.AgentSession != ""
	if !resume {
		sessionID = runner.NewSessionID()
	}
	argv, env, err := runner.Command(agent.Launch{
		WorktreeDir: w.WorktreeDir, Model: w.Model, Effort: w.Effort, SessionID: sessionID, Resume: resume,
		Name: fmt.Sprintf("babysitter %s#%d", w.Repo(), w.Number),
		Hook: s.hook(w.ID), HooksDir: filepath.Join(s.dataDir, "git-hooks"), ScreenReader: set.ScreenReader,
	})
	if err != nil {
		return nil, err
	}
	env = append(env, "BABYSITTER_WATCH="+strconv.FormatInt(w.ID, 10), "BABYSITTER_DATA_DIR="+s.dataDir)
	logPath := s.sessionLogPath(w.ID)
	size := s.sizes.get(w.ID)
	h, err := s.host.Start(ctx, session.Spec{
		Dir: w.WorktreeDir, Argv: argv, Env: env, LogPath: logPath, Rows: size.Rows, Cols: size.Cols,
	})
	if err != nil {
		return nil, err
	}
	if !resume {
		if err := s.store.SetWatchAgentSession(ctx, w.ID, sessionID); err != nil {
			return nil, err
		}
	}
	state := agent.StateIdle
	if runner.Signals() {
		state = agent.StateStarting
	}
	l := &live{handle: h, state: state, startedAt: s.now(), resumed: resume, signals: runner.Signals(), logPath: logPath, key: w.Key()}
	s.sessions.set(w.ID, l)
	s.catchUpSize(w.ID, l, size)
	s.recordSessionStart(ctx, w, l, sessionID, resume)
	go s.watchExit(w.ID, l)
	s.awaitReady(ctx, l)
	if resume {
		s.settleContinued(w, l)
		return l, nil
	}
	msg, err := agent.OpenMessage(agent.Open{
		PR: pullRequestOf(w), WorktreeDir: w.WorktreeDir, WorkBranch: w.WorkBranch,
		Interval: interval(s.Interval()), Prelude: runner.Prelude(), ReplyCommand: s.replyCommand(w),
	})
	if err != nil {
		return l, err
	}
	if _, err := s.send(ctx, w, l, msg, "told the agent about the pull request", "daemon", nil); err != nil {
		return l, err
	}
	return l, nil
}

func (s *Service) catchUpSize(watchID int64, l *live, startSize TerminalSize) {
	unlock := s.sizeLocks.Lock(watchID)
	defer unlock()
	size := s.sizes.get(watchID)
	if size == startSize {
		return
	}
	if err := l.handle.Resize(size.Rows, size.Cols); err != nil && !errors.Is(err, session.ErrExited) {
		s.log.Error("resize the new agent session", "watch", watchID, "err", err)
	}
}

func (s *Service) settleContinued(w store.Watch, l *live) {
	if l.State() != agent.StateStarting {
		return
	}
	if l.report(agent.StateIdle, s.now()) {
		s.store.PublishSession(w.Key())
	}
}

func (s *Service) recordSessionStart(ctx context.Context, w store.Watch, l *live, sessionID string, resume bool) {
	word := "started"
	if resume {
		word = "continued"
	}
	if _, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivitySessionStarted, Ref: stampRef(l.handle.PID(), s.now()),
		Summary: fmt.Sprintf("agent session %s (%s, pid %d)", word, w.Provider, l.handle.PID()),
		Payload: mustJSON(map[string]any{"pid": l.handle.PID(), "agent_session": sessionID, "resumed": resume, "provider": w.Provider}),
	}); err != nil {
		s.log.Error("record session start", "watch", w.ID, "err", err)
	}
	s.store.PublishSession(w.Key())
}

func pullRequestOf(w store.Watch) agent.PullRequest {
	return agent.PullRequest{
		Repo: w.Repo(), Number: w.Number, Title: w.Title, URL: w.URL, Author: w.Author,
		HeadRef: w.HeadRef, BaseRef: w.BaseRef, HeadSHA: w.HeadSHA, Dependabot: agent.IsDependabot(w.Author),
		DaemonPushes: hostedProvider(w.Provider), MergesBase: w.BranchUpdate == store.BranchMerge,
	}
}

func interval(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		return textx.Plural(int(d/time.Hour), "hour")
	case d >= time.Minute && d%time.Minute == 0:
		return textx.Plural(int(d/time.Minute), "minute")
	case d >= time.Second:
		return textx.Plural(int(d/time.Second), "second")
	default:
		return textx.Plural(int(d/time.Millisecond), "millisecond")
	}
}

func (s *Service) awaitReady(ctx context.Context, l *live) {
	if !l.signals {
		_ = l.handle.Ready(ctx)
		return
	}
	settling, stop := context.WithCancel(ctx)
	defer stop()
	settled := make(chan struct{})
	go func() {
		defer close(settled)
		_ = l.handle.Ready(settling)
	}()
	tick := time.NewTicker(readyPoll)
	defer tick.Stop()
	for {
		select {
		case <-settled:
			return
		case <-tick.C:
			if l.State() != agent.StateStarting {
				return
			}
		}
	}
}

func (s *Service) watchExit(watchID int64, l *live) {
	<-l.handle.Done()
	if stopping := l.markExited(s.now()); stopping {
		return
	}
	ctx := context.WithoutCancel(s.background())
	if l.resumed && s.now().Sub(l.startedAt) < startupGrace {
		if err := s.store.SetWatchAgentSession(ctx, watchID, ""); err != nil {
			s.log.Error("forget the agent session", "watch", watchID, "err", err)
		}
	}
	w, err := s.store.GetWatch(ctx, watchID)
	if err != nil || w.Status != store.WatchActive {
		return
	}
	reason := "exit 0"
	if l.handle.Err() != nil {
		reason = l.handle.Err().Error()
	}
	if _, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivitySessionExited, Ref: stampRef(l.handle.PID(), s.now()),
		Summary: "agent session exited: " + reason + "; it starts again with the next message",
		Payload: mustJSON(map[string]any{"pid": l.handle.PID(), "error": reason, "log": l.logPath}),
	}); err != nil {
		s.log.Error("record session exit", "watch", watchID, "err", err)
	}
	s.store.PublishSession(w.Key())
	s.endTurn(watchID, l.turnSeq())
}

func (s *Service) endSession(ctx context.Context, w store.Watch) {
	l := s.sessions.remove(w.ID)
	if l == nil {
		return
	}
	if err := stopLive(ctx, l); err != nil {
		s.log.Warn("stop the agent session", "watch", w.ID, "err", err)
	}
}

func stopLive(ctx context.Context, l *live) error {
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopTimeout)
	defer cancel()
	return l.handle.Stop(stopCtx)
}

func (s *Service) stopSessions() {
	var wg sync.WaitGroup
	for _, l := range s.sessions.removeAll() {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
			defer cancel()
			if err := l.handle.Stop(ctx); err != nil {
				s.log.Warn("stop the agent session", "pid", l.handle.PID(), "err", err)
			}
		})
	}
	wg.Wait()
}
