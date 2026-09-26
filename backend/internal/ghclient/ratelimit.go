package ghclient

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/go-github/v91/github"
)

var ErrPaused = errors.New("github rate limit nearly used, polling paused until it resets")

type RateGuard struct {
	Floor int
	Log   *slog.Logger
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error

	mu    sync.Mutex
	until time.Time
}

func (g *RateGuard) Pausing(now func() time.Time) *RateGuard {
	if g == nil {
		return nil
	}
	p := &RateGuard{Floor: g.Floor, Log: g.Log, Now: now}
	p.Sleep = p.hold
	return p
}

func (g *RateGuard) hold(_ context.Context, d time.Duration) error {
	until := g.now().Add(d)
	g.mu.Lock()
	if until.After(g.until) {
		g.until = until
	}
	g.mu.Unlock()
	g.log().Warn("polling paused, the GitHub rate limit is nearly used", "until", until)
	return ErrPaused
}

func (g *RateGuard) Paused() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.now().Before(g.until)
}

func (g *RateGuard) After(ctx context.Context, resp *github.Response, err error) error {
	if g == nil {
		return err
	}
	var (
		rateErr  *github.RateLimitError
		abuseErr *github.AbuseRateLimitError
	)
	switch {
	case errors.As(err, &rateErr):
		g.log().Warn("rate limited, waiting for reset", "reset", rateErr.Rate.Reset.Time)
		if serr := g.sleep(ctx, g.untilReset(rateErr.Rate.Reset.Time)); serr != nil {
			return serr
		}
		return err
	case errors.As(err, &abuseErr):
		d := time.Minute
		if abuseErr.RetryAfter != nil {
			d = *abuseErr.RetryAfter
		}
		g.log().Warn("secondary rate limit, waiting", "retry_after", d)
		if serr := g.sleep(ctx, d); serr != nil {
			return serr
		}
		return err
	case err != nil:
		return err
	}
	if resp != nil && resp.Rate.Limit > 0 && resp.Rate.Remaining < g.Floor {
		g.log().Warn("rate limit nearly used, waiting for reset",
			"remaining", resp.Rate.Remaining, "reset", resp.Rate.Reset.Time)
		return g.sleep(ctx, g.untilReset(resp.Rate.Reset.Time))
	}
	return nil
}

func (g *RateGuard) log() *slog.Logger {
	if g.Log != nil {
		return g.Log
	}
	return slog.New(slog.DiscardHandler)
}

func (g *RateGuard) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

func (g *RateGuard) sleep(ctx context.Context, d time.Duration) error {
	if g.Sleep != nil {
		return g.Sleep(ctx, d)
	}
	return SleepCtx(ctx, d)
}

func (g *RateGuard) untilReset(reset time.Time) time.Duration {
	d := max(reset.Sub(g.now())+time.Second, time.Second)
	return d
}

func SleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
