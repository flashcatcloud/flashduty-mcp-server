package flashduty

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bluele/gcache"
	goflashduty "github.com/flashcatcloud/go-flashduty"

	sdk "github.com/flashcatcloud/flashduty-sdk"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/flashduty"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/trace"
)

type contextKey string

const (
	configKey          = contextKey("flashdutyConfig")
	flashdutyClientKey = contextKey("flashdutyClient")
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

// getClient is a helper for tool handlers to obtain the Flashduty clients. It
// tries the context first; on a miss it builds both the typed go-flashduty
// client (used by every migrated tool) and the legacy flashduty-sdk client
// (kept only for the not-yet-covered endpoints), caches the pair, and stores it
// on the context for reuse within the same request. It falls back to the
// default config when the context carries none.
func getClient(ctx context.Context, defaultCfg FlashdutyConfig, version string) (context.Context, *flashduty.Clients, error) {
	if clients, ok := clientsFromContext(ctx); ok {
		return ctx, clients, nil
	}

	cfg, ok := ConfigFromContext(ctx)
	if !ok {
		cfg = defaultCfg
	}

	if cfg.APPKey == "" {
		return ctx, nil, fmt.Errorf("flashduty app key is not configured")
	}

	// Use APP key and BaseURL as cache key to handle different environments.
	cacheKey := fmt.Sprintf("%s|%s", cfg.APPKey, cfg.BaseURL)
	if cached, err := clientCache.Get(cacheKey); err == nil {
		clients := cached.(*flashduty.Clients)
		return contextWithClients(ctx, clients), clients, nil
	}

	userAgent := fmt.Sprintf("flashduty-mcp-server/%s", version)

	requestHook := func(req *http.Request) {
		if traceCtx := trace.FromContext(req.Context()); traceCtx != nil {
			traceCtx.SetHTTPHeaders(req.Header)
		}
	}

	// Primary client: typed go-flashduty.
	newOpts := []goflashduty.Option{
		goflashduty.WithUserAgent(userAgent),
		goflashduty.WithRequestHook(requestHook),
	}
	if cfg.BaseURL != "" {
		newOpts = append(newOpts, goflashduty.WithBaseURL(cfg.BaseURL))
	}
	newClient, err := goflashduty.NewClient(cfg.APPKey, newOpts...)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to create go-flashduty client: %w", err)
	}

	// Legacy client: only the not-yet-covered endpoints use it.
	legacyOpts := []sdk.Option{
		sdk.WithUserAgent(userAgent),
		sdk.WithRequestHook(requestHook),
	}
	if cfg.BaseURL != "" {
		legacyOpts = append(legacyOpts, sdk.WithBaseURL(cfg.BaseURL))
	}
	legacyClient, err := sdk.NewClient(cfg.APPKey, legacyOpts...)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to create legacy flashduty client: %w", err)
	}

	clients := &flashduty.Clients{New: newClient, Legacy: legacyClient}

	_ = clientCache.Set(cacheKey, clients)
	ctx = contextWithClients(ctx, clients)

	return ctx, clients, nil
}
