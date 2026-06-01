package flashduty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	flashduty "github.com/flashcatcloud/go-flashduty"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

// TestQueryIncidentsNumsReachesWire verifies the new `nums` argument is split
// and sent to /incident/list as the nums array (short-id filtering), within the
// since/until window.
func TestQueryIncidentsNumsReachesWire(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"items": []any{}, "total": 0},
		})
	}))
	defer ts.Close()

	client, err := flashduty.NewClient("test-key", flashduty.WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("new go-flashduty client: %v", err)
	}

	_, handler := QueryIncidents(func(ctx context.Context) (context.Context, *Clients, error) {
		return ctx, &Clients{New: client}, nil
	}, translations.NullTranslationHelper)

	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "query_incidents",
			Arguments: map[string]any{
				"nums":  "311510,ABC123",
				"since": "7d",
				"until": "now",
			},
		},
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		txt, _ := mcp.AsTextContent(result.Content[0])
		t.Fatalf("expected success result, got error: %s", txt.Text)
	}

	if gotPath != "/incident/list" {
		t.Fatalf("path = %q, want /incident/list", gotPath)
	}
	nums, _ := gotBody["nums"].([]any)
	if len(nums) != 2 || nums[0] != "311510" || nums[1] != "ABC123" {
		t.Errorf("nums = %#v, want [311510 ABC123]", gotBody["nums"])
	}
}
