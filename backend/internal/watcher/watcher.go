package watcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/timex"
)

type ClientFunc func(ctx context.Context) (*github.Client, error)

type Store interface {
	ListRepos(ctx context.Context) ([]store.Repo, error)
	OpenPRs(ctx context.Context, repoID int64) ([]store.PullRequest, error)
	UpsertPR(ctx context.Context, pr store.PullRequest) error
	SetRepoSync(ctx context.Context, id int64, at time.Time, syncErr error) error
}

const rateFloor = 10

const defaultCheckTTL = 10 * time.Minute

const firstCheckWait = time.Minute

type checkKey struct {
	repoID int64
	number int
	sha    string
}

type checkRead struct {
	at     time.Time
	wait   time.Duration
	status checks.CIStatus
}

type Watcher struct {
	store     Store
	newClient ClientFunc
	interval  *timex.Interval
	checkTTL  time.Duration
	log       *slog.Logger
	now       func() time.Time
	sleep     func(ctx context.Context, d time.Duration) error
	kick      chan struct{}
	guard     *ghclient.RateGuard

	checksMu         sync.Mutex
	checked          map[checkKey]checkRead
	longestCheckWait time.Duration

	afterPass func(ctx context.Context)
}

type Option func(*Watcher)

func WithInterval(d time.Duration) Option {
	return func(w *Watcher) { w.interval.Set(d) }
}

func WithCheckTTL(d time.Duration) Option {
	return func(w *Watcher) { w.checkTTL = d }
}

func WithLongestCheckWait(d time.Duration) Option {
	return func(w *Watcher) { w.SetLongestCheckWait(d) }
}

func WithLogger(l *slog.Logger) Option {
	return func(w *Watcher) { w.log = l }
}

func WithAfterPass(fn func(ctx context.Context)) Option {
	return func(w *Watcher) { w.afterPass = fn }
}

func WithClock(now func() time.Time, sleep func(context.Context, time.Duration) error) Option {
	return func(w *Watcher) {
		w.now = now
		w.sleep = sleep
	}
}

func New(st Store, newClient ClientFunc, opts ...Option) *Watcher {
	w := &Watcher{
		store:     st,
		newClient: newClient,
		interval:  timex.NewInterval(time.Minute),
		checkTTL:  defaultCheckTTL,
		log:       slog.New(slog.DiscardHandler),
		now:       time.Now,
		sleep:     ghclient.SleepCtx,
		kick:      make(chan struct{}, 1),
		checked:   map[checkKey]checkRead{},

		longestCheckWait: store.DefaultSettings().WatchMaxInterval,
	}
	for _, o := range opts {
		o(w)
	}
	w.guard = &ghclient.RateGuard{Floor: rateFloor, Log: w.log, Now: w.now, Sleep: w.sleep}
	return w
}

func (w *Watcher) Guard() *ghclient.RateGuard { return w.guard }

func (w *Watcher) Run(ctx context.Context) error {
	w.runPass(ctx)
	ticker := time.NewTicker(w.Interval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.runPass(ctx)
		case <-w.kick:
			w.forgetPendingChecks()
			w.runPass(ctx)
			ticker.Reset(w.Interval())
		case <-w.interval.Retuned():
			ticker.Reset(w.Interval())
		}
	}
}

func (w *Watcher) Interval() time.Duration { return w.interval.Duration() }

func (w *Watcher) SetInterval(d time.Duration) { w.interval.Set(d) }

func (w *Watcher) SetLongestCheckWait(d time.Duration) {
	if d <= 0 {
		return
	}
	w.checksMu.Lock()
	defer w.checksMu.Unlock()
	w.longestCheckWait = d
}

func (w *Watcher) Kick() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

func (w *Watcher) runPass(ctx context.Context) {
	if err := w.SyncAll(ctx); err != nil && ctx.Err() == nil {
		w.log.Error("sync pass finished with errors", "err", err)
	}
	if w.afterPass != nil && ctx.Err() == nil {
		w.afterPass(ctx)
	}
}

func (w *Watcher) SyncAll(ctx context.Context) error {
	repos, err := w.store.ListRepos(ctx)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		w.log.Info("no repositories are watched")
		return nil
	}
	client, err := w.newClient(ctx)
	if err != nil {
		return err
	}

	var errs []error
	for _, repo := range repos {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		start := w.now()
		err := w.syncRepo(ctx, client, repo)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			err = redact.Err(err)
			w.log.Error("sync failed", "repo", repo.FullName(), "err", err)
			errs = append(errs, fmt.Errorf("%s: %w", repo.FullName(), err))
		} else {
			w.log.Info("synced", "repo", repo.FullName(), "took", w.now().Sub(start).Round(time.Millisecond))
		}
		if serr := w.store.SetRepoSync(ctx, repo.ID, w.now(), err); serr != nil {
			errs = append(errs, serr)
		}
	}
	return errors.Join(errs...)
}
