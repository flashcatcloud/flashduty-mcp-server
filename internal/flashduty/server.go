package flashduty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	pkgerrors "github.com/flashcatcloud/flashduty-mcp-server/pkg/errors"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/flashduty"
	mcplog "github.com/flashcatcloud/flashduty-mcp-server/pkg/log"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/trace"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

type FlashdutyConfig struct {
	// Version of the server
	Version string

	// Flashduty API Base URL
	BaseURL string

	// Flashduty APP Key to authenticate with the Flashduty API
	APPKey string

	// AccessToken is an OAuth access token issued by the Flashduty
	// authorization server; when set it is used instead of APPKey.
	AccessToken string

	// EnabledToolsets is a list of toolsets to enable
	EnabledToolsets []string

	// ReadOnly indicates if we should only offer read-only tools
	ReadOnly bool

	// Translator provides translated text for the server tooling
	Translator translations.TranslationHelperFunc

	// Logger is an optional structured logger (defaults to slog.Default() when nil)
	Logger *slog.Logger
}

// serverInstructions is the default text returned to clients in the
// initialize response; see cfg.Translator for how it can be overridden.
const serverInstructions = "This server provides a curated, task-oriented toolset for Flashduty; it intentionally does not mirror the full Flashduty Open API. For operations not covered by these tools, use the Flashduty CLI (https://github.com/flashcatcloud/flashduty-cli), which covers the open API 1:1 — agents with shell access can drive it directly."

func toolsetRequested(requested []string, targetToolset string) bool {
	for _, item := range requested {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if trimmed == "all" {
			return true
		}
		if targetToolset != "" && trimmed == targetToolset {
			return true
		}
	}
	return false
}

