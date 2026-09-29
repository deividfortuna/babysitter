package prwatch

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/store"
)

var ErrSelfWatch = errors.New("the watch has no agent session in the daemon: the session that started it is its agent")

var ErrHostedWatch = errors.New("the agent of the watch runs in the daemon; read its session with watch output")

var ErrNextInFlight = errors.New("another caller already waits for the next message of this watch")

type waiting struct {
	registry[struct{}]
}

func (r *waiting) enter(id int64) bool {
	_, first := r.getOrMake(id, func() struct{} { return struct{}{} })
	return first
}

func (r *waiting) leave(id int64) {
	r.drop(id)
}

type NextMessage struct {
	Watch   store.Watch
	Message *store.Activity
}

func (s *Service) done(o NextMessage) bool {
	return o.Message != nil || o.Watch.Status != store.WatchActive || s.settledReady(o.Watch)
}

func (s *Service) settledReady(w store.Watch) bool {
	return settled(w.ReadySince, s.now(), s.Interval())
}

const selfWorkDeadline = 30 * time.Minute

type selfWork struct {
	registry[time.Time]
}

func (r *selfWork) start(id int64, until time.Time) {
	r.set(id, until)
}

func (r *selfWork) clear(id int64) {
	r.drop(id)
}

func (r *selfWork) working(id int64, now time.Time) bool {
	return now.Before(r.get(id))
}

func (s *Service) Next(ctx context.Context, id int64, wait time.Duration) (NextMessage, error) {
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return NextMessage{}, err
	}
	if s.hosted(w) {
		return NextMessage{}, ErrHostedWatch
	}
	if !s.waiting.enter(id) {
		return NextMessage{}, ErrNextInFlight
	}
	defer s.waiting.leave(id)
	s.work.clear(id)
	woke, unsubscribe := s.changesOf(w)
	defer unsubscribe()
	client, err := s.newClient(ctx)
	if err != nil {
		return NextMessage{}, err
	}
	if err := s.pollWatch(context.WithoutCancel(ctx), client, id); err != nil && ctx.Err() == nil {
		s.log.Warn("poll before the next message failed", "watch", id, "err", err)
	}
	out, err := s.take(ctx, client, id)
	if err != nil || s.done(out) || wait <= 0 {
		return s.handOver(ctx, out, err)
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		expired := false
		select {
		case <-woke:
		case <-timer.C:
			expired = true
		case <-ctx.Done():
			return NextMessage{}, ctx.Err()
		}
		out, err = s.take(ctx, client, id)
		if expired || err != nil || s.done(out) {
			return s.handOver(ctx, out, err)
		}
	}
}

func (s *Service) handOver(ctx context.Context, out NextMessage, err error) (NextMessage, error) {
	if err != nil || out.Message == nil {
		return out, err
	}
	if ctx.Err() == nil {
		s.report(context.WithoutCancel(ctx), out.Watch, []store.Activity{*out.Message})
		return out, err
	}
	if back := s.undeliver(context.WithoutCancel(ctx), out); back != nil {
		s.log.Error("take back the message of a caller that gave up", "watch", out.Watch.ID, "err", back)
		return out, err
	}
	return NextMessage{}, ctx.Err()
}

func (s *Service) returnUndelivered(ctx context.Context, w store.Watch) error {
	if s.hosted(w) {
		return nil
	}
	rows, err := s.store.UnreportedActivity(ctx, w.ID)
	if err != nil {
		return err
	}
	for _, a := range rows {
		if a.Kind != store.ActivityNudged {
			continue
		}
		if err := s.undeliver(ctx, NextMessage{Watch: w, Message: &a}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) restoreWork(ctx context.Context, w store.Watch) error {
	if s.hosted(w) {
		return nil
	}
	row, ok, err := s.store.LastNudged(ctx, w.ID)
	if err != nil || !ok {
		return err
	}
	until := row.At.Add(selfWorkDeadline)
	if !s.now().Before(until) {
		return nil
	}
	s.work.start(w.ID, until)
	return nil
}

func (s *Service) undeliver(ctx context.Context, out NextMessage) error {
	var p struct {
		ActivityIDs []int64 `json:"activity_ids"`
	}
	if err := json.Unmarshal(out.Message.Payload, &p); err != nil {
		return err
	}
	if err := s.store.UnmarkActivityNudged(ctx, p.ActivityIDs); err != nil {
		return err
	}
	if err := s.store.DeleteActivity(ctx, out.Message.ID); err != nil {
		return err
	}
	s.work.clear(out.Watch.ID)
	s.store.PublishActivity(out.Watch.Key())
	return nil
}

func (s *Service) take(ctx context.Context, client *github.Client, id int64) (NextMessage, error) {
	unlock := s.locks.Lock(id)
	defer unlock()
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return NextMessage{}, err
	}
	if s.hosted(w) {
		return NextMessage{}, ErrHostedWatch
	}
	if w.Status != store.WatchActive {
		return NextMessage{Watch: w}, nil
	}
	todo, err := s.actionable(ctx, w)
	if err != nil {
		return NextMessage{Watch: w}, err
	}
	msg, err := s.compose(ctx, client, w, todo)
	if err != nil || msg.empty() {
		return NextMessage{Watch: w}, err
	}
	row, err := s.recordMessage(context.WithoutCancel(ctx), w, msg.text, msg.summary, deliverRoutine.source(), msg.rows)
	if err != nil {
		return NextMessage{}, err
	}
	s.work.start(id, s.now().Add(selfWorkDeadline))
	s.schedule.stir(id)
	return NextMessage{Watch: w, Message: &row}, nil
}

func (s *Service) changesOf(w store.Watch) (<-chan struct{}, func()) {
	woke := make(chan struct{}, 1)
	if s.bus == nil {
		return woke, func() {}
	}
	repo, number := w.Repo(), w.Number
	unsubscribe := s.bus.Subscribe(func(e events.Event) {
		if !wakes(e, repo, number) {
			return
		}
		select {
		case woke <- struct{}{}:
		default:
		}
	})
	return woke, unsubscribe
}

func wakes(e events.Event, repo string, number int) bool {
	if e.Repo != repo || e.Number != number {
		return false
	}
	return e.Type == events.WatchActivity || e.Type == events.WatchStopped || e.Type == events.WatchReady
}
