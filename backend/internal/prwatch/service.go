package prwatch

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/go-github/v91/github"
	"golang.org/x/sync/errgroup"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/keyedlock"
	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/processalive"
	"github.com/deividfortuna/babysitter/internal/session"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/timex"
	"github.com/deividfortuna/babysitter/internal/watcher"
	"github.com/deividfortuna/babysitter/internal/worktree"
)

var (
	ErrNotOpen      = errors.New("the pull request is not open")
	ErrNoPushAccess = errors.New("the token cannot push to the head repository")
	ErrNoIdentity   = errors.New("git user.name and user.email are not set in the checkout")
	ErrWrongRepo    = errors.New("the origin of the checkout is not the head repository of the pull request")
	ErrNoAgent      = errors.New("the provider of the watch is not available in this daemon")
	ErrWrongBranch  = errors.New("the checkout is not on the head branch of the pull request")
	ErrNoCheckout   = errors.New("the watch needs a checkout")
)

const lostAccessAfter = 3

const passWidth = 4

type Checkouts interface {
	Ensure(ctx context.Context, repo string) (dir string, err error)
}

type Deps struct {
	Store         *store.Store
	NewClient     watcher.ClientFunc
	Git           worktree.Manager
	Checkouts     Checkouts
	Release       gitrelease.Git
	Agents        map[string]agent.Runner
	Host          session.Host
	Exe           string
	Notifications notify.Poster
	Log           *slog.Logger
	DataDir       string
	Guard         *ghclient.RateGuard
	Bus           *events.Bus
}

type Option func(*Service)

func WithInterval(d time.Duration) Option {
	return func(s *Service) { s.interval.Set(d) }
}

func WithMaxInterval(d time.Duration) Option {
	return func(s *Service) { s.maxInterval.Set(d) }
}

func WithHeartbeat(d time.Duration) Option {
	return func(s *Service) { s.heartbeat = d }
}

func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

func WithProcessAlive(alive func(pid int) bool) Option {
	return func(s *Service) { s.alive = alive }
}

type Service struct {
	store         *store.Store
	newClient     watcher.ClientFunc
	git           worktree.Manager
	checkouts     Checkouts
	rel           gitrelease.Git
	notifications notify.Poster
	log           *slog.Logger
	dataDir       string
	exe           string
	guard         *ghclient.RateGuard
	bus           *events.Bus
	agents        map[string]agent.Runner
	host          session.Host
	interval      *timex.Interval
	maxInterval   *timex.Interval
	schedule      *schedule
	heartbeat     time.Duration
	now           func() time.Time
	alive         func(pid int) bool
	kick          chan struct{}

	locks         keyedlock.Locks[int64]
	sessions      sessions
	sizes         registry[TerminalSize]
	sizeLocks     keyedlock.Locks[int64]
	turns         keyedQueues
	work          selfWork
	waiting       waiting
	rereviewTries rereviewTries

	bgMu sync.Mutex
	bg   context.Context
	wg   sync.WaitGroup
}

func New(d Deps, opts ...Option) *Service {
	s := &Service{
		store:         d.Store,
		newClient:     d.NewClient,
		git:           d.Git,
		checkouts:     d.Checkouts,
		rel:           d.Release,
		notifications: d.Notifications,
		log:           d.Log,
		dataDir:       d.DataDir,
		exe:           d.Exe,
		bus:           d.Bus,
		agents:        map[string]agent.Runner{},
		host:          d.Host,
		interval:      timex.NewInterval(3 * time.Minute),
		maxInterval:   timex.NewInterval(defaultMaxInterval),
		schedule:      newSchedule(time.Now),
		heartbeat:     time.Hour,
		now:           time.Now,
		alive:         processalive.Alive,
		kick:          make(chan struct{}, 1),

		bg: context.Background(),
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if s.host == nil {
		s.host = session.New()
	}
	if s.rel == nil {
		s.rel = gitrelease.New()
	}
	for provider, runner := range d.Agents {
		if p, ok := normalizeProvider(provider); ok {
			s.agents[p] = runner
		}
	}
	for _, o := range opts {
		o(s)
	}
	s.guard = d.Guard.Pausing(func() time.Time { return s.now() })
	return s
}

func (s *Service) afterWrite(ctx context.Context, resp *github.Response, err error) error {
	paused := s.guard.After(ctx, resp, err)
	if err == nil {
		return nil
	}
	return paused
}

func (s *Service) attended(w store.Watch) bool {
	return !s.hosted(w) || s.runs(w)
}

func (s *Service) hosted(w store.Watch) bool {
	return hostedProvider(w.Provider)
}

func (s *Service) runs(w store.Watch) bool {
	return s.agentFor(w.Provider) != nil
}

func (s *Service) Providers() []Provider {
	out := Catalog()
	for i := range out {
		out[i].Available = s.agents[out[i].ID] != nil
	}
	return out
}

func (s *Service) agentFor(provider string) agent.Runner {
	if p, ok := normalizeProvider(provider); ok {
		return s.agents[p]
	}
	return nil
}

func (s *Service) background() context.Context {
	s.bgMu.Lock()
	defer s.bgMu.Unlock()
	return s.bg
}

func (s *Service) spawn(fn func()) {
	s.wg.Go(fn)
}

func (s *Service) Run(ctx context.Context) error {
	s.bgMu.Lock()
	s.bg = ctx
	s.bgMu.Unlock()
	s.recover(ctx)
	tuned := s.cadence()
	reached := s.pass(ctx)
	timer := time.NewTimer(s.untilNextPoll(reached))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			s.wg.Wait()
			s.stopSessions()
			return ctx.Err()
		case <-timer.C:
			reached = s.pass(ctx)
		case <-s.kick:
			reached = s.pass(ctx)
		case <-s.interval.Retuned():
			tuned = s.retune(tuned)
		case <-s.maxInterval.Retuned():
			tuned = s.retune(tuned)
		}
		timer.Reset(s.untilNextPoll(reached))
	}
}

