package flashduty

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	flashduty "github.com/flashcatcloud/go-flashduty"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

// TestQueryIncidentsIDsBatched verifies incident_ids longer than the
// /incident/list-by-ids cap are split into batches of 100 and merged back
// newest first.
func TestQueryIncidentsIDsBatched(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var batchSizes []int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			IncidentIDs []string `json:"incident_ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		batchSizes = append(batchSizes, len(body.IncidentIDs))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/incident/list-by-ids" || len(body.IncidentIDs) > 100 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": "InvalidParameter", "message": "too many incident_ids"},
			})
			return
		}
		items := make([]map[string]any, 0, len(body.IncidentIDs))
		for _, id := range body.IncidentIDs {
			var n int64
			_, _ = fmt.Sscanf(id, "id%d", &n)
			items = append(items, map[string]any{"incident_id": id, "start_time": n})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"items": items, "total": len(items)},
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

	ids := make([]string, 250)
	for i := range ids {
		ids[i] = fmt.Sprintf("id%d", i+1)
	}
	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "query_incidents",
			Arguments: map[string]any{"incident_ids": strings.Join(ids, ",")},
		},
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	txt, _ := mcp.AsTextContent(result.Content[0])
	if result.IsError {
		t.Fatalf("expected success result, got error: %s", txt.Text)
	}

	if fmt.Sprint(batchSizes) != "[100 100 50]" {
		t.Errorf("batch sizes = %v, want [100 100 50]", batchSizes)
	}
	var got struct {
		Incidents []struct {
			IncidentID string `json:"incident_id"`
		} `json:"incidents"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(txt.Text), &got); err != nil {
		t.Fatalf("decode result: %v\n%s", err, txt.Text)
	}
	if got.Total != 250 || len(got.Incidents) != 250 {
		t.Fatalf("total = %d, incidents = %d, want 250/250", got.Total, len(got.Incidents))
	}
	if got.Incidents[0].IncidentID != "id250" || got.Incidents[249].IncidentID != "id1" {
		t.Errorf("order: first %s last %s, want id250 … id1", got.Incidents[0].IncidentID, got.Incidents[249].IncidentID)
	}
}
