package prwatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/store"
)

var (
	ErrTakenOver      = errors.New("the session is with you; run babysitter watch handback first")
	ErrSessionRunning = errors.New("the session of the daemon did not stop, so it stays with the daemon; try the takeover again")
)

type Takeover struct {
	Watch           store.Watch
	WorktreeDir     string
	WorkBranch      string
	HeadRef         string
	Argv            []string
	Declined        []int
	NewConversation bool
}

type TakeoverOptions struct {
	PID   int
	Shell bool
}

func (s *Service) withAuthor(w store.Watch) bool {
	return w.TakenOverAt != nil
}

func (s *Service) hostsSession(w store.Watch) bool {
	return s.runs(w) && !s.withAuthor(w)
}

func (s *Service) authorBlocker(w store.Watch) string {
	if s.withAuthor(w) {
		return "the session is with you"
	}
	return ""
}

func (s *Service) Takeover(ctx context.Context, id int64, o TakeoverOptions) (Takeover, error) {
	unlock := s.locks.Lock(id)
	defer unlock()
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return Takeover{}, err
	}
	runner, err := s.takeoverRunner(w, o)
	if err != nil {
		return Takeover{}, err
	}
	fresh := w.AgentSession == "" && !o.Shell
	session := w.AgentSession
	if fresh {
		session = runner.NewSessionID()
	}
	argv, err := authorCommand(runner, w, session, fresh, o)
	if err != nil {
		return Takeover{}, err
	}
	now := s.now()
	if w, err = s.store.SetWatchTakeover(ctx, id, &now, o.PID); err != nil {
		return Takeover{}, err
	}
	w.AgentSession = session
	declined, err := s.giveToAuthor(ctx, w, fresh, o, now)
	if err != nil {
		return Takeover{}, errors.Join(err, s.undoTakeover(ctx, id, fresh))
	}
	s.Kick(w.ID)
	return Takeover{
		Watch: w, WorktreeDir: w.WorktreeDir, WorkBranch: w.WorkBranch, HeadRef: w.HeadRef,
		Argv: argv, Declined: declined, NewConversation: fresh,
	}, nil
}

func authorCommand(runner agent.Runner, w store.Watch, session string, fresh bool, o TakeoverOptions) ([]string, error) {
	if o.Shell {
		return []string{}, nil
	}
	return runner.AuthorCommand(agent.Launch{
		WorktreeDir: w.WorktreeDir, Model: w.Model, Effort: w.Effort, SessionID: session, Resume: !fresh,
	})
}

func (s *Service) giveToAuthor(ctx context.Context, w store.Watch, fresh bool, o TakeoverOptions, now time.Time) ([]int, error) {
	waiting, err := s.store.DeclineWaitingProposals(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	if err := s.handOverSession(ctx, w); err != nil {
		return nil, errors.Join(err, s.store.RestoreProposals(ctx, waiting))
	}
	declined := proposalNumbers(waiting)
	s.work.clear(w.ID)
	if fresh {
		if err := s.store.SetWatchAgentSession(ctx, w.ID, w.AgentSession); err != nil {
			return nil, err
		}
	}
	text := "you took over the session in your terminal"
	if len(declined) > 0 {
		text += "; " + declinedWord(declined)
	}
	_, err = s.record(ctx, w, store.Activity{
		Kind: store.ActivityTakenOver, Ref: stampRef("takeover", now), At: now, Summary: text,
		Payload: mustJSON(map[string]any{"by": "author", "pid": o.PID, "declined": declined}),
	})
	return declined, err
}

func (s *Service) undoTakeover(ctx context.Context, id int64, fresh bool) error {
	_, err := s.store.SetWatchTakeover(ctx, id, nil, 0)
	if fresh {
		err = errors.Join(err, s.store.SetWatchAgentSession(ctx, id, ""))
	}
	return err
}

func (s *Service) handOverSession(ctx context.Context, w store.Watch) error {
	l := s.sessions.remove(w.ID)
	if l == nil {
		return nil
	}
	err := stopLive(ctx, l)
	if err == nil {
		return nil
	}
	l.cancelStop()
	s.sessions.set(w.ID, l)
	return fmt.Errorf("%w (pid %d): %w", ErrSessionRunning, l.handle.PID(), err)
}

func (s *Service) takeoverRunner(w store.Watch, o TakeoverOptions) (agent.Runner, error) {
	switch {
	case w.Status != store.WatchActive:
		return nil, ErrWatchStopped
	case !s.hosted(w):
		return nil, ErrSelfWatch
	case s.withAuthor(w):
		return nil, ErrTakenOver
	case o.Shell:
		return nil, nil
	}
	runner := s.agentFor(w.Provider)
	if runner == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoAgent, w.Provider)
	}
	return runner, nil
}