func NewMCPServer(cfg FlashdutyConfig) (*server.MCPServer, error) {
	cleanedToolsets := make([]string, 0, len(cfg.EnabledToolsets))
	for _, ts := range cfg.EnabledToolsets {
		if trimmed := strings.TrimSpace(ts); trimmed != "" {
			cleanedToolsets = append(cleanedToolsets, trimmed)
		}
	}
	if len(cleanedToolsets) == 0 {
		cleanedToolsets = []string{"all"}
	}
	cfg.EnabledToolsets = cleanedToolsets

	if cfg.Translator == nil {
		cfg.Translator = translations.NullTranslationHelper
	}

	toJSONString := func(v any) string {
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}

	buildLogAttrs := func(ctx context.Context, id any, method mcp.MCPMethod, extraAttrs ...any) []any {
		attrs := []any{}
		if tc := trace.FromContext(ctx); tc != nil {
			attrs = append(attrs, "trace_id", tc.TraceID)
		}
		attrs = append(attrs, "id", id, "method", method)
		return append(attrs, extraAttrs...)
	}

	hookLogger := func() *slog.Logger {
		if cfg.Logger != nil {
			return newRedactingLogger(cfg.Logger)
		}
		return newRedactingLogger(slog.Default())
	}

	hooks := &server.Hooks{
		OnBeforeAny: []server.BeforeAnyHookFunc{
			func(ctx context.Context, _ any, _ mcp.MCPMethod, _ any) {
				pkgerrors.ContextWithFlashdutyErrors(ctx)
			},
			func(ctx context.Context, id any, method mcp.MCPMethod, message any) {
				attrs := buildLogAttrs(ctx, id, method, "params", RedactAppKey(mcplog.TruncateBodyDefault(toJSONString(message))))
				hookLogger().Info("mcp request", attrs...)
			},
		},
		OnSuccess: []server.OnSuccessHookFunc{
			func(ctx context.Context, id any, method mcp.MCPMethod, message any, result any) {
				attrs := buildLogAttrs(ctx, id, method, "result", RedactAppKey(mcplog.TruncateBodyDefault(toJSONString(result))))
				hookLogger().Info("mcp response", attrs...)
			},
		},
		OnError: []server.OnErrorHookFunc{
			func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
				errMsg := ""
				if err != nil {
					errMsg = RedactAppKey(err.Error())
				}
				attrs := buildLogAttrs(ctx, id, method, "error", errMsg)
				hookLogger().Error("mcp error", attrs...)
			},
		},
	}

	getClientFn := func(ctx context.Context) (context.Context, *flashduty.Clients, error) {
		return getClient(ctx, cfg, cfg.Version)
	}

	// Create default toolsets and enable configured server-level toolsets
	tsg := flashduty.DefaultToolsetGroup(getClientFn, cfg.ReadOnly, cfg.Translator)
	err := tsg.EnableToolsets(cfg.EnabledToolsets)
	if err != nil {
		return nil, fmt.Errorf("failed to enable toolsets: %w", err)
	}

	toolMeta := tsg.ToolMetadataMap()
	serverEnabledToolsets := tsg.EnabledToolsetSet()

	checkToolAccess := func(ctx context.Context, toolName string, annotations *mcp.ToolAnnotation) error {
		reqCfg, hasReqCfg := ConfigFromContext(ctx)

		meta, known := toolMeta[toolName]
		isReadOnlyTool := false
		if known {
			isReadOnlyTool = meta.ReadOnly
		} else if annotations != nil && annotations.ReadOnlyHint != nil {
			isReadOnlyTool = *annotations.ReadOnlyHint
		}

		// Server-level ReadOnly can never be downgraded by per-request config.
		effectiveReadOnly := cfg.ReadOnly || (hasReqCfg && reqCfg.ReadOnly)
		if effectiveReadOnly && !isReadOnlyTool {
			return fmt.Errorf("permission denied: tool %q is a write operation and disabled in read-only mode", toolName)
		}

		// Client toolsets can only intersect/subset server-enabled toolsets, never expand them.
		if known {
			if !serverEnabledToolsets[meta.Toolset] {
				return fmt.Errorf("tool %q is disabled: toolset %q is not enabled on this server", toolName, meta.Toolset)
			}
			if hasReqCfg && len(reqCfg.EnabledToolsets) > 0 && !toolsetRequested(reqCfg.EnabledToolsets, meta.Toolset) {
				return fmt.Errorf("tool %q is disabled: toolset %q is not enabled for this request", toolName, meta.Toolset)
			}
		} else if hasReqCfg && len(reqCfg.EnabledToolsets) > 0 && !toolsetRequested(reqCfg.EnabledToolsets, "") {
			return fmt.Errorf("tool %q is disabled: not in enabled toolsets", toolName)
		}

		return nil
	}

	toolFilter := func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
		filtered := make([]mcp.Tool, 0, len(tools))
		for _, tool := range tools {
			ann := tool.Annotations
			if checkToolAccess(ctx, tool.Name, &ann) == nil {
				filtered = append(filtered, tool)
			}
		}
		return filtered
	}

	toolPolicyMiddleware := func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := checkToolAccess(ctx, request.Params.Name, nil); err != nil {
				return nil, err
			}
			return next(ctx, request)
		}
	}

	flashdutyServer := server.NewMCPServer(
		"flashduty-mcp-server",
		cfg.Version,
		server.WithHooks(hooks),
		server.WithInstructions(cfg.Translator("SERVER_INSTRUCTIONS", serverInstructions)),
		server.WithToolFilter(toolFilter),
		server.WithToolHandlerMiddleware(toolPolicyMiddleware),
	)

	// Register all mcp functionality with the server
	tsg.RegisterAll(flashdutyServer)

	return flashdutyServer, nil
}

func newStreamableHTTPServer(mcpServer *server.MCPServer, logger *slog.Logger, contextFunc server.HTTPContextFunc) *server.StreamableHTTPServer {
	return server.NewStreamableHTTPServer(
		mcpServer,
		// mcp-go v0.55 renamed the transport logger option to
		// WithStreamableHTTPLogger and switched it to *slog.Logger, so the
		// former slogAdapter shim is no longer needed.
		server.WithStreamableHTTPLogger(newRedactingLogger(logger)),
		// Return 405 for GET requests — this server doesn't use server-initiated
		// features (sampling, elicitation). Without this, the SDK's standalone SSE
		// GET hangs indefinitely because mcp-go creates an orphan session and blocks.
		server.WithDisableStreaming(true),
		// Keep no per-session state. Credentials arrive with every request and
		// nothing server-initiated is sent, so a session carries nothing. In
		// stateful mode every initialize registers a session that is freed only
		// by an explicit DELETE, which most clients never send, so memory grows
		// with the number of handshakes until the process is killed.
		server.WithStateLess(true),
		server.WithHTTPContextFunc(contextFunc),
	)
}

