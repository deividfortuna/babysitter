package ghclient

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

var meterEpoch = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

type rateAPI struct {
	mu     sync.Mutex
	header map[string]string
	status int
	body   string
}

func (a *rateAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, v := range a.header {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", "application/json")
	if a.status != 0 {
		w.WriteHeader(a.status)
		fmt.Fprint(w, a.body)
		return
	}
	fmt.Fprint(w, `{"login":"octocat"}`)
}

func (a *rateAPI) answer(status int, body string, header map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status, a.body, a.header = status, body, header
}

func coreHeader(limit, remaining int, reset time.Time) map[string]string {
	return rateHeader("core", limit, remaining, reset)
}

func rateHeader(resource string, limit, remaining int, reset time.Time) map[string]string {
	return map[string]string{
		"X-RateLimit-Resource":  resource,
		"X-RateLimit-Limit":     strconv.Itoa(limit),
		"X-RateLimit-Remaining": strconv.Itoa(remaining),
		"X-RateLimit-Reset":     strconv.FormatInt(reset.Unix(), 10),
	}
}

type meterClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *meterClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *meterClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func meteredClient(t *testing.T) (*github.Client, *RateMeter, *rateAPI, *meterClock) {
	t.Helper()
	api := &rateAPI{}
	clock := &meterClock{now: meterEpoch}
	meter := &RateMeter{Now: clock.Now}
	c := ghfake.Serve(t, api).Client(t, github.WithTransport(&meteringTransport{meter: meter}))
	return c, meter, api, clock
}

func callUser(t *testing.T, c *github.Client) error {
	t.Helper()
	_, _, err := c.Users.Get(context.Background(), "")
	return err
}

func TestRateMeterIsUnknownBeforeGitHubAnswers(t *testing.T) {
	_, meter, _, _ := meteredClient(t)

	got := meter.Status(10)

	if got.State != RateUnknown {
		t.Fatalf("state = %q, want unknown", got.State)
	}
}

func TestRateMeterReadsTheCoreBudgetOfEachAnswer(t *testing.T) {
	c, meter, api, _ := meteredClient(t)
	reset := meterEpoch.Add(38 * time.Minute)
	api.answer(0, "", coreHeader(5000, 4212, reset))

	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	got := meter.Status(10)
	want := RateStatus{State: RateOK, Limit: 5000, Remaining: 4212, Reset: reset}
	if !sameStatus(got, want) {
		t.Fatalf("status = %+v, want %+v", got, want)
	}
}

func TestRateMeterShowsTheCoreBudgetWhileGraphQLHasMoreLeft(t *testing.T) {
	c, meter, api, _ := meteredClient(t)
	reset := meterEpoch.Add(time.Hour)
	api.answer(0, "", coreHeader(5000, 4000, reset))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	api.answer(0, "", rateHeader("graphql", 5000, 4800, reset))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	if got := meter.Status(10); got.Remaining != 4000 {
		t.Fatalf("remaining = %d, want 4000 of the core budget", got.Remaining)
	}
}

func TestRateMeterPausesOnTheGraphQLBudget(t *testing.T) {
	c, meter, api, _ := meteredClient(t)
	api.answer(0, "", coreHeader(5000, 4000, meterEpoch.Add(time.Hour)))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	graphqlReset := meterEpoch.Add(20 * time.Minute)
	api.answer(0, "", rateHeader("graphql", 5000, 9, graphqlReset))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	got := meter.Status(10)
	want := RateStatus{State: RatePaused, Limit: 5000, Remaining: 9, Reset: graphqlReset}
	if !sameStatus(got, want) {
		t.Fatalf("status = %+v, want %+v", got, want)
	}
}

func TestRateMeterLeavesTheSearchBudgetOut(t *testing.T) {
	c, meter, api, _ := meteredClient(t)
	reset := meterEpoch.Add(time.Hour)
	api.answer(0, "", coreHeader(5000, 4000, reset))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	api.answer(0, "", rateHeader("search", 30, 3, reset))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	if got := meter.Status(10); got.Remaining != 4000 {
		t.Fatalf("remaining = %d, want 4000 of the core budget", got.Remaining)
	}
}

func TestRateMeterKeepsTheLowestCountOfOneWindow(t *testing.T) {
	c, meter, api, _ := meteredClient(t)
	reset := meterEpoch.Add(time.Hour)
	api.answer(0, "", coreHeader(5000, 4000, reset))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	api.answer(0, "", coreHeader(5000, 4005, reset))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	if got := meter.Status(10); got.Remaining != 4000 {
		t.Fatalf("remaining = %d, want 4000", got.Remaining)
	}
}

func TestRateMeterTakesTheCountOfANewWindow(t *testing.T) {
	c, meter, api, clock := meteredClient(t)
	api.answer(0, "", coreHeader(5000, 12, meterEpoch.Add(time.Minute)))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	clock.advance(2 * time.Minute)
	next := meterEpoch.Add(62 * time.Minute)
	api.answer(0, "", coreHeader(5000, 4999, next))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	got := meter.Status(10)
	if got.Remaining != 4999 || !got.Reset.Equal(next) {
		t.Fatalf("status = %+v, want 4999 left until %s", got, next)
	}
}

func TestRateMeterKeepsTheNewWindowWhenAnAnswerOfTheOldOneArrivesLast(t *testing.T) {
	c, meter, api, clock := meteredClient(t)
	clock.advance(2 * time.Minute)
	next := meterEpoch.Add(62 * time.Minute)
	api.answer(0, "", coreHeader(5000, 4999, next))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	api.answer(0, "", coreHeader(5000, 12, meterEpoch.Add(time.Minute)))
	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	got := meter.Status(10)
	want := RateStatus{State: RateOK, Limit: 5000, Remaining: 4999, Reset: next}
	if !sameStatus(got, want) {
		t.Fatalf("status = %+v, want %+v", got, want)
	}
}

func TestRateMeterNamesHowMuchIsLeft(t *testing.T) {
	tests := []struct {
		name      string
		remaining int
		want      RateState
	}{
		{name: "plenty", remaining: 4212, want: RateOK},
		{name: "a tenth left", remaining: 500, want: RateLow},
		{name: "nearly used", remaining: 312, want: RateLow},
		{name: "at the floor", remaining: 10, want: RateLow},
		{name: "under the floor", remaining: 9, want: RatePaused},
		{name: "used", remaining: 0, want: RatePaused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, meter, api, _ := meteredClient(t)
			api.answer(0, "", coreHeader(5000, tt.remaining, meterEpoch.Add(20*time.Minute)))
			_ = callUser(t, c)

			if got := meter.Status(10); got.State != tt.want {
				t.Fatalf("state = %q, want %q", got.State, tt.want)
			}
		})
	}
}

