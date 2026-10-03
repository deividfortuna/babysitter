package agent

import (
	"encoding/json"
	"testing"
	"time"
)

func TestResetTimeReadsTheClockOfTheLimitMessage(t *testing.T) {
	t.Parallel()
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Skip("no time zone data")
	}
	now := time.Date(2026, 10, 4, 13, 20, 0, 0, london)
	cases := []struct {
		text string
		want time.Time
	}{
		{"You've hit your session limit · resets 3pm (Europe/London)", time.Date(2026, 10, 4, 15, 0, 0, 0, london)},
		{"You've hit your session limit · resets 3:45pm (Europe/London)", time.Date(2026, 10, 4, 15, 45, 0, 0, london)},
		{"You've hit your session limit · resets 1pm (Europe/London)", time.Date(2026, 10, 5, 13, 0, 0, 0, london)},
		{"You've hit your session limit · resets 12am (Europe/London)", time.Date(2026, 10, 5, 0, 0, 0, 0, london)},
		{"You've hit your weekly limit · resets Oct 7, 3:30pm (Europe/London)", time.Date(2026, 10, 7, 15, 30, 0, 0, london)},
		{"You've hit your weekly limit · resets Oct 7 at 9am (Europe/London)", time.Date(2026, 10, 7, 9, 0, 0, 0, london)},
		{"You've hit your weekly limit · resets Jan 2, 2027, 8am (Europe/London)", time.Date(2027, 1, 2, 8, 0, 0, 0, london)},
		{"You've hit your weekly limit · resets Jan 2, 8am (Europe/London)", time.Date(2027, 1, 2, 8, 0, 0, 0, london)},
		{"You've hit your session limit · resets 3pm (America/New_York)", time.Date(2026, 10, 4, 15, 0, 0, 0, mustZone(t, "America/New_York"))},
		{"You've hit your session limit · resets 3pm", time.Date(2026, 10, 4, 15, 0, 0, 0, london)},
		{"You've hit your session limit · resets 3pm (Nowhere/Land)", time.Date(2026, 10, 4, 15, 0, 0, 0, london)},
	}
	for _, tc := range cases {
		got, ok := resetTime(tc.text, now)
		if !ok || !got.Equal(tc.want) {
			t.Errorf("resetTime(%q) = %v, %v, want %v", tc.text, got, ok, tc.want)
		}
	}
	for _, text := range []string{"", "API Error: Request rejected (429)", "resets 13pm", "resets 3:75pm", "resets Foo 7, 3pm"} {
		if got, ok := resetTime(text, now); ok {
			t.Errorf("resetTime(%q) = %v, want no reset", text, got)
		}
	}
}

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skip("no time zone data")
	}
	return loc
}

func TestLimitOfReadsOnlyARateLimitFailure(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 4, 13, 20, 0, 0, time.UTC)
	cases := []struct {
		event, payload string
		want           time.Time
		limited        bool
	}{
		{EventStopFailure, `{"error":"rate_limit","last_assistant_message":"You've hit your session limit · resets 3pm (UTC)"}`, time.Date(2026, 10, 4, 15, 0, 0, 0, time.UTC), true},
		{EventStopFailure, `{"error":"rate_limit","error_details":"resets 4pm (UTC)"}`, time.Date(2026, 10, 4, 16, 0, 0, 0, time.UTC), true},
		{EventStopFailure, `{"error":"rate_limit","last_assistant_message":"API Error: Request rejected (429)"}`, now.Add(limitRetry), true},
		{EventStopFailure, `{"error":"overloaded","last_assistant_message":"resets 3pm"}`, time.Time{}, false},
		{EventStop, `{"error":"rate_limit","last_assistant_message":"resets 3pm"}`, time.Time{}, false},
	}
	for _, tc := range cases {
		got, limited := LimitOf(tc.event, json.RawMessage(tc.payload), now)
		if limited != tc.limited || !got.Equal(tc.want) {
			t.Errorf("LimitOf(%s, %s) = %v, %v, want %v, %v", tc.event, tc.payload, got, limited, tc.want, tc.limited)
		}
	}
}

func TestResetTimeKeepsTheClockOnADaylightSavingDay(t *testing.T) {
	t.Parallel()
	london := mustZone(t, "Europe/London")
	cases := []struct {
		now  time.Time
		text string
		want time.Time
	}{
		{time.Date(2026, 3, 29, 10, 0, 0, 0, london), "resets 3pm (Europe/London)", time.Date(2026, 3, 29, 15, 0, 0, 0, london)},
		{time.Date(2026, 3, 28, 10, 0, 0, 0, london), "resets Mar 29, 3pm (Europe/London)", time.Date(2026, 3, 29, 15, 0, 0, 0, london)},
		{time.Date(2026, 10, 25, 10, 0, 0, 0, london), "resets 3pm (Europe/London)", time.Date(2026, 10, 25, 15, 0, 0, 0, london)},
		{time.Date(2026, 3, 28, 16, 0, 0, 0, london), "resets 3pm (Europe/London)", time.Date(2026, 3, 29, 15, 0, 0, 0, london)},
	}
	for _, tc := range cases {
		got, ok := resetTime(tc.text, tc.now)
		if !ok || !got.Equal(tc.want) {
			t.Errorf("resetTime(%q) at %v = %v, %v, want %v", tc.text, tc.now, got, ok, tc.want)
		}
	}
}
