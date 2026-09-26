package httpd

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func TestRateLimitAnswersTheBudgetOfTheToken(t *testing.T) {
	reset := time.Date(2026, 9, 24, 12, 38, 0, 0, time.UTC)
	h := NewRouter(Deps{Log: testutil.Logger(t), Bus: events.NewBus(), RateLimit: func() RateLimit {
		return RateLimit{State: "low", Limit: 5000, Remaining: 312, ResetAt: &reset}
	}})

	var got RateLimit
	if rec := call(t, h, http.MethodGet, "/ratelimit", "", &got); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got.State != "low" || got.Limit != 5000 || got.Remaining != 312 || got.ResetAt == nil || !got.ResetAt.Equal(reset) {
		t.Fatalf("rate limit = %+v, want 312 of 5000 left until %s", got, reset)
	}
}

func TestRateLimitLeavesOutTheInstantsItDoesNotKnow(t *testing.T) {
	h := NewRouter(Deps{Log: testutil.Logger(t), Bus: events.NewBus(), RateLimit: func() RateLimit {
		return RateLimit{State: "ok", Limit: 5000, Remaining: 5000}
	}})

	rec := call(t, h, http.MethodGet, "/ratelimit", "", nil)

	body := rec.Body.String()
	if strings.Contains(body, "resetAt") || strings.Contains(body, "retryAt") {
		t.Fatalf("body = %s, want no reset and no retry", body)
	}
}

func TestRateLimitIsUnknownWithoutAMeter(t *testing.T) {
	h := NewRouter(Deps{Log: testutil.Logger(t), Bus: events.NewBus()})

	var got RateLimit
	if rec := call(t, h, http.MethodGet, "/ratelimit", "", &got); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got.State != "unknown" {
		t.Fatalf("state = %q, want unknown", got.State)
	}
}