func TestRateMeterCountsTheWholeBudgetOnceTheWindowReset(t *testing.T) {
	c, meter, api, clock := meteredClient(t)
	api.answer(0, "", coreHeader(5000, 3, meterEpoch.Add(12*time.Minute)))
	_ = callUser(t, c)

	clock.advance(12 * time.Minute)

	got := meter.Status(10)
	want := RateStatus{State: RateOK, Limit: 5000, Remaining: 5000}
	if !sameStatus(got, want) {
		t.Fatalf("status = %+v, want %+v", got, want)
	}
}

func TestRateMeterReadsTheSecondaryLimit(t *testing.T) {
	budget := coreHeader(5000, 2880, meterEpoch.Add(40*time.Minute))
	secondary := `{"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again.","documentation_url":"https://docs.github.com/rest/overview/rate-limits-for-the-rest-api#about-secondary-rate-limits"}`
	tests := []struct {
		name       string
		status     int
		body       string
		retryAfter string
		want       time.Time
	}{
		{name: "retry after", status: http.StatusForbidden, body: secondary, retryAfter: "30", want: meterEpoch.Add(30 * time.Second)},
		{name: "message alone", status: http.StatusForbidden, body: secondary, want: meterEpoch.Add(time.Minute)},
		{name: "too many requests", status: http.StatusTooManyRequests, body: `{"message":"slow down"}`, want: meterEpoch.Add(time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, meter, api, _ := meteredClient(t)
			header := maps.Clone(budget)
			if tt.retryAfter != "" {
				header["Retry-After"] = tt.retryAfter
			}
			api.answer(tt.status, tt.body, header)

			err := callUser(t, c)

			got := meter.Status(10)
			if got.State != RateSlowed || !got.RetryAt.Equal(tt.want) {
				t.Fatalf("status = %+v, want slowed until %s", got, tt.want)
			}
			if got.Remaining != 2880 {
				t.Fatalf("remaining = %d, want the core budget beside the secondary limit", got.Remaining)
			}
			var abuse *github.AbuseRateLimitError
			if tt.body == secondary && !errors.As(err, &abuse) {
				t.Fatalf("error = %v, want the secondary limit error of the body the meter read", err)
			}
		})
	}
}