type StdioServerConfig struct {
	// Version of the server
	Version string

	// Flashduty API Base URL
	BaseURL string

	// Flashduty APP Key to authenticate with the Flashduty API
	APPKey string

	// EnabledToolsets is a list of toolsets to enable
	EnabledToolsets []string

	// ReadOnly indicates if we should only register read-only tools
	ReadOnly bool

	// OutputFormat specifies the format for tool results (json or toon)
	OutputFormat string

	// ExportTranslations indicates if we should export translations
	ExportTranslations bool

	// EnableCommandLogging indicates if we should log commands
	EnableCommandLogging bool

	// Path to the log file if not stderr
	LogFilePath string
}

// RunStdioServer is not concurrent safe.
func RunStdioServer(cfg StdioServerConfig) error {
	// Create app context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Set the global output format
	flashduty.SetOutputFormat(flashduty.ParseOutputFormat(cfg.OutputFormat))

	t, dumpTranslations := translations.TranslationHelper()

	flashdutyServer, err := NewMCPServer(FlashdutyConfig{
		Version:         cfg.Version,
		BaseURL:         cfg.BaseURL,
		APPKey:          cfg.APPKey,
		EnabledToolsets: cfg.EnabledToolsets,
		ReadOnly:        cfg.ReadOnly,
		Translator:      t,
	})
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	if cfg.ExportTranslations {
		dumpTranslations()
		return nil
	}

	stdioServer := server.NewStdioServer(flashdutyServer)

	// Setup slog logger
	var slogHandler slog.Handler
	if cfg.LogFilePath != "" {
		file, err := os.OpenFile(cfg.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		slogHandler = newOrderedTextHandler(file, slog.LevelDebug)
	} else {
		slogHandler = newOrderedTextHandler(os.Stderr, slog.LevelInfo)
	}
	logger := slog.New(slogHandler)

	// Start listening for messages
	errC := make(chan error, 1)
	go func() {
		in, out := io.Reader(os.Stdin), io.Writer(os.Stdout)

		if cfg.EnableCommandLogging {
			loggedIO := mcplog.NewIOLogger(in, out, logger)
			in, out = loggedIO, loggedIO
		}
		// enable Flashduty errors in the context
		ctx := pkgerrors.ContextWithFlashdutyErrors(ctx)
		errC <- stdioServer.Listen(ctx, in, out)
	}()

	// Output flashduty-mcp-server string
	_, _ = fmt.Fprintf(os.Stderr, "Flashduty MCP Server running on stdio\n")

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		logger.Info("shutting down server...")
	case err := <-errC:
		if err != nil {
			return fmt.Errorf("error running server: %w", err)
		}
	}

	return nil
}

type HTTPServerConfig struct {
	// Version of the server
	Version string
	// Commit of the server
	Commit string
	// Date of the server
	Date string

	// Flashduty API Base URL
	BaseURL string

	// EnabledToolsets restricts which toolsets the HTTP server exposes (defaults to all)
	EnabledToolsets []string

	// ReadOnly restricts the entire HTTP server to read-only operations
	ReadOnly bool

	// Port to listen on
	Port string

	// OutputFormat specifies the format for tool results (json or toon)
	OutputFormat string

	// Path to the log file if not stderr
	LogFilePath string
}

// oauthTokenPrefix marks access tokens issued by the Flashduty authorization
// server; any other bearer credential is an APP key.
const oauthTokenPrefix = "oauth:"

// extractCredential extracts the bearer credential from the Authorization
// header, falling back to the app_key query parameter.
func extractCredential(r *http.Request) string {
	if authHeader := strings.TrimSpace(r.Header.Get("Authorization")); authHeader != "" {
		tokenParts := strings.SplitN(authHeader, " ", 2)
		if len(tokenParts) == 2 && strings.EqualFold(tokenParts[0], "bearer") {
			return strings.TrimSpace(tokenParts[1])
		}
	}
	return strings.TrimSpace(r.URL.Query().Get("app_key"))
}

func parseReadOnlyQueryParam(v string) bool {
	trimmed := strings.TrimSpace(v)
	return strings.EqualFold(trimmed, "true") || trimmed == "1"
}

