package prwatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/store"
)

const limitContinueMessage = "Your usage limit is over. Continue the work you were doing when the limit stopped you."

func limited(w store.Watch) bool {
	return w.AgentLimitedUntil != nil
}

func limitBlocker(w store.Watch) string {
	if !limited(w) {
		return ""
	}
	return "the agent hit its usage limit until " + limitClock(*w.AgentLimitedUntil)
}

func limitClock(t time.Time) string {
	return t.Local().Format("Jan 2 15:04 MST")
}

func (s *Service) noteLimit(ctx context.Context, id int64, event string, payload []byte) {
	if event == agent.EventStop {
		s.clearLimit(ctx, id)
		return
	}
	until, ok := agent.LimitOf(event, payload, s.now())
	if !ok {
		return
	}
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		s.log.Error("read the watch of a usage limit", "watch", id, "err", err)
		return
	}
	if err := s.store.SetWatchAgentLimit(ctx, id, &until); err != nil {
		s.log.Error("store the usage limit of the agent", "watch", id, "err", err)
		return
	}
	s.store.PublishSession(w.Key())
	if _, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityAgentFailed, Ref: "limit@" + until.UTC().Format(time.RFC3339),
		Summary: fmt.Sprintf("the agent hit its usage limit; the watch tells it to continue at %s", limitClock(until)),
		Payload: mustJSON(map[string]any{"what": "usage limit", "until": until.UTC()}),
	}); err != nil {
		s.log.Error("record the usage limit of the agent", "watch", id, "err", err)
	}
}

func (s *Service) clearLimit(ctx context.Context, id int64) {
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		s.log.Error("read the watch to clear its usage limit", "watch", id, "err", err)
		return
	}
	if !limited(w) {
		return
	}
	if err := s.store.SetWatchAgentLimit(ctx, id, nil); err != nil {
		s.log.Error("clear the usage limit of the agent", "watch", id, "err", err)
		return
	}
	s.store.PublishSession(w.Key())
}

func (s *Service) limitedNow(ctx context.Context, id int64) (bool, error) {
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return false, fmt.Errorf("read the usage limit before the message: %w", err)
	}
	return limited(w), nil
}

func (s *Service) limitIsOver(w store.Watch) bool {
	return limited(w) && !s.now().Before(*w.AgentLimitedUntil)
}

func (s *Service) resumeAfterLimit(ctx context.Context, w store.Watch) {
	if !s.limitIsOver(w) {
		return
	}
	_, err := s.deliver(ctx, w, limitContinueMessage, "told the agent to continue after its usage limit", deliverBetweenTurns, nil)
	switch {
	case errors.Is(err, ErrAgentBusy), errors.Is(err, ErrAgentWorking):
		s.log.Info("the agent is not free, it continues after its usage limit at the next poll", "watch", w.ID, "reason", err)
		return
	case err != nil:
		s.agentFailed(ctx, w, "tell the agent to continue after its usage limit", err)
		return
	}
	cleared, err := s.store.ClearWatchAgentLimit(ctx, w.ID, *w.AgentLimitedUntil)
	if err != nil {
		s.log.Error("clear the usage limit the agent continued from", "watch", w.ID, "err", err)
		return
	}
	if cleared {
		s.store.PublishSession(w.Key())
	}
}
