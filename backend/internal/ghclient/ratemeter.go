package ghclient

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"
)

type RateState string

const (
	RateUnknown RateState = "unknown"
	RateOK      RateState = "ok"
	RateLow     RateState = "low"
	RatePaused  RateState = "paused"
	RateSlowed  RateState = "slowed"
)

const lowShare = 10

const secondaryWait = time.Minute

const maxErrorPeek = 64 << 10

type RateStatus struct {
	State     RateState
	Limit     int
	Remaining int
	Reset     time.Time
	RetryAt   time.Time
}

var meteredResources = []string{"core", "graphql"}

var byPauseNearness = []RateState{RateOK, RateLow, RatePaused}

type RateMeter struct {
	Now func() time.Time

	mu      sync.Mutex
	budgets map[string]budget
	retryAt time.Time
}

var sharedRates = &RateMeter{}

func SharedRates() *RateMeter { return sharedRates }

func (m *RateMeter) Status(floor int) RateStatus {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	slowed := now.Before(m.retryAt)
	if len(m.budgets) == 0 && !slowed {
		return RateStatus{State: RateUnknown}
	}
	s := m.nearestToPause(now, floor)
	if slowed {
		s.State, s.RetryAt = RateSlowed, m.retryAt
	}
	return s
}

func (m *RateMeter) nearestToPause(now time.Time, floor int) RateStatus {
	var nearest RateStatus
	for _, resource := range meteredResources {
		b, ok := m.budgets[resource]
		if !ok {
			continue
		}
		s := b.status(now, floor)
		if slices.Index(byPauseNearness, s.State) > slices.Index(byPauseNearness, nearest.State) {
			nearest = s
		}
	}
	return nearest
}

func (m *RateMeter) observe(resp *http.Response) {
	now := m.now()
	resource, b, hasBudget := meteredBudget(resp.Header)
	retryAt, hasRetry := secondaryRetry(resp, now)
	m.mu.Lock()
	defer m.mu.Unlock()
	if hasBudget {
		m.record(resource, b)
	}
	if hasRetry && retryAt.After(m.retryAt) {
		m.retryAt = retryAt
	}
}

func (m *RateMeter) record(resource string, b budget) {
	held, seen := m.budgets[resource]
	olderWindow := seen && b.reset.Before(held.reset)
	sameWindowWithMore := seen && b.reset.Equal(held.reset) && b.remaining > held.remaining
	if olderWindow || sameWindowWithMore {
		return
	}
	if m.budgets == nil {
		m.budgets = make(map[string]budget, len(meteredResources))
	}
	m.budgets[resource] = b
}

func (m *RateMeter) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

type budget struct {
	limit     int
	remaining int
	reset     time.Time
}

func (b budget) status(now time.Time, floor int) RateStatus {
	s := RateStatus{Limit: b.limit, Remaining: b.remaining, Reset: b.reset}
	if !now.Before(b.reset) {
		s.Remaining, s.Reset = b.limit, time.Time{}
	}
	switch {
	case s.Remaining < floor:
		s.State = RatePaused
	case s.Remaining*lowShare <= s.Limit:
		s.State = RateLow
	default:
		s.State = RateOK
	}
	return s
}

func meteredBudget(h http.Header) (string, budget, bool) {
	resource := h.Get("X-RateLimit-Resource")
	if !slices.Contains(meteredResources, resource) {
		return "", budget{}, false
	}
	limit, errLimit := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	remaining, errRemaining := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	reset, errReset := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	unreadable := errors.Join(errLimit, errRemaining, errReset) != nil
	if unreadable || limit <= 0 {
		return "", budget{}, false
	}
	return resource, budget{limit: limit, remaining: remaining, reset: time.Unix(reset, 0)}, true
}

func secondaryRetry(resp *http.Response, now time.Time) (time.Time, bool) {
	refusedForRate := resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests
	if !refusedForRate || resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return time.Time{}, false
	}
	if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
		return now.Add(time.Duration(seconds) * time.Second), true
	}
	if resp.StatusCode == http.StatusTooManyRequests || namesSecondaryLimit(resp) {
		return now.Add(secondaryWait), true
	}
	return time.Time{}, false
}

func namesSecondaryLimit(resp *http.Response) bool {
	if resp.Body == nil {
		return false
	}
	head, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorPeek))
	resp.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(head), resp.Body), Closer: resp.Body}
	if err != nil {
		return false
	}
	lower := bytes.ToLower(head)
	return bytes.Contains(lower, []byte("secondary rate limit")) || bytes.Contains(lower, []byte("secondary-rate-limits"))
}

type meteringTransport struct {
	base  http.RoundTripper
	meter *RateMeter
}

func (t *meteringTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	t.meter.observe(resp)
	return resp, nil
}
