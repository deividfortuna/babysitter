package ghclient

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"
)

func TestRateGuardNilPassesError(t *testing.T) {
	t.Parallel()
	var g *RateGuard
	want := errors.New("boom")
	if err := g.After(context.Background(), nil, want); !errors.Is(err, want) {
		t.Fatalf("After() = %v, want %v", err, want)
	}
	if err := g.After(context.Background(), &github.Response{Rate: github.Rate{Limit: 10, Remaining: 0}}, nil); err != nil {
		t.Fatalf("After() = %v, want nil", err)
	}
}

func TestRateGuardWaitsOnLowBudget(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	var slept []time.Duration
	g := &RateGuard{
		Floor: 10,
		Now:   func() time.Time { return now },
		Sleep: func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}
	resp := &github.Response{Rate: github.Rate{Limit: 5000, Remaining: 3, Reset: github.Timestamp{Time: now.Add(30 * time.Minute)}}}
	if err := g.After(context.Background(), resp, nil); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 1 || slept[0] != 30*time.Minute+time.Second {
		t.Fatalf("slept = %v", slept)
	}

	slept = nil
	resp.Rate.Remaining = 10
	if err := g.After(context.Background(), resp, nil); err != nil || len(slept) != 0 {
		t.Fatalf("After() = %v, slept = %v", err, slept)
	}
}

func TestRateGuardWaitsOnRateLimitError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	var slept []time.Duration
	g := &RateGuard{
		Now:   func() time.Time { return now },
		Sleep: func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}
	rateErr := &github.RateLimitError{
		Rate:     github.Rate{Reset: github.Timestamp{Time: now.Add(-time.Hour)}},
		Response: &http.Response{StatusCode: http.StatusForbidden},
	}
	if err := g.After(context.Background(), nil, rateErr); !errors.Is(err, rateErr) {
		t.Fatalf("After() = %v, want the rate error", err)
	}
	if len(slept) != 1 || slept[0] != time.Second {
		t.Fatalf("slept = %v", slept)
	}

	slept = nil
	retry := 5 * time.Second
	abuse := &github.AbuseRateLimitError{RetryAfter: &retry, Response: &http.Response{StatusCode: http.StatusForbidden}}
	if err := g.After(context.Background(), nil, abuse); !errors.Is(err, abuse) {
		t.Fatalf("After() = %v, want the abuse error", err)
	}
	if len(slept) != 1 || slept[0] != retry {
		t.Fatalf("slept = %v", slept)
	}

	g.Sleep = func(ctx context.Context, _ time.Duration) error { return context.Canceled }
	if err := g.After(context.Background(), nil, rateErr); !errors.Is(err, context.Canceled) {
		t.Fatalf("After() = %v, want canceled", err)
	}
}
