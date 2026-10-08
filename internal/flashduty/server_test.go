package flashduty

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	goflashduty "github.com/flashcatcloud/go-flashduty"

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

// newTestMux returns the HTTP routes with a stub MCP handler that records
// whether a request got through.
func newTestMux(baseURL string) (http.Handler, *bool) {
	reached := false
	return newHTTPMux(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}), baseURL), &reached
}

func TestHTTPMux_MissingCredentialChallenges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		headers map[string]string
		want    string
	}{
		{
			name: "direct",
			path: "/mcp",
			want: `Bearer resource_metadata="http://mcp.example.com/.well-known/oauth-protected-resource/mcp"`,
		},
		{
			name:    "behind proxy",
			path:    "/mcp",
			headers: map[string]string{"X-Forwarded-Proto": "https, http", "X-Forwarded-Host": "mcp.flashcat.cloud"},
			want:    `Bearer resource_metadata="https://mcp.flashcat.cloud/.well-known/oauth-protected-resource/mcp"`,
		},
		{
			name: "legacy path",
			path: "/flashduty",
			want: `Bearer resource_metadata="http://mcp.example.com/.well-known/oauth-protected-resource/flashduty"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mux, reached := newTestMux("https://api.flashcat.cloud")
			req := httptest.NewRequest(http.MethodPost, "http://mcp.example.com"+tt.path, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != tt.want {
				t.Fatalf("WWW-Authenticate = %q, want %q", got, tt.want)
			}
			if *reached {
				t.Fatal("request without credential reached the MCP handler")
			}
		})
	}
}

func TestHTTPMux_CredentialPassesThrough(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		target        string
		authorization string
	}{
		{name: "app key query", target: "http://mcp.example.com/mcp?app_key=key"},
		{name: "oauth bearer", target: "http://mcp.example.com/mcp", authorization: "Bearer oauth:token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mux, reached := newTestMux("https://api.flashcat.cloud")
			req := httptest.NewRequest(http.MethodPost, tt.target, nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK || !*reached {
				t.Fatalf("status = %d, reached = %v; want 200 and reached", rec.Code, *reached)
			}
		})
	}
}

func TestHTTPMux_ProtectedResourceMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		path         string
		headers      map[string]string
		wantResource string
	}{
		{
			name:         "path inserted",
			path:         "/.well-known/oauth-protected-resource/mcp",
			wantResource: "http://mcp.example.com/mcp",
		},
		{
			name:         "root describes /mcp",
			path:         "/.well-known/oauth-protected-resource",
			wantResource: "http://mcp.example.com/mcp",
		},
		{
			name:         "legacy path",
			path:         "/.well-known/oauth-protected-resource/flashduty",
			wantResource: "http://mcp.example.com/flashduty",
		},
		{
			name:         "behind proxy",
			path:         "/.well-known/oauth-protected-resource/mcp",
			headers:      map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "mcp.flashcat.cloud"},
			wantResource: "https://mcp.flashcat.cloud/mcp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mux, _ := newTestMux("https://api.flashcat.cloud")
			req := httptest.NewRequest(http.MethodGet, "http://mcp.example.com"+tt.path, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			var got struct {
				Resource             string   `json:"resource"`
				AuthorizationServers []string `json:"authorization_servers"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode metadata: %v", err)
			}
			if got.Resource != tt.wantResource {
				t.Fatalf("resource = %q, want %q", got.Resource, tt.wantResource)
			}
			if !slices.Equal(got.AuthorizationServers, []string{"https://api.flashcat.cloud"}) {
				t.Fatalf("authorization_servers = %v", got.AuthorizationServers)
			}
		})
	}
}

// upstreamRequest is what a fake Flashduty API observed.
type upstreamRequest struct {
	authorization string
	appKey        string
}

func newFakeAPI(t *testing.T) (*httptest.Server, *[]upstreamRequest) {
	t.Helper()
	var seen []upstreamRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, upstreamRequest{
			authorization: r.Header.Get("Authorization"),
			appKey:        r.URL.Query().Get("app_key"),
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"items":[]}}`)
	}))
	t.Cleanup(ts.Close)
	return ts, &seen
}

// callAPIAs builds the request context the way the HTTP transport does and
// issues one API call through the resulting client.
func callAPIAs(t *testing.T, target, authorization, defaultBaseURL string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	ctx := httpContextFunc(context.Background(), req, defaultBaseURL)
	ctx, clients, err := getClient(ctx, FlashdutyConfig{}, "test")
	if err != nil {
		t.Fatalf("getClient: %v", err)
	}
	if _, _, err := clients.New.Teams.ReadInfos(ctx, &goflashduty.TeamInfosRequest{TeamIDs: []uint64{1}}); err != nil {
		t.Fatalf("API call: %v", err)
	}
}

func TestHTTPCredentialRouting(t *testing.T) {
	t.Parallel()

	t.Run("oauth token is sent as bearer", func(t *testing.T) {
		t.Parallel()
		api, seen := newFakeAPI(t)
		callAPIAs(t, "http://mcp.example.com/mcp", "Bearer oauth:routing-token", api.URL)
		want := []upstreamRequest{{authorization: "Bearer oauth:routing-token"}}
		if !slices.Equal(*seen, want) {
			t.Fatalf("upstream saw %+v, want %+v", *seen, want)
		}
	})

	t.Run("app key is sent as query parameter", func(t *testing.T) {
		t.Parallel()
		api, seen := newFakeAPI(t)
		callAPIAs(t, "http://mcp.example.com/mcp", "Bearer routing-app-key", api.URL)
		want := []upstreamRequest{{appKey: "routing-app-key"}}
		if !slices.Equal(*seen, want) {
			t.Fatalf("upstream saw %+v, want %+v", *seen, want)
		}
	})

	t.Run("oauth token ignores base_url override", func(t *testing.T) {
		t.Parallel()
		api, seen := newFakeAPI(t)
		other, otherSeen := newFakeAPI(t)
		callAPIAs(t, "http://mcp.example.com/mcp?base_url="+other.URL, "Bearer oauth:base-url-token", api.URL)
		if len(*seen) != 1 || len(*otherSeen) != 0 {
			t.Fatalf("default API saw %d requests, override saw %d; want 1 and 0", len(*seen), len(*otherSeen))
		}
	})

	t.Run("app key follows base_url override", func(t *testing.T) {
		t.Parallel()
		api, seen := newFakeAPI(t)
		other, otherSeen := newFakeAPI(t)
		callAPIAs(t, "http://mcp.example.com/mcp?base_url="+other.URL, "Bearer base-url-app-key", api.URL)
		if len(*seen) != 0 || len(*otherSeen) != 1 {
			t.Fatalf("default API saw %d requests, override saw %d; want 0 and 1", len(*seen), len(*otherSeen))
		}
	})
}