func parseQueryToolsets(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	parsed := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			parsed = append(parsed, trimmed)
		}
	}
	if len(parsed) == 0 {
		// Non-empty query parameter with no valid names (e.g. ",,,") must match nothing, not everything.
		return []string{""}
	}
	return parsed
}

func buildHTTPRequestConfig(r *http.Request, defaultBaseURL string, serverReadOnly bool, logger *slog.Logger) FlashdutyConfig {
	resolvedBaseURL := strings.TrimSpace(defaultBaseURL)
	if resolvedBaseURL == "" {
		resolvedBaseURL = defaultFlashdutyBaseURL
	}

	queryParams := r.URL.Query()
	enabledToolsets := parseQueryToolsets(queryParams.Get("toolsets"))

	// Server-level ReadOnly can never be downgraded by ?read_only=false.
	readOnly := serverReadOnly || parseReadOnlyQueryParam(queryParams.Get("read_only"))

	// Never honor client ?base_url= query overrides to prevent SSRF and credential exfiltration.
	if rawBaseURL := strings.TrimSpace(queryParams.Get("base_url")); rawBaseURL != "" {
		if logger != nil && !strings.EqualFold(strings.TrimRight(rawBaseURL, "/"), strings.TrimRight(resolvedBaseURL, "/")) {
			newRedactingLogger(logger).Warn("ignoring client base_url query parameter in HTTP mode",
				"requested_base_url", sanitizeURLForLog(rawBaseURL),
			)
		}
	}

	cfg := FlashdutyConfig{
		BaseURL:         resolvedBaseURL,
		EnabledToolsets: enabledToolsets,
		ReadOnly:        readOnly,
		Logger:          logger,
	}

	credential := extractCredential(r)
	if strings.HasPrefix(credential, oauthTokenPrefix) {
		cfg.AccessToken = credential
	} else {
		cfg.APPKey = credential
	}

	return cfg
}

// httpContextFunc extracts configuration from the HTTP request and injects it into the context.
func httpContextFunc(ctx context.Context, r *http.Request, defaultBaseURL string) context.Context {
	return ContextWithConfig(ctx, buildHTTPRequestConfig(r, defaultBaseURL, false, slog.Default()))
}

