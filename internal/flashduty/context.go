package flashduty

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bluele/gcache"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/trace"
	sdk "github.com/flashcatcloud/flashduty-sdk"
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

// clientFromContext returns the Flashduty client from the context.
func clientFromContext(ctx context.Context) (*sdk.Client, bool) {
	client, ok := ctx.Value(flashdutyClientKey).(*sdk.Client)
	return client, ok
}

// contextWithClient adds the Flashduty client to the context.
func contextWithClient(ctx context.Context, client *sdk.Client) context.Context {
	return context.WithValue(ctx, flashdutyClientKey, client)
}

var clientCache = gcache.New(1000).
	Expiration(time.Hour).
	Build()

// getClient is a helper function for tool handlers to get a flashduty client.
// It will try to get the client from the context first. If not found, it will create a new one
// based on the config in the context, and cache it in the context for future use in the same request.
// It falls back to the default config if no config is found in the context.
func getClient(ctx context.Context, defaultCfg FlashdutyConfig, version string) (context.Context, *sdk.Client, error) {
	if client, ok := clientFromContext(ctx); ok {
		return ctx, client, nil
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
	if client, err := clientCache.Get(cacheKey); err == nil {
		return contextWithClient(ctx, client.(*sdk.Client)), client.(*sdk.Client), nil
	}

	userAgent := fmt.Sprintf("flashduty-mcp-server/%s", version)

	opts := []sdk.Option{
		sdk.WithUserAgent(userAgent),
		sdk.WithRequestHook(func(req *http.Request) {
			if traceCtx := trace.FromContext(req.Context()); traceCtx != nil {
				traceCtx.SetHTTPHeaders(req.Header)
			}
		}),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, sdk.WithBaseURL(cfg.BaseURL))
	}

	client, err := sdk.NewClient(cfg.APPKey, opts...)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to create Flashduty client: %w", err)
	}

	_ = clientCache.Set(cacheKey, client)
	ctx = contextWithClient(ctx, client)

	return ctx, client, nil
}
