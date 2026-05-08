package timeutil

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parse converts a time string to a unix timestamp (seconds).
// Supported formats:
//   - Go duration (relative to now): "5m", "1h", "24h", "168h"
//     Interpreted as "now minus duration"
//   - Future duration with "+" prefix: "+24h", "+7d"
//     Interpreted as "now plus duration"
//   - Day shorthand: "7d", "30d" — converted to hours automatically
//   - Date: "2026-04-01" (parsed as local midnight)
//   - Datetime: "2026-04-01 10:00:00" (parsed as local time)
//   - Unix timestamp: "1712000000" (passed through)
func Parse(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "now" {
		return time.Now().Unix(), nil
	}

	future := false
	raw := s
	if strings.HasPrefix(s, "+") {
		future = true
		raw = s[1:]
	}

	durStr := expandDays(raw)
	if d, err := time.ParseDuration(durStr); err == nil {
		if d < 0 {
			return 0, fmt.Errorf("negative duration %q is not supported", s)
		}
		if future {
			return time.Now().Add(d).Unix(), nil
		}
		return time.Now().Add(-d).Unix(), nil
	}

	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t.Unix(), nil
	}

	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t.Unix(), nil
	}

	if ts, err := strconv.ParseInt(s, 10, 64); err == nil && ts > 1000000000 {
		return ts, nil
	}

	return 0, fmt.Errorf("unable to parse time %q: expected duration (24h), date (2006-01-02), datetime (2006-01-02 15:04:05), or unix timestamp", s)
}

// expandDays converts day shorthand (e.g. "7d", "30d") to hours for time.ParseDuration.
// time.ParseDuration does not natively support "d" because day length is calendar-dependent.
func expandDays(s string) string {
	if !strings.HasSuffix(s, "d") {
		return s
	}
	numPart := s[:len(s)-1]
	if days, err := strconv.Atoi(numPart); err == nil {
		return fmt.Sprintf("%dh", days*24)
	}
	return s
}
