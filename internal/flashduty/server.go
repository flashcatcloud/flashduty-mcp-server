package flashduty

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	pkgerrors "github.com/flashcatcloud/flashduty-mcp-server/pkg/errors"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/flashduty"
	mcplog "github.com/flashcatcloud/flashduty-mcp-server/pkg/log"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
)

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
			logrus.Warnf("Could not get client during initialization, maybe APP key is not yet available: %v", err)
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

	hooks := &server.Hooks{
		OnBeforeInitialize: []server.OnBeforeInitializeFunc{beforeInit},
		OnBeforeAny: []server.BeforeAnyHookFunc{
			func(ctx context.Context, _ any, _ mcp.MCPMethod, _ any) {
				// Ensure the context is cleared of any previous errors
				// as context isn't propagated through middleware
				pkgerrors.ContextWithFlashdutyErrors(ctx)
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

	logrusLogger := logrus.New()
	if cfg.LogFilePath != "" {
		file, err := os.OpenFile(cfg.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}

		logrusLogger.SetLevel(logrus.DebugLevel)
		logrusLogger.SetOutput(file)
	}
	stdLogger := log.New(logrusLogger.Writer(), "stdioserver", 0)
	stdioServer.SetErrorLogger(stdLogger)

	if cfg.ExportTranslations {
		// Once server is initialized, all translations are loaded
		dumpTranslations()
	}

	// Start listening for messages
	errC := make(chan error, 1)
	go func() {
		in, out := io.Reader(os.Stdin), io.Writer(os.Stdout)

		if cfg.EnableCommandLogging {
			loggedIO := mcplog.NewIOLogger(in, out, logrusLogger)
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
		logrusLogger.Infof("shutting down server...")
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

	// Path to the log file if not stderr
	LogFilePath string
}

// httpContextFunc extracts configuration from the HTTP request and injects it into the context.
func httpContextFunc(ctx context.Context, r *http.Request, defaultBaseURL string) context.Context {
	// Extract app_key from Authorization header
	authHeader := r.Header.Get("Authorization")
	var appKey string
	if authHeader != "" {
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) == 2 && strings.ToLower(tokenParts[0]) == "bearer" {
			appKey = tokenParts[1]
		}
	}

	// Extract other parameters from query
	queryParams := r.URL.Query()

	// If appKey is still empty, try to get it from query params as a fallback.
	if appKey == "" {
		if keyFromQuery := queryParams.Get("app_key"); keyFromQuery != "" {
			appKey = keyFromQuery
		}
	}

	toolsets := queryParams.Get("toolsets")
	var enabledToolsets []string
	if toolsets != "" {
		enabledToolsets = strings.Split(toolsets, ",")
	}
	readOnly := queryParams.Get("read_only") == "true"
	baseURL := queryParams.Get("base_url")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	// Create session config and put it in context
	cfg := FlashdutyConfig{
		BaseURL:         baseURL,
		APPKey:          appKey,
		EnabledToolsets: enabledToolsets,
		ReadOnly:        readOnly,
	}

	return ContextWithConfig(ctx, cfg)
}

func RunHTTPServer(cfg HTTPServerConfig) error {
	// Setup logging
	logrusLogger := logrus.New()
	if cfg.LogFilePath != "" {
		// #nosec G304
		file, err := os.OpenFile(cfg.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		logrusLogger.SetOutput(file)
		logrusLogger.SetLevel(logrus.DebugLevel)
	} else {
		logrusLogger.SetOutput(os.Stderr)
		logrusLogger.SetLevel(logrus.InfoLevel)
	}

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
		logrusLogger.Fatalf("failed to create MCP server: %v", err)
	}

	httpServer := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithLogger(logrusLogger),
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			logrusLogger.Infof("Handling request for %s. Headers: %v", r.URL, r.Header)
			authHeader := r.Header.Get("Authorization")
			logrusLogger.Infof("Authorization header received: %q", authHeader)
			return httpContextFunc(ctx, r, cfg.BaseURL)
		}),
	)

	mux := http.NewServeMux()
	mux.Handle("/flashduty", httpServer)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // No timeout for streaming
	}

	go func() {
		logrusLogger.Infof("Server listening on http://0.0.0.0:%s, version: %s, commit: %s, date: %s", cfg.Port, cfg.Version, cfg.Commit, cfg.Date)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logrusLogger.Fatalf("listen: %s\n", err)
		}
	}()

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	logrusLogger.Info("Shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logrusLogger.Fatalf("Server shutdown failed: %+v", err)
	}

	logrusLogger.Info("Server exited properly")
	return nil
}
