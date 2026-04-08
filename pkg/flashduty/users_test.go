package flashduty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

func TestQueryTeamsByIDsPreservesLegacyItemsShape(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/team/infos" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []any{
					map[string]any{
						"team_id":   101,
						"team_name": "alpha",
					},
				},
			},
		})
	}))
	defer ts.Close()

	client, err := sdk.NewClient("test-key", sdk.WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("new sdk client: %v", err)
	}

	_, handler := QueryTeams(func(ctx context.Context) (context.Context, *sdk.Client, error) {
		return ctx, client, nil
	}, translations.NullTranslationHelper)

	result, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "query_teams",
			Arguments: map[string]any{
				"team_ids": "101",
			},
		},
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got error result: %#v", result)
	}

	textContent, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatalf("expected text content, got %#v", result.Content[0])
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(textContent.Text), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if _, ok := payload["items"]; !ok {
		t.Fatalf("expected legacy items key, got %#v", payload)
	}
	if _, ok := payload["teams"]; ok {
		t.Fatalf("did not expect teams key for team_ids lookup, got %#v", payload)
	}
}