func TestRateMeterEndsTheSecondaryLimitAtItsRetry(t *testing.T) {
	c, meter, api, clock := meteredClient(t)
	header := coreHeader(5000, 2880, meterEpoch.Add(40*time.Minute))
	header["Retry-After"] = "60"
	api.answer(http.StatusForbidden, `{"message":"secondary rate limit"}`, header)
	_ = callUser(t, c)

	clock.advance(time.Minute)

	if got := meter.Status(10); got.State != RateOK || !got.RetryAt.IsZero() {
		t.Fatalf("status = %+v, want ok with no retry", got)
	}
}

func TestRateMeterPutsTheSecondaryRetryBeforeALowBudget(t *testing.T) {
	c, meter, api, _ := meteredClient(t)
	header := coreHeader(5000, 9, meterEpoch.Add(time.Hour))
	header["Retry-After"] = "60"
	api.answer(http.StatusForbidden, `{"message":"You have exceeded a secondary rate limit."}`, header)

	_ = callUser(t, c)

	got := meter.Status(10)
	if got.State != RateSlowed || !got.RetryAt.Equal(meterEpoch.Add(time.Minute)) {
		t.Fatalf("status = %+v, want slowed until %s as the guard waits", got, meterEpoch.Add(time.Minute))
	}
}

func TestRateMeterTellsAForbiddenCallFromALimit(t *testing.T) {
	c, meter, api, _ := meteredClient(t)
	api.answer(http.StatusForbidden, `{"message":"Resource not accessible by integration"}`, coreHeader(5000, 4000, meterEpoch.Add(time.Hour)))

	_ = callUser(t, c)

	if got := meter.Status(10); got.State != RateOK {
		t.Fatalf("state = %q, want ok: a refused permission is no rate limit", got.State)
	}
}

func TestRateMeterCountsAUsedBudgetAsAPauseNotASlowDown(t *testing.T) {
	c, meter, api, _ := meteredClient(t)
	api.answer(http.StatusForbidden, `{"message":"API rate limit exceeded"}`, coreHeader(5000, 0, meterEpoch.Add(12*time.Minute)))

	_ = callUser(t, c)

	if got := meter.Status(10); got.State != RatePaused {
		t.Fatalf("state = %q, want paused", got.State)
	}
}

func TestNewClientsFeedTheSharedMeter(t *testing.T) {
	api := &rateAPI{}
	reset := time.Now().Add(time.Hour).Truncate(time.Second)
	api.answer(0, "", coreHeader(5000, 4321, reset))
	c := ghfake.Serve(t, api).Client(t, github.WithTransport(sharedTransport()))

	if err := callUser(t, c); err != nil {
		t.Fatal(err)
	}

	if got := SharedRates().Status(10); got.Remaining != 4321 {
		t.Fatalf("remaining = %d, want 4321", got.Remaining)
	}
}

func sameStatus(a, b RateStatus) bool {
	return a.State == b.State && a.Limit == b.Limit && a.Remaining == b.Remaining &&
		a.Reset.Equal(b.Reset) && a.RetryAt.Equal(b.RetryAt)
}
