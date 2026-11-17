package flashduty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bluele/gcache"
	goflashduty "github.com/flashcatcloud/go-flashduty"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/flashduty"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/trace"
)

type contextKey string

const (
	configKey               = contextKey("flashdutyConfig")
	flashdutyClientKey      = contextKey("flashdutyClient")
	defaultFlashdutyBaseURL = "https://api.flashcat.cloud"
	maxUpstreamErrorBytes   = 64 * 1024
)

// ContextWithConfig adds the Flashduty config to the context.
func ContextWithConfig(ctx context.Context, cfg FlashdutyConfig) context.Context {
	return context.WithValue(ctx, configKey, cfg)
}

// ConfigFromContext returns the Flashduty config from the context.
func ConfigFromContext(ctx context.Context) (FlashdutyConfig, bool) {
	cfg, ok := ctx.Value(configKey).(FlashdutyConfig)
	return cfg, ok
}

// clientsFromContext returns the Flashduty clients from the context.
func clientsFromContext(ctx context.Context) (*flashduty.Clients, bool) {
	clients, ok := ctx.Value(flashdutyClientKey).(*flashduty.Clients)
	return clients, ok
}

// contextWithClients adds the Flashduty clients to the context.
func contextWithClients(ctx context.Context, clients *flashduty.Clients) context.Context {
	return context.WithValue(ctx, flashdutyClientKey, clients)
}

var clientCache = gcache.New(1000).
	Expiration(time.Hour).
	Build()

type redactingRoundTripper struct {
	base http.RoundTripper
}

func (rt *redactingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, errors.New(RedactAppKey(err.Error()))
	}
	if resp == nil || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return resp, nil
	}

	// For non-2xx JSON error bodies, redact any echoed app_key query values before
	// go-flashduty surfaces error.message into MCP tool responses.
	origBody := resp.Body
	limited, readErr := io.ReadAll(io.LimitReader(origBody, maxUpstreamErrorBytes+1))
	_, _ = io.Copy(io.Discard, io.LimitReader(origBody, maxUpstreamErrorBytes))
	_ = origBody.Close()
	if readErr != nil {
		resp.Body = io.NopCloser(strings.NewReader(""))
		resp.ContentLength = 0
		return resp, nil
	}
	if len(limited) <= maxUpstreamErrorBytes && bytes.Contains(bytes.ToLower(limited), []byte("app_key")) && json.Valid(limited) {
		redacted := RedactAppKey(string(limited))
		resp.Body = io.NopCloser(strings.NewReader(redacted))
		resp.ContentLength = int64(len(redacted))
		return resp, nil
	}
	resp.Body = io.NopCloser(bytes.NewReader(limited))
	resp.ContentLength = int64(len(limited))
	return resp, nil
}

func newUpstreamHTTPClient() *http.Client {
	transport := http.DefaultTransport
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = defaultTransport.Clone()
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &redactingRoundTripper{base: transport},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			first := via[0].URL
			if !strings.EqualFold(req.URL.Scheme, first.Scheme) || !strings.EqualFold(req.URL.Host, first.Host) {
				return errors.New("cross-origin redirects are not allowed")
			}
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			return nil
		},
	}
}

// redactingFlashdutyLogger adapts log/slog to goflashduty.Logger while redacting app_key values.
type redactingFlashdutyLogger struct {
	logger *slog.Logger
}

func (l redactingFlashdutyLogger) slogger() *slog.Logger {
	if l.logger != nil {
		return newRedactingLogger(l.logger)
	}
	return newRedactingLogger(slog.Default())
}

func sanitizeLogKV(kv []any) []any {
	if len(kv) == 0 {
		return nil
	}
	sanitized := make([]any, len(kv))
	for i, v := range kv {
		switch typed := v.(type) {
		case string:
			sanitized[i] = RedactAppKey(typed)
		case error:
			sanitized[i] = errors.New(RedactAppKey(typed.Error()))
		case fmt.Stringer:
			sanitized[i] = RedactAppKey(typed.String())
		default:
			sanitized[i] = v
		}
	}
	return sanitized
}

func (l redactingFlashdutyLogger) Debug(msg string, keysAndValues ...any) {
	l.slogger().Debug(RedactAppKey(msg), sanitizeLogKV(keysAndValues)...)
}

