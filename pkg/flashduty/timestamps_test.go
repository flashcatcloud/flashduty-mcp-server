package flashduty

import (
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestMarshalResult_HumanizesTimestamps locks the wiring: every tool result
// routed through MarshalResultWithFormat must have its timestamps humanized, so
// a raw Unix integer never reaches the model.
func TestMarshalResult_HumanizesTimestamps(t *testing.T) {
	const ts = 1748419200
	res := MarshalResultWithFormat(map[string]any{"start_time": ts}, OutputFormatJSON)
	tc, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		t.Fatalf("expected text content, got %#v", res.Content[0])
	}
	if strings.Contains(tc.Text, "1748419200") {
		t.Fatalf("raw unix timestamp leaked into tool result: %s", tc.Text)
	}
	if !strings.Contains(tc.Text, "start_time") {
		t.Fatalf("expected start_time key in result: %s", tc.Text)
	}
}

func tsInstant(t *testing.T, v any) int64 {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("expected RFC3339 string, got %T (%v)", v, v)
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("value %q is not RFC3339: %v", s, err)
	}
	return parsed.Unix()
}

func TestHumanizeTimestamps_ConvertsSecondsAndMillis(t *testing.T) {
	const sec = 1748419200
	m := humanizeTimestamps(map[string]any{
		"start_time": sec,
		"created_at": int64(sec) * 1000,
	}).(map[string]any)
	if inst := tsInstant(t, m["start_time"]); inst != sec {
		t.Fatalf("start_time instant = %d, want %d", inst, sec)
	}
	if inst := tsInstant(t, m["created_at"]); inst != sec {
		t.Fatalf("created_at instant = %d, want %d", inst, sec)
	}
}

func TestHumanizeTimestamps_DetectsByFieldName(t *testing.T) {
	const ts = 1748419200
	in := map[string]any{
		"ack_time": ts, "close_time": ts, "assigned_at": ts,
		"acknowledged_at": ts, "timestamp": ts, "end_time": ts, "trigger_time": ts,
	}
	m := humanizeTimestamps(in).(map[string]any)
	for k := range in {
		if inst := tsInstant(t, m[k]); inst != ts {
			t.Fatalf("%s instant = %d, want %d", k, inst, ts)
		}
	}
}

func TestHumanizeTimestamps_LeavesIDAndDurationFields(t *testing.T) {
	in := map[string]any{
		// Large values that WOULD convert by magnitude — proves the field-name
		// exclusion (not just the magnitude guard) is what keeps IDs numeric.
		"updated_by":  int64(1748419200),
		"timeline_id": int64(1748419200),
		"channel_ids": []any{int64(1748419200)},
		"snooze_time": int64(300), // small => duration, not a 1970 date
		"ack_time":    0,          // zero => not a timestamp
	}
	m := humanizeTimestamps(in).(map[string]any)
	for k := range in {
		if _, isStr := m[k].(string); isStr {
			t.Fatalf("%s must not be converted to a date string", k)
		}
	}
}

func TestHumanizeTimestamps_RecursesNestedAndSlices(t *testing.T) {
	const ts = 1748419200
	in := map[string]any{
		"incidents": []any{
			map[string]any{"start_time": ts, "labels": map[string]any{"close_time": ts}},
		},
	}
	m := humanizeTimestamps(in).(map[string]any)
	inc := m["incidents"].([]any)[0].(map[string]any)
	if inst := tsInstant(t, inc["start_time"]); inst != ts {
		t.Fatalf("nested start_time instant = %d, want %d", inst, ts)
	}
	if inst := tsInstant(t, inc["labels"].(map[string]any)["close_time"]); inst != ts {
		t.Fatalf("deeply nested close_time instant = %d, want %d", inst, ts)
	}
}

func TestHumanizeTimestamps_ConvertsTypedStruct(t *testing.T) {
	type incident struct {
		Title     string `json:"title"`
		StartTime int64  `json:"start_time"`
		UpdatedBy int64  `json:"updated_by"`
	}
	const ts = 1748419200
	m := humanizeTimestamps(incident{Title: "db down", StartTime: ts, UpdatedBy: 7}).(map[string]any)
	if inst := tsInstant(t, m["start_time"]); inst != ts {
		t.Fatalf("struct start_time instant = %d, want %d", inst, ts)
	}
	if _, isStr := m["updated_by"].(string); isStr {
		t.Fatalf("struct updated_by must remain numeric")
	}
	if m["title"] != "db down" {
		t.Fatalf("title = %v, want \"db down\"", m["title"])
	}
}
