package flashduty

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"testing"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

func TestNewStreamableHTTPServer_DoesNotDisableStreaming(t *testing.T) {
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

	value := reflect.ValueOf(httpServer).Elem().FieldByName("disableStreaming")
	if !value.IsValid() {
		t.Fatal("expected streamable HTTP server to expose disableStreaming field")
	}
	if value.Bool() {
		t.Fatal("expected streaming to remain enabled for GET/SSE clients")
	}
}
