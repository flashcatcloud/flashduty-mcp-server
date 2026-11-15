package flashduty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goflashduty "github.com/flashcatcloud/go-flashduty"
	mcpClient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newTestHTTPMCPClient(t *testing.T, endpointURL string, headers map[string]string) *mcpClient.Client {
	t.Helper()

	var opts []transport.StreamableHTTPCOption
	if len(headers) > 0 {
		opts = append(opts, transport.WithHTTPHeaders(headers))
	}

	cli, err := mcpClient.NewStreamableHttpClient(endpointURL, opts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = cli.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = "2025-03-26"
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "security-test-client",
		Version: "1.0.0",
	}
	_, err = cli.Initialize(ctx, initReq)
	require.NoError(t, err)

	return cli
}

func listToolNames(t *testing.T, cli *mcpClient.Client) map[string]bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)

	names := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	return names
}

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

// TestNewStreamableHTTPServer_Stateless asserts that the server hands out no
// session: initialize returns no Mcp-Session-Id, and a later request without
// one is served. A session the server issues is kept in memory until the
// client sends DELETE, so issuing none is what keeps memory flat.
func TestNewStreamableHTTPServer_Stateless(t *testing.T) {
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

	post := func(body string) (http.Header, []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", body, err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST %s: status %d", body, resp.StatusCode)
		}
		return resp.Header, data
	}

	header, _ := post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if id := header.Get("Mcp-Session-Id"); id != "" {
		t.Fatalf("initialize returned session id %q; want none", id)
	}

	_, data := post(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var got struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	if len(got.Result.Tools) == 0 {
		t.Fatal("tools/list without a session returned no tools")
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
			// A TLS-terminating load balancer that sets no X-Forwarded-Proto.
			name: "no forwarded proto",
			path: "/mcp",
			want: `Bearer resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource/mcp"`,
		},
		{
			name:    "loopback",
			path:    "/mcp",
			headers: map[string]string{"X-Forwarded-Host": "127.0.0.1:11310"},
			want:    `Bearer resource_metadata="http://127.0.0.1:11310/.well-known/oauth-protected-resource/mcp"`,
		},
		{
			name:    "unknown forwarded proto",
			path:    "/mcp",
			headers: map[string]string{"X-Forwarded-Proto": "gopher"},
			want:    `Bearer resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource/mcp"`,
		},
		{
			name:    "forwarded http",
			path:    "/mcp",
			headers: map[string]string{"X-Forwarded-Proto": "http", "X-Forwarded-Host": "mcp.internal"},
			want:    `Bearer resource_metadata="http://mcp.internal/.well-known/oauth-protected-resource/mcp"`,
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
			want: `Bearer resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource/flashduty"`,
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
			wantResource: "https://mcp.example.com/mcp",
		},
		{
			name:         "root describes /mcp",
			path:         "/.well-known/oauth-protected-resource",
			wantResource: "https://mcp.example.com/mcp",
		},
		{
			name:         "legacy path",
			path:         "/.well-known/oauth-protected-resource/flashduty",
			wantResource: "https://mcp.example.com/flashduty",
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

	t.Run("app key ignores base_url override", func(t *testing.T) {
		t.Parallel()
		api, seen := newFakeAPI(t)
		other, otherSeen := newFakeAPI(t)
		callAPIAs(t, "http://mcp.example.com/mcp?base_url="+other.URL, "Bearer base-url-app-key", api.URL)
		if len(*seen) != 1 || len(*otherSeen) != 0 {
			t.Fatalf("default API saw %d requests, override saw %d; want 1 and 0", len(*seen), len(*otherSeen))
		}
	})
}

func TestHTTPServer_IgnoresClientBaseURLOverride_PreventsSSRF(t *testing.T) {
	t.Parallel()

	var legitHits atomic.Int32
	legitUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		legitHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"items": []any{
					map[string]any{
						"field_id":     "f1",
						"field_name":   "service",
						"display_name": "Service",
						"field_type":   "single_select",
						"value_type":   "string",
					},
				},
			},
		})
	}))
	defer legitUpstream.Close()

	var attackerHits atomic.Int32
	attackerTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attackerHits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("SECRET_CLOUD_METADATA_TOKEN_12345"))
	}))
	defer attackerTarget.Close()

	var logBuf syncBuffer
	logger := slog.New(newOrderedTextHandler(&logBuf, slog.LevelDebug))

	handler, err := newHTTPHandler(HTTPServerConfig{
		Version: "test",
		BaseURL: legitUpstream.URL,
	}, translations.NullTranslationHelper, logger)
	require.NoError(t, err)

	mcpHTTP := httptest.NewServer(handler)
	defer mcpHTTP.Close()

	sampleAppKey := "test_app_param_999"
	maliciousBaseURL := attackerTarget.URL + "/latest/meta-data/?app_key=" + sampleAppKey
	endpoint := mcpHTTP.URL + "/mcp?base_url=" + url.QueryEscape(maliciousBaseURL) + "&app_key=" + url.QueryEscape(sampleAppKey)

	cli := newTestHTTPMCPClient(t, endpoint, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := cli.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "query_fields",
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	textContent, ok := mcp.AsTextContent(res.Content[0])
	require.True(t, ok)
	assert.NotContains(t, textContent.Text, "SECRET_CLOUD_METADATA_TOKEN_12345")
	assert.Contains(t, textContent.Text, `"field_name":"service"`)

	assert.Equal(t, int32(0), attackerHits.Load(), "SSRF target must never be dialed")
	assert.Equal(t, int32(1), legitHits.Load(), "configured server BaseURL must handle the request")
	assert.NotContains(t, logBuf.String(), sampleAppKey, "raw app_key must never appear in logs")
}