func (l redactingFlashdutyLogger) Info(msg string, keysAndValues ...any) {
	l.slogger().Info(RedactAppKey(msg), sanitizeLogKV(keysAndValues)...)
}

func (l redactingFlashdutyLogger) Warn(msg string, keysAndValues ...any) {
	l.slogger().Warn(RedactAppKey(msg), sanitizeLogKV(keysAndValues)...)
}

func (l redactingFlashdutyLogger) Error(msg string, keysAndValues ...any) {
	l.slogger().Error(RedactAppKey(msg), sanitizeLogKV(keysAndValues)...)
}

// getClient is a helper for tool handlers to obtain the Flashduty clients. It
// tries the context first; on a miss it builds the typed go-flashduty client,
// caches it, and stores it on the context for reuse within the same request. It
// falls back to the default config when the context carries none.
func getClient(ctx context.Context, defaultCfg FlashdutyConfig, version string) (context.Context, *flashduty.Clients, error) {
	if clients, ok := clientsFromContext(ctx); ok {
		return ctx, clients, nil
	}

	cfg, ok := ConfigFromContext(ctx)
	if !ok {
		cfg = defaultCfg
	} else {
		// Server-configured BaseURL always takes precedence over per-request context.
		if strings.TrimSpace(defaultCfg.BaseURL) != "" {
			cfg.BaseURL = strings.TrimSpace(defaultCfg.BaseURL)
		}
		if cfg.APPKey == "" && cfg.AccessToken == "" {
			cfg.APPKey = defaultCfg.APPKey
			cfg.AccessToken = defaultCfg.AccessToken
		}
	}

	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultFlashdutyBaseURL
	}

	if cfg.APPKey == "" && cfg.AccessToken == "" {
		return ctx, nil, fmt.Errorf("flashduty app key is not configured")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = defaultCfg.Logger
	}

	// Key by credential kind, credential, BaseURL, and logger so app keys and
	// OAuth access tokens never share an entry, and environments stay separate.
	cacheKey := fmt.Sprintf("app_key|%s|%s|%p", cfg.APPKey, cfg.BaseURL, logger)
	if cfg.AccessToken != "" {
		cacheKey = fmt.Sprintf("access_token|%s|%s|%p", cfg.AccessToken, cfg.BaseURL, logger)
	}
	if cached, err := clientCache.Get(cacheKey); err == nil {
		clients := cached.(*flashduty.Clients)
		return contextWithClients(ctx, clients), clients, nil
	}

	userAgent := fmt.Sprintf("flashduty-mcp-server/%s", version)

	requestHook := func(req *http.Request) {
		if traceCtx := trace.FromContext(req.Context()); traceCtx != nil {
			traceCtx.SetHTTPHeaders(req.Header)
		}
		if session := server.ClientSessionFromContext(req.Context()); session != nil {
			if swci, ok := session.(server.SessionWithClientInfo); ok {
				ci := swci.GetClientInfo()
				if ci.Name != "" {
					req.Header.Set("User-Agent", fmt.Sprintf("flashduty-mcp-server/%s (%s/%s)", version, ci.Name, ci.Version))
				}
			}
		}
	}

	newOpts := []goflashduty.Option{
		goflashduty.WithUserAgent(userAgent),
		goflashduty.WithHTTPClient(newUpstreamHTTPClient()),
		goflashduty.WithLogger(redactingFlashdutyLogger{logger: logger}),
		goflashduty.WithBaseURL(cfg.BaseURL),
		goflashduty.WithRequestHook(requestHook),
	}
	var newClient *goflashduty.Client
	var err error
	if cfg.AccessToken != "" {
		newClient, err = goflashduty.NewClientWithAccessToken(cfg.AccessToken, newOpts...)
	} else {
		newClient, err = goflashduty.NewClient(cfg.APPKey, newOpts...)
	}
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to create go-flashduty client: %s", RedactAppKey(err.Error()))
	}

	clients := &flashduty.Clients{New: newClient}

	_ = clientCache.Set(cacheKey, clients)
	ctx = contextWithClients(ctx, clients)

	return ctx, clients, nil
}
