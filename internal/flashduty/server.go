package flashduty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// slogAdapter adapts slog.Logger to mcp-go's util.Logger interface
type slogAdapter struct {
	logger *slog.Logger
}

func (a *slogAdapter) Infof(format string, v ...any) {
	a.logger.Info(fmt.Sprintf(format, v...))
}

func (a *slogAdapter) Errorf(format string, v ...any) {
	a.logger.Error(fmt.Sprintf(format, v...))
}

type FlashdutyConfig struct {
	// Version of the server
	Version string

	// Flashduty API Base URL
	BaseURL string

	// Flashduty APP Key to authenticate with the Flashduty API
	APPKey string

	// EnabledToolsets is a list of toolsets to enable
	EnabledToolsets []string

	// ReadOnly indicates if we should only offer read-only tools
	ReadOnly bool

	// Translator provides translated text for the server tooling
	Translator translations.TranslationHelperFunc
}

func NewMCPServer(cfg FlashdutyConfig) (*server.MCPServer, error) {
	// When a client send an initialize request, update the user agent to include the client info.
	beforeInit := func(ctx context.Context, _ any, message *mcp.InitializeRequest) {
		_, client, err := getClient(ctx, cfg, cfg.Version)
		if err != nil {
			// Cannot return error here, just log it.
			// For HTTP server, the APP key is per-request, so it might not be available
			// during the initial 'initialize' call if the server doesn't provide it.
			// We can log a warning and proceed. The client will be created on-demand
			// during actual tool calls.
			slog.Warn("Could not get client during initialization, maybe APP key is not yet available", "error", err)
			return
		}

		userAgent := fmt.Sprintf(
			"flashduty-mcp-server/%s (%s/%s)",
			cfg.Version,
			message.Params.ClientInfo.Name,
			message.Params.ClientInfo.Version,
		)
		client.SetUserAgent(userAgent)
	}

	if len(cfg.EnabledToolsets) == 0 {
		cfg.EnabledToolsets = []string{"all"}
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

	hooks := &server.Hooks{
		OnBeforeInitialize: []server.OnBeforeInitializeFunc{beforeInit},
		OnBeforeAny: []server.BeforeAnyHookFunc{
			func(ctx context.Context, _ any, _ mcp.MCPMethod, _ any) {
				pkgerrors.ContextWithFlashdutyErrors(ctx)
			},
			func(ctx context.Context, id any, method mcp.MCPMethod, message any) {
				attrs := buildLogAttrs(ctx, id, method, "params", mcplog.TruncateBodyDefault(toJSONString(message)))
				slog.Info("mcp request", attrs...)
			},
		},
		OnSuccess: []server.OnSuccessHookFunc{
			func(ctx context.Context, id any, method mcp.MCPMethod, message any, result any) {
				attrs := buildLogAttrs(ctx, id, method, "result", mcplog.TruncateBodyDefault(toJSONString(result)))
				slog.Info("mcp response", attrs...)
			},
		},
		OnError: []server.OnErrorHookFunc{
			func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
				attrs := buildLogAttrs(ctx, id, method, "error", err)
				slog.Error("mcp error", attrs...)
			},
		},
	}

	flashdutyServer := server.NewMCPServer("flashduty-mcp-server", cfg.Version, server.WithHooks(hooks))

	getClientFn := func(ctx context.Context) (context.Context, *flashduty.Client, error) {
		return getClient(ctx, cfg, cfg.Version)
	}

	// Create default toolsets
	tsg := flashduty.DefaultToolsetGroup(getClientFn, cfg.ReadOnly, cfg.Translator)
	err := tsg.EnableToolsets(cfg.EnabledToolsets)
	if err != nil {
		return nil, fmt.Errorf("failed to enable toolsets: %w", err)
	}

	// Register all mcp functionality with the server
	tsg.RegisterAll(flashdutyServer)

	return flashdutyServer, nil
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

	// Port to listen on
	Port string

	// OutputFormat specifies the format for tool results (json or toon)
	OutputFormat string

	// Path to the log file if not stderr
	LogFilePath string
}

// extractAppKey extracts app_key from Authorization header or query parameters
func extractAppKey(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) == 2 && strings.ToLower(tokenParts[0]) == "bearer" {
			return tokenParts[1]
		}
	}
	return r.URL.Query().Get("app_key")
}

// httpContextFunc extracts configuration from the HTTP request and injects it into the context.
func httpContextFunc(ctx context.Context, r *http.Request, defaultBaseURL string) context.Context {
	queryParams := r.URL.Query()

	var enabledToolsets []string
	if toolsets := queryParams.Get("toolsets"); toolsets != "" {
		enabledToolsets = strings.Split(toolsets, ",")
	}

	baseURL := queryParams.Get("base_url")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	cfg := FlashdutyConfig{
		BaseURL:         baseURL,
		APPKey:          extractAppKey(r),
		EnabledToolsets: enabledToolsets,
		ReadOnly:        queryParams.Get("read_only") == "true",
	}

	return ContextWithConfig(ctx, cfg)
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

	// Create a single MCP server instance with a default/empty config.
	// The actual config will be provided per-session via the context.
	mcpServer, err := NewMCPServer(FlashdutyConfig{
		Version:         cfg.Version,
		Translator:      t,
		EnabledToolsets: []string{"all"},
	})
	if err != nil {
		return fmt.Errorf("failed to create MCP server: %w", err)
	}

	httpServer := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithLogger(&slogAdapter{logger: logger}),
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			// Extract W3C Trace Context from HTTP headers, or generate a new one
			traceCtx, err := trace.FromHTTPHeadersOrNew(r.Header)
			if err != nil {
				logger.Warn("Failed to generate trace context, continuing without trace", "error", err)
				// Continue without trace context if generation fails
			} else {
				ctx = trace.ContextWithTraceContext(ctx, traceCtx)
			}

			// Note: HTTP request logging is handled by MCP hooks (OnBeforeAny, OnSuccess, OnError)
			// which provide more detailed information including method, params, and results.

			return httpContextFunc(ctx, r, cfg.BaseURL)
		}),
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp", httpServer)
	mux.Handle("/flashduty", httpServer) // Keep for backward compatibility

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
