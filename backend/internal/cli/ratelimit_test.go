package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRateLimitPrintsTheBudgetAndWhenItResets(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	reset := time.Now().Add(21*time.Minute + 30*time.Second).UTC()
	d.rateLimit = fmt.Sprintf(`{"state":"low","limit":5000,"remaining":312,"resetAt":%q}`, reset.Format(time.RFC3339))

	out, err := runAgainstDaemon(t, d, "ratelimit")
	if err != nil {
		t.Fatalf("ratelimit error = %v", err)
	}
	for _, want := range []string{"nearly used", "312 of 5000", "in 22m", reset.Local().Format("15:04")} {
		if !strings.Contains(out, want) {
			t.Fatalf("ratelimit = %q, want it to carry %q", out, want)
		}
	}
}

func TestRateLimitPrintsTheRetryOfASecondaryLimit(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	now := time.Now().UTC()
	d.rateLimit = fmt.Sprintf(`{"state":"slowed","limit":5000,"remaining":2880,"resetAt":%q,"retryAt":%q}`,
		now.Add(40*time.Minute).Format(time.RFC3339), now.Add(50*time.Second).Format(time.RFC3339))

	out, err := runAgainstDaemon(t, d, "ratelimit")
	if err != nil {
		t.Fatalf("ratelimit error = %v", err)
	}
	for _, want := range []string{"GitHub asked to slow down", "2880 of 5000", "Retry", "in 1m"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ratelimit = %q, want it to carry %q", out, want)
		}
	}
}

func TestRateLimitSaysSoWhileGitHubHasNotAnswered(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runAgainstDaemon(t, d, "ratelimit")
	if err != nil {
		t.Fatalf("ratelimit error = %v", err)
	}
	if !strings.Contains(out, "GitHub has not answered a call yet") {
		t.Fatalf("ratelimit = %q, want it to say the budget is not known", out)
	}
	if strings.Contains(out, "Left") {
		t.Fatalf("ratelimit = %q, want no count it does not know", out)
	}
}

func TestRateLimitPrintsTheAnswerOfTheDaemonAsJSON(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.rateLimit = `{"state":"paused","limit":5000,"remaining":3,"resetAt":"2026-09-24T12:12:00Z"}`

	out, err := runAgainstDaemon(t, d, "ratelimit", "--output", "json")
	if err != nil {
		t.Fatalf("ratelimit error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output = %q, want JSON: %v", out, err)
	}
	if got["state"] != "paused" || got["remaining"] != float64(3) || got["resetAt"] != "2026-09-24T12:12:00Z" {
		t.Fatalf("output = %v, want the answer of the daemon", got)
	}
}
