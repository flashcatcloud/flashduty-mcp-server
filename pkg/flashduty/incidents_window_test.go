package flashduty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	flashduty "github.com/flashcatcloud/go-flashduty"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

// newQueryIncidentsHarness spins up a fake /incident/list backend that records
// the request body, and returns the wired query_incidents handler.
func newQueryIncidentsHarness(t *testing.T, gotBody *map[string]any) (server *httptest.Server, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"items": []any{}, "total": 0},
		})
	}))

	client, err := flashduty.NewClient("test-key", flashduty.WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("new go-flashduty client: %v", err)
	}
	_, h := QueryIncidents(func(ctx context.Context) (context.Context, *Clients, error) {
		return ctx, &Clients{New: client}, nil
	}, translations.NullTranslationHelper)
	return ts, h
}

func bodyInt(t *testing.T, body map[string]any, key string) int64 {
	t.Helper()
	v, ok := body[key]
	if !ok {
		t.Fatalf("request body missing %q; got %#v", key, body)
	}
	f, ok := v.(float64) // JSON numbers decode to float64
	if !ok {
		t.Fatalf("%q = %#v, want a number", key, v)
	}
	return int64(f)
}

// TestQueryIncidentsDefaultsWindowWhenOmitted is the regression for the reported
// failure: "find current severe incidents" with severity+progress but NO
// since/until used to be rejected with "both since and until are required".
// It must now succeed and default to the last 30 days.
func TestQueryIncidentsDefaultsWindowWhenOmitted(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	ts, handler := newQueryIncidentsHarness(t, &gotBody)
	defer ts.Close()

	before := time.Now().Unix()
	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "query_incidents",
			Arguments: map[string]any{
				// Mirrors the real report: severity + progress, no time window.
				"severity": "Critical",
				"progress": "Triggered,Processing",
			},
		},
	})
	after := time.Now().Unix()
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		txt, _ := mcp.AsTextContent(result.Content[0])
		t.Fatalf("expected success with defaulted window, got error: %s", txt.Text)
	}

	start := bodyInt(t, gotBody, "start_time")
	end := bodyInt(t, gotBody, "end_time")

	// end should be ~now.
	if end < before || end > after {
		t.Errorf("end_time = %d, want within [%d,%d] (now)", end, before, after)
	}
	// span should be ~30 days (allow a couple seconds of clock drift).
	wantSpan := int64(DefaultIncidentWindow / time.Second)
	if span := end - start; span < wantSpan-5 || span > wantSpan+5 {
		t.Errorf("window span = %ds, want ~%ds (30 days)", span, wantSpan)
	}
	// the defaulted window must be inside the backend's 31-day cap.
	if span := end - start; span > int64(MaxTimeWindow/time.Second) {
		t.Errorf("defaulted span %ds exceeds MaxTimeWindow", span)
	}
}

// TestQueryIncidentsOnlyUntilErrors: a one-sided window — only `until` given,
// `since` omitted — is a real mistake, so it must error without hitting the
// backend rather than silently inventing a `since`. (Both-omitted defaults to
// 30d; a bare `since` is fine because `until` defaults to now — see the
// neighbouring tests.)
func TestQueryIncidentsOnlyUntilErrors(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	ts, handler := newQueryIncidentsHarness(t, &gotBody)
	defer ts.Close()

	// only `until` provided, `since` omitted → helpful error, no backend call.
	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "query_incidents",
			Arguments: map[string]any{
				"until": "now",
			},
		},
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected an error when only `until` is given, got success")
	}
	if gotBody != nil {
		t.Errorf("backend should not be called on the one-sided-window error; got body %#v", gotBody)
	}
}

// TestQueryIncidentsOnlySinceUsesNowUntil confirms the existing contract is
// preserved: a bare `since` is fine because `until` documents a "now" default.
func TestQueryIncidentsBareSinceDefaultsUntilToNow(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	ts, handler := newQueryIncidentsHarness(t, &gotBody)
	defer ts.Close()

	before := time.Now().Unix()
	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "query_incidents",
			Arguments: map[string]any{
				"since": "7d",
			},
		},
	})
	after := time.Now().Unix()
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		txt, _ := mcp.AsTextContent(result.Content[0])
		t.Fatalf("bare `since` should succeed, got error: %s", txt.Text)
	}

	end := bodyInt(t, gotBody, "end_time")
	if end < before || end > after {
		t.Errorf("end_time = %d, want ~now within [%d,%d]", end, before, after)
	}
	start := bodyInt(t, gotBody, "start_time")
	wantStart := before - 7*24*3600
	if start < wantStart-5 || start > wantStart+5 {
		t.Errorf("start_time = %d, want ~now-7d (%d)", start, wantStart)
	}
}
