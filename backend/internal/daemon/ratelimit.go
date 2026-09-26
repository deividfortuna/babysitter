package daemon

import (
	"time"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/httpd"
)

func rateLimit(s ghclient.RateStatus) httpd.RateLimit {
	return httpd.RateLimit{
		State:     string(s.State),
		Limit:     s.Limit,
		Remaining: s.Remaining,
		ResetAt:   instant(s.Reset),
		RetryAt:   instant(s.RetryAt),
	}
}

func instant(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