// requestOrigin returns the scheme and host the client used to reach this
// server, honoring X-Forwarded-Proto/X-Forwarded-Host set by a reverse proxy.
func requestOrigin(r *http.Request) string {
	host := r.Host
	if fwdHost := firstHeaderValue(r, "X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	return requestScheme(r, host) + "://" + host
}

// requestScheme trusts X-Forwarded-Proto when it names http or https. Without
// it (a TLS-terminating load balancer that does not set the header), only a
// loopback host is plain http: MCP clients require https for any other
// authorization target, so the public resource is https.
func requestScheme(r *http.Request, host string) string {
	switch proto := strings.ToLower(firstHeaderValue(r, "X-Forwarded-Proto")); proto {
	case "http", "https":
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	if hostname == "localhost" {
		return "http"
	}
	if ip := net.ParseIP(hostname); ip != nil && ip.IsLoopback() {
		return "http"
	}
	return "https"
}

// firstHeaderValue returns the first entry of a possibly comma-separated
// header value, as appended by chained proxies.
func firstHeaderValue(r *http.Request, name string) string {
	v, _, _ := strings.Cut(r.Header.Get(name), ",")
	return strings.TrimSpace(v)
}

// requireCredential answers requests that carry no credential with 401 and a
// WWW-Authenticate challenge pointing at the protected resource metadata
// (RFC 9728 §5.1), which starts the MCP OAuth authorization flow.
func requireCredential(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if extractCredential(r) == "" {
			metadataURL := requestOrigin(r) + server.WellKnownProtectedResourcePath + r.URL.Path
			w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer resource_metadata=%q", metadataURL))
			http.Error(w, "missing credential: authenticate with OAuth or send Authorization: Bearer <app_key>", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// protectedResourceMetadataHandler serves RFC 9728 metadata for the MCP
// endpoint at path. The resource identifier is derived from the request
// origin; the authorization server is the Flashduty API base URL.
func protectedResourceMetadataHandler(path, authorizationServer string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.NewProtectedResourceMetadataHandler(server.ProtectedResourceMetadataConfig{
			Resource:             requestOrigin(r) + path,
			AuthorizationServers: []string{authorizationServer},
		}).ServeHTTP(w, r)
	})
}

// newHTTPMux routes the MCP endpoints behind requireCredential and serves
// their protected resource metadata. The root well-known path describes /mcp.
func newHTTPMux(mcpHandler http.Handler, baseURL string) *http.ServeMux {
	mux := http.NewServeMux()
	for _, path := range []string{"/mcp", "/flashduty"} { // /flashduty is kept for backward compatibility
		mux.Handle(path, requireCredential(mcpHandler))
		mux.Handle(server.WellKnownProtectedResourcePath+path, protectedResourceMetadataHandler(path, baseURL))
	}
	mux.Handle(server.WellKnownProtectedResourcePath, protectedResourceMetadataHandler("/mcp", baseURL))
	return mux
}

func newHTTPHandler(cfg HTTPServerConfig, translator translations.TranslationHelperFunc, logger *slog.Logger) (*http.ServeMux, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if translator == nil {
		translator = translations.NullTranslationHelper
	}

	defaultBaseURL := strings.TrimSpace(cfg.BaseURL)
	if defaultBaseURL == "" {
		defaultBaseURL = defaultFlashdutyBaseURL
	}

	enabledToolsets := cfg.EnabledToolsets
	if len(enabledToolsets) == 0 {
		enabledToolsets = []string{"all"}
	}

	mcpServer, err := NewMCPServer(FlashdutyConfig{
		Version:         cfg.Version,
		BaseURL:         defaultBaseURL,
		EnabledToolsets: enabledToolsets,
		ReadOnly:        cfg.ReadOnly,
		Translator:      translator,
		Logger:          logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP server: %w", err)
	}

	httpServer := newStreamableHTTPServer(mcpServer, logger, func(ctx context.Context, r *http.Request) context.Context {
		// Extract W3C Trace Context from HTTP headers, or generate a new one
		traceCtx, traceErr := trace.FromHTTPHeadersOrNew(r.Header)
		if traceErr != nil {
			newRedactingLogger(logger).Warn("Failed to generate trace context, continuing without trace", "error", traceErr)
		} else {
			ctx = trace.ContextWithTraceContext(ctx, traceCtx)
		}

		// Note: HTTP request logging is handled by MCP hooks (OnBeforeAny, OnSuccess, OnError)
		// which provide more detailed information including method, params, and results.

		reqCfg := buildHTTPRequestConfig(r, defaultBaseURL, cfg.ReadOnly, logger)
		reqCfg.Version = cfg.Version
		return ContextWithConfig(ctx, reqCfg)
	})

	return newHTTPMux(httpServer, defaultBaseURL), nil
}

func RunHTTPServer(cfg HTTPServerConfig) error {
	// Set the global output format
	flashduty.SetOutputFormat(flashduty.ParseOutputFormat(cfg.OutputFormat))

	// Setup slog logger
	var slogHandler slog.Handler
	if cfg.LogFilePath != "" {
		// #nosec G304
		file, err := os.OpenFile(cfg.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		slogHandler = newOrderedTextHandler(file, slog.LevelDebug)
	} else {
		slogHandler = newOrderedTextHandler(os.Stderr, slog.LevelInfo)
	}
	logger := slog.New(slogHandler)
	// Set as default logger for global slog calls
	slog.SetDefault(logger)

	// Create translation helper
	t, _ := translations.TranslationHelper()

	mux, err := newHTTPHandler(cfg, t, logger)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       0,                // No timeout for streaming
		WriteTimeout:      0,                // No timeout for streaming
		IdleTimeout:       60 * time.Second, // Prevent dangling connections
		MaxHeaderBytes:    128 * 1024,       // 128KB
	}

	errC := make(chan error, 1)
	go func() {
		logger.Info("Server listening",
			"addr", "http://0.0.0.0:"+cfg.Port,
			"version", cfg.Version,
			"commit", cfg.Commit,
			"date", cfg.Date,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errC <- err
		}
	}()

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		logger.Info("Shutting down server...")
	case err := <-errC:
		return fmt.Errorf("listen failed: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	logger.Info("Server exited properly")
	return nil
}
