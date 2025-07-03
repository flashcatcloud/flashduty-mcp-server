package flashduty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

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
