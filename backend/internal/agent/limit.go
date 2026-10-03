package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const limitRetry = 30 * time.Minute

var resetPattern = regexp.MustCompile(`(?i)resets\s+(?:([a-z]{3})\s+(\d{1,2}),?\s+(?:(\d{4}),?\s+)?(?:at\s+)?)?(\d{1,2})(?::(\d{2}))?\s*([ap]m)(?:\s+\(([^)]+)\))?`)

func LimitOf(event string, payload json.RawMessage, now time.Time) (time.Time, bool) {
	limited := event == EventStopFailure && payloadField(payload, "error") == "rate_limit"
	if !limited {
		return time.Time{}, false
	}
	text := payloadField(payload, "last_assistant_message") + "\n" + payloadField(payload, "error_details")
	if reset, ok := resetTime(text, now); ok {
		return reset, true
	}
	return now.Add(limitRetry), true
}

func resetTime(text string, now time.Time) (time.Time, bool) {
	m := resetPattern.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}, false
	}
	month, monthDay, year, hour, minute, half, zone := m[1], m[2], m[3], m[4], m[5], m[6], m[7]
	clockHour, clockMinute, ok := clockOf(hour, minute, half)
	if !ok {
		return time.Time{}, false
	}
	local := now.In(zoneOf(zone, now.Location()))
	if month == "" {
		return nextClock(local, clockHour, clockMinute), true
	}
	return dateClock(local, month, monthDay, year, clockHour, clockMinute)
}

func clockOf(hour, minute, half string) (h, m int, ok bool) {
	h, err := strconv.Atoi(hour)
	onClockFace := err == nil && h >= 1 && h <= 12
	if !onClockFace {
		return 0, 0, false
	}
	m, err = minutesOf(minute)
	if err != nil {
		return 0, 0, false
	}
	h %= 12
	if strings.EqualFold(half, "pm") {
		h += 12
	}
	return h, m, true
}

func minutesOf(minute string) (int, error) {
	if minute == "" {
		return 0, nil
	}
	m, err := strconv.Atoi(minute)
	if err != nil {
		return 0, err
	}
	if m > 59 {
		return 0, fmt.Errorf("minute %d is past 59", m)
	}
	return m, nil
}

func zoneOf(name string, fallback *time.Location) *time.Location {
	if name == "" {
		return fallback
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return fallback
	}
	return loc
}

func passed(reset, local time.Time) bool {
	return reset.Before(local.Truncate(time.Minute))
}

func nextClock(local time.Time, h, m int) time.Time {
	reset := time.Date(local.Year(), local.Month(), local.Day(), h, m, 0, 0, local.Location())
	if passed(reset, local) {
		reset = time.Date(local.Year(), local.Month(), local.Day()+1, h, m, 0, 0, local.Location())
	}
	return reset
}

func dateClock(local time.Time, month, monthDay, year string, h, m int) (time.Time, bool) {
	parsed, err := time.Parse("Jan 2", month+" "+monthDay)
	if err != nil {
		return time.Time{}, false
	}
	y := local.Year()
	if year != "" {
		if y, err = strconv.Atoi(year); err != nil {
			return time.Time{}, false
		}
	}
	reset := time.Date(y, parsed.Month(), parsed.Day(), h, m, 0, 0, local.Location())
	if year == "" && passed(reset, local) {
		reset = reset.AddDate(1, 0, 0)
	}
	return reset, true
}