func TestHTTPServer_AppKeyRedactedInLogsAndUpstreamErrors(t *testing.T) {
	t.Parallel()

	// Unit-check RedactAppKey idempotency and URL-encoded query handling.
	rawURL := "https://mcp.flashcat.cloud/mcp?app_key=super_secret_123&toolsets=fields"
	once := RedactAppKey(rawURL)
	assert.Equal(t, "https://mcp.flashcat.cloud/mcp?app_key=[REDACTED]&toolsets=fields", once)
	assert.Equal(t, once, RedactAppKey(once), "RedactAppKey must be idempotent")
	assert.Equal(t, "url=https%3A%2F%2Fapi.flashcat.cloud%3Fapp_key%3d[REDACTED]", RedactAppKey("url=https%3A%2F%2Fapi.flashcat.cloud%3Fapp_key%3dencoded_secret_456"))

	var mode atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch mode.Load() {
		case 1:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html><body><h1>502 Bad Gateway</h1><p>Internal upstream pod ip-10-42-0-99</p></body></html>"))
		case 2:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":"InvalidParameter","message":"bad request for https://api.flashcat.cloud/field/list?app_key=leaked_inline_key_777"}}`))
		}
	}))
	defer upstream.Close()

	var orderedBuf syncBuffer
	orderedLogger := slog.New(newOrderedTextHandler(&orderedBuf, slog.LevelDebug))
	orderedLogger.Error("request failed", "err", errors.New("Get https://api.flashcat.cloud/field/list?app_key=direct_err_secret_321: EOF"))

	var stdTextBuf syncBuffer
	stdTextLogger := slog.New(slog.NewTextHandler(&stdTextBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handler, err := newHTTPHandler(HTTPServerConfig{
		Version: "test",
		BaseURL: upstream.URL,
	}, translations.NullTranslationHelper, stdTextLogger)
	require.NoError(t, err)

	mcpHTTP := httptest.NewServer(handler)
	defer mcpHTTP.Close()

	cli := newTestHTTPMCPClient(t, mcpHTTP.URL+"/mcp?app_key=caller_app_key_888", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Case 1: HTML 502 error body is never echoed verbatim
	mode.Store(1)
	res1, err := cli.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "query_fields"}})
	require.NoError(t, err)
	require.True(t, res1.IsError)
	txt1, _ := mcp.AsTextContent(res1.Content[0])
	assert.NotContains(t, txt1.Text, "<html>")
	assert.NotContains(t, txt1.Text, "ip-10-42-0-99")

	// Case 2: Structured JSON error has inline app_key redacted in both tool response and logs
	mode.Store(2)
	res2, err := cli.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "query_fields"}})
	require.NoError(t, err)
	require.True(t, res2.IsError)
	txt2, _ := mcp.AsTextContent(res2.Content[0])
	assert.Contains(t, txt2.Text, "InvalidParameter")
	assert.NotContains(t, txt2.Text, "leaked_inline_key_777")
	assert.Contains(t, txt2.Text, "app_key=[REDACTED]")

	assert.NotContains(t, orderedBuf.String(), "direct_err_secret_321")
	assert.Contains(t, orderedBuf.String(), "app_key=[REDACTED]")

	logs := stdTextBuf.String()
	assert.NotContains(t, logs, "caller_app_key_888")
	assert.NotContains(t, logs, "leaked_inline_key_777")
	assert.NotContains(t, logs, "ip-10-42-0-99")
	assert.Contains(t, logs, "[REDACTED]")
}

