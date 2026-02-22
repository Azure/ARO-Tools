package timeparse

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// ParseDuration parses a duration string with support for days and weeks.
// First tries standard library time.ParseDuration (supports h, m, s, ms, us, ns).
// Falls back to custom parsing for "d" (days) and "w" (weeks).
//
// Examples:
//   - "2h"    -> 2 hours (stdlib)
//   - "30m"   -> 30 minutes (stdlib)
//   - "1d"    -> 24 hours
//   - "2w"    -> 336 hours
func ParseDuration(s string) (time.Duration, error) {
	// Try standard library first (supports h, m, s, ms, us, ns)
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Handle d, w
	re := regexp.MustCompile(`^(\d+)([dw])$`)
	matches := re.FindStringSubmatch(s)

	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid duration: %s (expected format: 2h, 30m, 1d, 2w, etc.)", s)
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid number in duration: %s", matches[1])
	}

	switch matches[2] {
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	case "w":
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid duration unit: %s", matches[2])
	}
}

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

type Parser struct {
	clock Clock
}

func NewParser(clock Clock) *Parser {
	if clock == nil {
		clock = systemClock{}
	}
	return &Parser{clock: clock}
}

// ToUTC parses a time string into a timestamp.
// Supports:
//   - RFC 3339 format: "2025-11-02T15:30:00Z" or "2025-11-02T15:30:00-05:00"
//   - Duration: "1d", "2w", "12h" (relative to now, going back in time)
//
// All times are returned in UTC.
func (p *Parser) ToUTC(timeStr string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t.UTC(), nil
	}

	duration, err := ParseDuration(timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time: %s (expected RFC3339 or duration like 1d, 2w, 12h)", timeStr)
	}

	return p.clock.Now().Add(-duration), nil
}

// FormatRelativeTime formats a duration into a human-readable relative time string.
func FormatRelativeTime(d time.Duration) string {
	var out string
	var num int
	switch {
	case d < time.Minute:
		out = "less than a minute"
	case d < time.Hour:
		num = int(d.Minutes())
		out = fmt.Sprintf("%d minute", num)
	case d < 24*time.Hour:
		num = int(d.Hours())
		out = fmt.Sprintf("%d hour", num)
	default:
		num = int(d.Hours() / 24)
		out = fmt.Sprintf("%d day", num)
	}

	if num > 1 {
		out = out + "s"
	}

	return out
}
