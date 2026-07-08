package flashduty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	flashduty "github.com/flashcatcloud/go-flashduty"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

// captureWire spins up a test backend that records the request path and decoded
// JSON body, then returns an empty list response. It lets a tool handler run
// end-to-end so we can assert what actually reached the wire.
func captureWire(t *testing.T) (*httptest.Server, *string, *map[string]any) {
	t.Helper()
	var gotPath string
	gotBody := map[string]any{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"items": []any{}, "total": 0},
		})
	}))
	return ts, &gotPath, &gotBody
}

func newTestClients(t *testing.T, baseURL string) GetFlashdutyClientFn {
	t.Helper()
	client, err := flashduty.NewClient("test-key", flashduty.WithBaseURL(baseURL))
	if err != nil {
		t.Fatalf("new go-flashduty client: %v", err)
	}
	return func(ctx context.Context) (context.Context, *Clients, error) {
		return ctx, &Clients{New: client}, nil
	}
}

func callOK(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), name string, args map[string]any) {
	t.Helper()
	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: name, Arguments: args},
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		txt, _ := mcp.AsTextContent(result.Content[0])
		t.Fatalf("expected success result, got error: %s", txt.Text)
	}
}

// TestQueryIncidentsPageReachesWire verifies `page` is sent to /incident/list as
// the 1-based `p` field, and that the default (page 1) omits it so non-paginating
// callers keep an unchanged payload.
func TestQueryIncidentsPageReachesWire(t *testing.T) {
	t.Parallel()

	t.Run("page 2 sends p", func(t *testing.T) {
		ts, gotPath, gotBody := captureWire(t)
		defer ts.Close()
		_, handler := QueryIncidents(newTestClients(t, ts.URL), translations.NullTranslationHelper)

		callOK(t, handler, "query_incidents", map[string]any{
			"since": "7d", "until": "now", "page": float64(2),
		})

		if *gotPath != "/incident/list" {
			t.Fatalf("path = %q, want /incident/list", *gotPath)
		}
		if p, ok := (*gotBody)["p"]; !ok || p != float64(2) {
			t.Errorf("p = %#v, want 2", (*gotBody)["p"])
		}
	})

	t.Run("default omits p", func(t *testing.T) {
		ts, _, gotBody := captureWire(t)
		defer ts.Close()
		_, handler := QueryIncidents(newTestClients(t, ts.URL), translations.NullTranslationHelper)

		callOK(t, handler, "query_incidents", map[string]any{"since": "7d", "until": "now"})

		if _, ok := (*gotBody)["p"]; ok {
			t.Errorf("p should be absent for default page 1, got %#v", (*gotBody)["p"])
		}
	})
}

// TestQueryChannelsLimitPageReachesWire verifies the newly added limit/page reach
// /channel/list. channel_ids is a filter on the same paginated endpoint (not a
// dedicated by-ID lookup), so a channel_ids lookup pages like any other listing.
func TestQueryChannelsLimitPageReachesWire(t *testing.T) {
	t.Parallel()

	t.Run("search path sends limit and p", func(t *testing.T) {
		ts, gotPath, gotBody := captureWire(t)
		defer ts.Close()
		_, handler := QueryChannels(newTestClients(t, ts.URL), translations.NullTranslationHelper)

		callOK(t, handler, "query_channels", map[string]any{
			"name": "prod", "limit": float64(50), "page": float64(3),
		})

		if *gotPath != "/channel/list" {
			t.Fatalf("path = %q, want /channel/list", *gotPath)
		}
		if v, ok := (*gotBody)["limit"]; !ok || v != float64(50) {
			t.Errorf("limit = %#v, want 50", (*gotBody)["limit"])
		}
		if v, ok := (*gotBody)["p"]; !ok || v != float64(3) {
			t.Errorf("p = %#v, want 3", (*gotBody)["p"])
		}
	})

	t.Run("channel_ids lookup also pages", func(t *testing.T) {
		ts, gotPath, gotBody := captureWire(t)
		defer ts.Close()
		_, handler := QueryChannels(newTestClients(t, ts.URL), translations.NullTranslationHelper)

		callOK(t, handler, "query_channels", map[string]any{
			"channel_ids": "1,2,3", "limit": float64(50), "page": float64(3),
		})

		if *gotPath != "/channel/list" {
			t.Fatalf("path = %q, want /channel/list", *gotPath)
		}
		if v, ok := (*gotBody)["limit"]; !ok || v != float64(50) {
			t.Errorf("limit = %#v, want 50 (channel_ids lookup must page the same endpoint)", (*gotBody)["limit"])
		}
		if v, ok := (*gotBody)["p"]; !ok || v != float64(3) {
			t.Errorf("p = %#v, want 3", (*gotBody)["p"])
		}
	})
}

