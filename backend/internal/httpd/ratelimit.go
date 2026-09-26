package httpd

import (
	"net/http"
	"time"
)

type RateLimit struct {
	State     string     `json:"state" enum:"unknown,ok,low,paused,slowed" description:"How much is left. unknown: GitHub has not answered yet; ok: more than a tenth; low: a tenth or less; paused: the polls wait for the reset; slowed: GitHub refused a call for its secondary limit and the polls wait for the retry"`
	Limit     int        `json:"limit" description:"Requests the token may make in one window; 0 while unknown"`
	Remaining int        `json:"remaining" description:"Requests left in the window"`
	ResetAt   *time.Time `json:"resetAt,omitempty" description:"When the window resets; absent while unknown and once the window passed"`
	RetryAt   *time.Time `json:"retryAt,omitempty" description:"When the secondary limit ends; only in state slowed"`
}

type RateLimitFunc func() RateLimit

func (a *api) handleRateLimit(w http.ResponseWriter, r *http.Request) {
	if a.rateLimit == nil {
		writeJSON(w, http.StatusOK, RateLimit{State: "unknown"})
		return
	}
	writeJSON(w, http.StatusOK, a.rateLimit())
}
