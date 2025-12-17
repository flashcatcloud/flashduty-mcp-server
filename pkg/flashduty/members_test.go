package flashduty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createMCPRequest is a helper function to create a MCP request with the given arguments.
func createMCPRequest(args any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: struct {
			Name      string    `json:"name"`
			Arguments any       `json:"arguments,omitempty"`
			Meta      *mcp.Meta `json:"_meta,omitempty"`
		}{
			Arguments: args,
		},
	}
}

// getTextResult is a helper function that returns a text result from a tool call.
func getTextResult(t *testing.T, result *mcp.CallToolResult) mcp.TextContent {
	t.Helper()
	assert.NotNil(t, result)
	require.Len(t, result.Content, 1)
	require.IsType(t, mcp.TextContent{}, result.Content[0])
	textContent := result.Content[0].(mcp.TextContent)
	assert.Equal(t, "text", textContent.Type)
	return textContent
}

// testServer creates a mock Flashduty API server for testing
func testServer(t *testing.T, responses map[string]interface{}) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if a response is configured for the given path
		if response, exists := responses[r.URL.Path]; exists {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			err := json.NewEncoder(w).Encode(response)
			if err != nil {
				t.Fatalf("Failed to write mock response: %v", err)
			}
			return
		}

		// Check if an error is configured
		if errorResponse, exists := responses["error:"+r.URL.Path]; exists {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			err := json.NewEncoder(w).Encode(errorResponse)
			if err != nil {
				t.Fatalf("Failed to write mock error response: %v", err)
			}
			return
		}

		// Default to not found
		http.NotFound(w, r)
	}))
}

// testTranslationHelper is a simple translation helper for testing
func testTranslationHelper(key, defaultValue string) string {
	return defaultValue
}

// testSetup provides common test setup for Flashduty tests
func testSetup(t *testing.T, responses map[string]interface{}) (GetFlashdutyClientFn, translations.TranslationHelperFunc) {
	t.Helper()

	// Create a test server
	server := testServer(t, responses)
	t.Cleanup(server.Close) // Ensure the server is closed after the test

	// Create a client that points to the test server
	getClient := func(ctx context.Context) (context.Context, *Client, error) {
		client, err := NewClient("test-app-key", server.URL, "test-agent")
		if err != nil {
			return nil, nil, err
		}
		return ctx, client, nil
	}

	translator := testTranslationHelper

	return getClient, translator
}

func TestMemberInfos(t *testing.T) {
	responses := map[string]interface{}{
		"/person/infos": map[string]interface{}{
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"person_id": 1, "person_name": "John Doe", "avatar": "https://example.com/avatar1.png", "as": "member"},
					map[string]interface{}{"person_id": 2, "person_name": "Jane Smith", "avatar": "https://example.com/avatar2.png", "as": "account"},
				},
			},
		},
	}
	getClient, translator := testSetup(t, responses)

	tool, handler := MemberInfos(getClient, translator)
	assert.Equal(t, "flashduty_member_infos", tool.Name)

	request := createMCPRequest(map[string]interface{}{
		"person_ids": "1,2",
	})
	ctx := context.Background()
	result, err := handler(ctx, request)

	assert.NoError(t, err)
	textResult := getTextResult(t, result)
	assert.Contains(t, textResult.Text, "John Doe")
	assert.Contains(t, textResult.Text, "Jane Smith")
}