func TestHTTPServer_ReadOnlyQueryParam_FiltersListAndBlocksWriteCalls(t *testing.T) {
	t.Parallel()

	var writeHits atomic.Int32
	var readHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/incident/create", "/incident/ack", "/incident/resolve", "/incident/reset", "/status-page/change/create", "/status-page/change/timeline":
			writeHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"incident_id": "inc-created"},
			})
		case "/field/list":
			readHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"items": []any{}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	handler, err := newHTTPHandler(HTTPServerConfig{
		Version: "test",
		BaseURL: upstream.URL,
	}, translations.NullTranslationHelper, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	mcpHTTP := httptest.NewServer(handler)
	defer mcpHTTP.Close()

	roClient := newTestHTTPMCPClient(t, mcpHTTP.URL+"/mcp?read_only=true", map[string]string{
		"Authorization": "Bearer test-key",
	})
	rwClient := newTestHTTPMCPClient(t, mcpHTTP.URL+"/mcp", map[string]string{
		"Authorization": "Bearer test-key",
	})

	// 1. tools/list hides all write tools for read_only=true while keeping them for rwClient
	roTools := listToolNames(t, roClient)
	for _, writeTool := range []string{
		"create_incident",
		"update_incident",
		"ack_incident",
		"close_incident",
		"create_status_incident",
		"create_change_timeline",
	} {
		assert.False(t, roTools[writeTool], "write tool %s must be hidden when read_only=true", writeTool)
	}
	assert.True(t, roTools["query_incidents"])
	assert.True(t, roTools["query_fields"])
	assert.True(t, roTools["query_status_pages"])

	rwTools := listToolNames(t, rwClient)
	assert.True(t, rwTools["create_incident"], "default client without read_only=true should see write tools")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 2. Direct tools/call on write tools with read_only=true is blocked before reaching upstream
	_, err = roClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "create_incident",
			Arguments: map[string]any{
				"title":    "Attempted write in read_only mode",
				"severity": "Info",
			},
		},
	})
	require.Error(t, err)
	assert.Equal(t, int32(0), writeHits.Load(), "blocked write tool must never invoke upstream API")

	// 3. Read tools succeed under read_only=true
	readRes, err := roClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "query_fields"},
	})
	require.NoError(t, err)
	require.False(t, readRes.IsError)
	assert.Equal(t, int32(1), readHits.Load())

	// 4. Write tools still work for normal read-write client on the same server
	writeRes, err := rwClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "create_incident",
			Arguments: map[string]any{
				"title":    "Allowed write",
				"severity": "Info",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, writeRes.IsError)
	assert.Equal(t, int32(1), writeHits.Load())
}

func TestHTTPServer_ServerLevelReadOnly_CannotBeBypassed(t *testing.T) {
	t.Parallel()

	var writeHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/field/list" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{}}})
			return
		}
		writeHits.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"incident_id": "inc-1"}})
	}))
	defer upstream.Close()

	handler, err := newHTTPHandler(HTTPServerConfig{
		Version:  "test",
		BaseURL:  upstream.URL,
		ReadOnly: true,
	}, translations.NullTranslationHelper, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	mcpHTTP := httptest.NewServer(handler)
	defer mcpHTTP.Close()

	for _, query := range []string{"?read_only=false", "?read_only=0", ""} {
		cli := newTestHTTPMCPClient(t, mcpHTTP.URL+"/mcp"+query, map[string]string{
			"Authorization": "Bearer test-key",
		})

		tools := listToolNames(t, cli)
		assert.False(t, tools["create_incident"], "query %q must not expose write tools when server ReadOnly=true", query)
		assert.False(t, tools["ack_incident"], "query %q must not expose write tools when server ReadOnly=true", query)
		assert.True(t, tools["query_fields"])

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, callErr := cli.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "create_incident",
				Arguments: map[string]any{
					"title":    "bypass attempt",
					"severity": "Info",
				},
			},
		})
		cancel()
		require.Error(t, callErr, "query %q must not bypass server-level ReadOnly=true on tools/call", query)
	}

	assert.Equal(t, int32(0), writeHits.Load(), "upstream write endpoint must never be reached")
}

