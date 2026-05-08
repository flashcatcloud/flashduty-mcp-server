package flashduty

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

// TestNewStreamableHTTPServer_RejectsSSEGet asserts that GET requests are
// rejected with 405. Without WithDisableStreaming, mcp-go's standalone SSE
// handler creates an orphan session and hangs indefinitely.
func TestNewStreamableHTTPServer_RejectsSSEGet(t *testing.T) {
	t.Parallel()

	mcpServer, err := NewMCPServer(FlashdutyConfig{
		Version:         "test",
		Translator:      translations.NullTranslationHelper,
		EnabledToolsets: []string{"incidents"},
	})
	if err != nil {
		t.Fatalf("failed to create MCP server: %v", err)
	}

	httpServer := newStreamableHTTPServer(
		mcpServer,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(ctx context.Context, _ *http.Request) context.Context {
			return ctx
		},
	)

	ts := httptest.NewServer(httpServer)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 2 * time.Second} // must not hang
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET request failed (likely SSE hang): %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed for SSE GET, got %d", resp.StatusCode)
	}
}
