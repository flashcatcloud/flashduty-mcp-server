package flashduty

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// humanizeTimestamps returns a copy of v with Unix-timestamp fields rendered as
// RFC3339 strings in the local timezone, leaving everything else untouched.
//
// Flashduty's API returns time fields as bare Unix integers, which is opaque to
// an LLM reading tool output. RFC3339 is unambiguous, sortable, and the format
// models are most fluent in. The local timezone is the process timezone (the
// sandbox/environment timezone when the server runs inside an agent sandbox).
//
// Detection is by JSON field name: a field ending in "_time" or "_at", or named
// exactly "timestamp", whose value is an integer large enough to be a real
// timestamp (>= 1e9 seconds, i.e. year 2001+). Millisecond values (>= 1e12) are
// detected by magnitude. ID-like fields (*_by, *_id, *_ids) are never touched.
//
// v is round-tripped through JSON into a generic structure so the same walk
// handles both typed SDK structs and the map[string]any payloads tools build by
// hand. On any marshal/decode error it returns v unchanged — humanization is
// best-effort and never blocks output.
func humanizeTimestamps(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return v
	}
	return humanizeWalk(generic, "")
}

func humanizeWalk(v any, key string) any {
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			val[k] = humanizeWalk(child, k)
		}
		return val
	case []any:
		for i, child := range val {
			val[i] = humanizeWalk(child, key)
		}
		return val
	case json.Number:
		if isTimestampField(key) {
			if s, ok := renderTimestamp(val); ok {
				return s
			}
		}
		return val
	default:
		return val
	}
}

// isTimestampField reports whether a JSON field name denotes an absolute time.
// ID-like suffixes are excluded first so e.g. "timeline_id" / "updated_by"
// never match.
func isTimestampField(key string) bool {
	k := strings.ToLower(key)
	if strings.HasSuffix(k, "_id") || strings.HasSuffix(k, "_ids") || strings.HasSuffix(k, "_by") {
		return false
	}
	return k == "timestamp" || strings.HasSuffix(k, "_time") || strings.HasSuffix(k, "_at")
}

// renderTimestamp converts a numeric Unix timestamp to RFC3339 in local time.
// Values below 1e9 are treated as durations/counts, not absolute timestamps,
// and left unconverted; values at/above 1e12 are interpreted as milliseconds.
func renderTimestamp(n json.Number) (string, bool) {
	i, err := n.Int64()
	if err != nil {
		return "", false
	}
	var t time.Time
	switch {
	case i >= 1e12:
		t = time.UnixMilli(i)
	case i >= 1e9:
		t = time.Unix(i, 0)
	default:
		return "", false
	}
	return t.In(time.Local).Format(time.RFC3339), true
}