func TestHTTPServer_Toolsets_EnforcedOnListAndCall_AndCannotExpandBeyondServer(t *testing.T) {
	t.Parallel()

	var channelHits atomic.Int32
	var fieldHits atomic.Int32
	var memberHits atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/channel/list":
			channelHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{}}})
		case "/field/list":
			fieldHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{}}})
		case "/member/list":
			memberHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{}, "total": 0}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	// Part 1: Server enables all toolsets; client restricts via ?toolsets=users,fields
	allHandler, err := newHTTPHandler(HTTPServerConfig{
		Version: "test",
		BaseURL: upstream.URL,
	}, translations.NullTranslationHelper, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	allSrv := httptest.NewServer(allHandler)
	defer allSrv.Close()

	subsetCli := newTestHTTPMCPClient(t, allSrv.URL+"/mcp?toolsets=users,fields", map[string]string{
		"Authorization": "Bearer test-key",
	})
	subsetTools := listToolNames(t, subsetCli)
	assert.Equal(t, map[string]bool{
		"query_members": true,
		"query_teams":   true,
		"query_fields":  true,
	}, subsetTools)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Calling a tool outside ?toolsets=users,fields is rejected before hitting upstream
	_, err = subsetCli.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "query_channels"},
	})
	require.Error(t, err)
	assert.Equal(t, int32(0), channelHits.Load())

	// Calling an enabled tool succeeds
	fieldRes, err := subsetCli.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "query_fields"},
	})
	require.NoError(t, err)
	require.False(t, fieldRes.IsError)
	assert.Equal(t, int32(1), fieldHits.Load())

	// Part 2: Server restricts EnabledToolsets to ["incidents", "fields"]; client cannot expand to "channels" or "users"
	restrictedHandler, err := newHTTPHandler(HTTPServerConfig{
		Version:         "test",
		BaseURL:         upstream.URL,
		EnabledToolsets: []string{"incidents", "fields"},
	}, translations.NullTranslationHelper, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	restrictedSrv := httptest.NewServer(restrictedHandler)
	defer restrictedSrv.Close()

	// ?toolsets=fields,channels,users -> intersection is ONLY "fields"
	intersectCli := newTestHTTPMCPClient(t, restrictedSrv.URL+"/mcp?toolsets=fields,channels,users", map[string]string{
		"Authorization": "Bearer test-key",
	})
	intersectTools := listToolNames(t, intersectCli)
	assert.Equal(t, map[string]bool{
		"query_fields": true,
	}, intersectTools)

	_, err = intersectCli.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "query_channels"},
	})
	require.Error(t, err)
	assert.Equal(t, int32(0), channelHits.Load())

	_, err = intersectCli.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "query_members"},
	})
	require.Error(t, err)
	assert.Equal(t, int32(0), memberHits.Load())

	_, err = intersectCli.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "query_incidents",
			Arguments: map[string]any{
				"incident_ids": "inc-1",
			},
		},
	})
	require.Error(t, err, "incidents was not in client's ?toolsets=fields,channels,users so it must be blocked")

	// ?toolsets=all on restricted server still only exposes ["incidents", "fields"]
	expandAllCli := newTestHTTPMCPClient(t, restrictedSrv.URL+"/mcp?toolsets=all", map[string]string{
		"Authorization": "Bearer test-key",
	})
	expandAllTools := listToolNames(t, expandAllCli)
	assert.True(t, expandAllTools["query_incidents"])
	assert.True(t, expandAllTools["query_fields"])
	assert.False(t, expandAllTools["query_channels"])
	assert.False(t, expandAllTools["query_members"])

	// ?toolsets=channels (completely disjoint from server's ["incidents", "fields"]) -> 0 tools listed and all calls blocked
	disjointCli := newTestHTTPMCPClient(t, restrictedSrv.URL+"/mcp?toolsets=channels", map[string]string{
		"Authorization": "Bearer test-key",
	})
	disjointTools := listToolNames(t, disjointCli)
	assert.Empty(t, disjointTools)

	_, err = disjointCli.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "query_fields"},
	})
	require.Error(t, err)
}