func (s *Service) retune(tuned cadence) cadence {
	c := s.cadence()
	if c != tuned {
		s.schedule.restart()
	}
	return c
}

func (s *Service) Interval() time.Duration { return s.interval.Duration() }

func (s *Service) SetInterval(d time.Duration) { s.interval.Set(d) }

func (s *Service) MaxInterval() time.Duration { return s.maxInterval.Duration() }

func (s *Service) SetMaxInterval(d time.Duration) { s.maxInterval.Set(d) }

func (s *Service) cadence() cadence {
	shortest := s.Interval()
	return cadence{shortest: shortest, longest: max(s.MaxInterval(), shortest)}
}

func (s *Service) untilNextPoll(reached bool) time.Duration {
	c := s.cadence()
	stalled := !reached || s.guard.Paused()
	if stalled {
		return c.shortest
	}
	return s.schedule.untilNext(c)
}

func (s *Service) Kick(id int64) {
	s.schedule.wake(id)
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

func (s *Service) recover(ctx context.Context) {
	watches, err := s.store.ListWatches(ctx, store.ListWatchesOptions{Status: store.WatchActive})
	if err != nil {
		s.log.Error("list watches", "err", err)
		return
	}
	for _, w := range watches {
		if err := s.returnUndelivered(ctx, w); err != nil {
			s.log.Error("hand back an undelivered message", "watch", w.ID, "err", err)
		}
		if err := s.restoreWork(ctx, w); err != nil {
			s.log.Error("restore the work of a self agent", "watch", w.ID, "err", err)
		}
		items, err := s.store.UnreportedActivity(ctx, w.ID)
		if err != nil {
			s.log.Error("unreported activity", "watch", w.ID, "err", err)
			continue
		}
		if len(items) > 0 {
			s.report(ctx, w, items)
		}
		if !s.hostsSession(w) {
			continue
		}
		s.spawn(func() {
			unlock := s.locks.Lock(w.ID)
			defer unlock()
			s.closeCutTurn(s.background(), w)
			if _, err := s.ensureSession(s.background(), w); err != nil {
				s.agentFailed(s.background(), w, "start the agent session", err)
			}
		})
	}
}

func (s *Service) pass(ctx context.Context) bool {
	watches, err := s.store.ListWatches(ctx, store.ListWatchesOptions{Status: store.WatchActive})
	if err != nil {
		s.log.Error("list watches", "err", err)
		return false
	}
	client, err := s.newClient(ctx)
	if err != nil {
		s.log.Error("github client", "err", err)
		return false
	}
	s.schedule.keep(watchIDs(watches))
	c := s.cadence()
	var g errgroup.Group
	g.SetLimit(passWidth)
	for _, w := range watches {
		if ctx.Err() != nil || s.guard.Paused() {
			break
		}
		if !s.schedule.due(w.ID, c) {
			continue
		}
		g.Go(func() error {
			s.passOne(ctx, client, w)
			return nil
		})
	}
	_ = g.Wait()
	return true
}

func watchIDs(watches []store.Watch) map[int64]bool {
	out := make(map[int64]bool, len(watches))
	for _, w := range watches {
		out[w.ID] = true
	}
	return out
}

func (s *Service) passOne(ctx context.Context, client *github.Client, w store.Watch) {
	if ctx.Err() != nil || s.guard.Paused() {
		return
	}
	unlock := s.locks.Lock(w.ID)
	err := s.poll(ctx, client, w)
	unlock()
	if worthReporting(ctx, err) {
		s.log.Error("poll failed", "watch", w.ID, "pr", prLabel(w), "err", err)
	}
}

func worthReporting(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	return !errors.Is(err, ghclient.ErrPaused)
}

func (s *Service) Poll(ctx context.Context, id int64) error {
	client, err := s.newClient(ctx)
	if err != nil {
		return err
	}
	return s.pollWatch(ctx, client, id)
}

func (s *Service) pollWatch(ctx context.Context, client *github.Client, id int64) error {
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return err
	}
	if w.Status != store.WatchActive {
		return nil
	}
	unlock := s.locks.Lock(w.ID)
	defer unlock()
	return s.poll(ctx, client, w)
}
