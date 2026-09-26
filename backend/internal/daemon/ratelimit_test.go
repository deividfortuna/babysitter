package daemon

import (
	"reflect"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/httpd"
)

func TestRateLimitCarriesTheResetOfTheWindow(t *testing.T) {
	t.Parallel()
	reset := time.Date(2026, 9, 24, 12, 38, 0, 0, time.UTC)

	got := rateLimit(ghclient.RateStatus{State: ghclient.RateLow, Limit: 5000, Remaining: 312, Reset: reset})

	want := httpd.RateLimit{State: "low", Limit: 5000, Remaining: 312, ResetAt: &reset}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rate limit = %+v, want %+v", got, want)
	}
}

func TestRateLimitCarriesTheRetryOfASecondaryLimit(t *testing.T) {
	t.Parallel()
	reset := time.Date(2026, 9, 24, 12, 40, 0, 0, time.UTC)
	retry := time.Date(2026, 9, 24, 12, 1, 0, 0, time.UTC)

	got := rateLimit(ghclient.RateStatus{State: ghclient.RateSlowed, Limit: 5000, Remaining: 2880, Reset: reset, RetryAt: retry})

	want := httpd.RateLimit{State: "slowed", Limit: 5000, Remaining: 2880, ResetAt: &reset, RetryAt: &retry}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rate limit = %+v, want %+v", got, want)
	}
}

func TestRateLimitLeavesOutTheInstantsNobodyKnows(t *testing.T) {
	t.Parallel()

	got := rateLimit(ghclient.RateStatus{State: ghclient.RateUnknown})

	want := httpd.RateLimit{State: "unknown"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rate limit = %+v, want %+v", got, want)
	}
}