// TestQueryMembersPagingReachesWire verifies the search path pages via
// /member/list, while a person_ids lookup uses the dedicated /member/infos
// endpoint and carries no paging fields.
func TestQueryMembersPagingReachesWire(t *testing.T) {
	t.Parallel()

	t.Run("name search sends limit and p", func(t *testing.T) {
		ts, gotPath, gotBody := captureWire(t)
		defer ts.Close()
		_, handler := QueryMembers(newTestClients(t, ts.URL), translations.NullTranslationHelper)

		callOK(t, handler, "query_members", map[string]any{
			"name": "alice", "limit": float64(40), "page": float64(2),
		})

		if *gotPath != "/member/list" {
			t.Fatalf("path = %q, want /member/list", *gotPath)
		}
		if v, ok := (*gotBody)["limit"]; !ok || v != float64(40) {
			t.Errorf("limit = %#v, want 40", (*gotBody)["limit"])
		}
		if v, ok := (*gotBody)["p"]; !ok || v != float64(2) {
			t.Errorf("p = %#v, want 2", (*gotBody)["p"])
		}
	})

	t.Run("person_ids lookup is unpaged", func(t *testing.T) {
		ts, gotPath, gotBody := captureWire(t)
		defer ts.Close()
		_, handler := QueryMembers(newTestClients(t, ts.URL), translations.NullTranslationHelper)

		callOK(t, handler, "query_members", map[string]any{
			"person_ids": "1,2", "limit": float64(40), "page": float64(2),
		})

		// Direct ID lookup uses the dedicated /person/infos endpoint, which
		// returns the requested profiles without paging.
		if !strings.Contains(*gotPath, "/infos") {
			t.Fatalf("path = %q, want the /infos by-ID endpoint", *gotPath)
		}
		if _, ok := (*gotBody)["p"]; ok {
			t.Errorf("p should be absent for person_ids lookup, got %#v", (*gotBody)["p"])
		}
		if _, ok := (*gotBody)["limit"]; ok {
			t.Errorf("limit should be absent for person_ids lookup, got %#v", (*gotBody)["limit"])
		}
	})
}

// decodeResult unmarshals a tool result's JSON text payload into a map.
func decodeResult(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	txt, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(txt.Text), &m); err != nil {
		t.Fatalf("decode result JSON: %v (raw: %s)", err, txt.Text)
	}
	return m
}

// TestQueryChangesPaging verifies the normal path pages via /change/list, and
// that a change_ids lookup (a client-side filter over one page — /change/list
// has no server-side id filter) does NOT emit a misleading page hint keyed off
// the window's unfiltered total.
func TestQueryChangesPaging(t *testing.T) {
	t.Parallel()

	t.Run("normal path sends limit and p", func(t *testing.T) {
		ts, gotPath, gotBody := captureWire(t)
		defer ts.Close()
		_, handler := QueryChanges(newTestClients(t, ts.URL), translations.NullTranslationHelper)

		callOK(t, handler, "query_changes", map[string]any{
			"since": "7d", "until": "now", "limit": float64(30), "page": float64(2),
		})

		if *gotPath != "/change/list" {
			t.Fatalf("path = %q, want /change/list", *gotPath)
		}
		if v, ok := (*gotBody)["limit"]; !ok || v != float64(30) {
			t.Errorf("limit = %#v, want 30", (*gotBody)["limit"])
		}
		if v, ok := (*gotBody)["p"]; !ok || v != float64(2) {
			t.Errorf("p = %#v, want 2", (*gotBody)["p"])
		}
	})

	t.Run("change_ids lookup reports found count, no page hint", func(t *testing.T) {
		// Backend returns 2 changes on the page but claims a huge window total.
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []any{
						map[string]any{"change_id": "c1"},
						map[string]any{"change_id": "c2"},
					},
					"total": 5000,
				},
			})
		}))
		defer ts.Close()
		_, handler := QueryChanges(newTestClients(t, ts.URL), translations.NullTranslationHelper)

		result, err := handler(context.Background(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: "query_changes", Arguments: map[string]any{
				"since": "7d", "until": "now", "change_ids": "c1",
			}},
		})
		if err != nil || result.IsError {
			t.Fatalf("handler failed: err=%v isErr=%v", err, result.IsError)
		}
		payload := decodeResult(t, result)
		if _, ok := payload["truncated"]; ok {
			t.Errorf("change_ids lookup must not set truncated, got %#v", payload["truncated"])
		}
		if payload["total"] != float64(1) {
			t.Errorf("total = %#v, want 1 (found count, not the window's 5000)", payload["total"])
		}
		if changes, _ := payload["changes"].([]any); len(changes) != 1 {
			t.Errorf("changes = %#v, want exactly the one matched id", payload["changes"])
		}
	})
}

// TestAddPageHint locks the pagination-aware truncation logic: the hint must
// fire only when the caller has NOT paged through everything. The critical case
// is the final partial page — comparing total>count alone would wrongly flag it
// and send the agent chasing an empty next page.
func TestAddPageHint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		count, total      int
		page, limit       int
		wantTruncated     bool
		wantNextPageToken string // substring the hint must contain when truncated
	}{
		{"first page of many", 20, 25, 1, 20, true, "page:2"},
		{"last partial page", 5, 25, 2, 20, false, ""},
		{"single full page", 5, 5, 1, 20, false, ""},
		{"middle page", 20, 100, 3, 20, true, "page:4"},
		{"exact last full page", 20, 40, 2, 20, false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := addPageHint(map[string]any{}, tc.count, tc.total, tc.page, tc.limit)
			_, truncated := res["truncated"]
			if truncated != tc.wantTruncated {
				t.Fatalf("truncated = %v, want %v (count=%d total=%d page=%d limit=%d)",
					truncated, tc.wantTruncated, tc.count, tc.total, tc.page, tc.limit)
			}
			if tc.wantTruncated {
				hint, _ := res["hint"].(string)
				if !strings.Contains(hint, tc.wantNextPageToken) {
					t.Errorf("hint = %q, want it to mention %q", hint, tc.wantNextPageToken)
				}
			}
		})
	}
}
