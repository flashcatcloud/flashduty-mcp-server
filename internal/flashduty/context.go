package flashduty

import (
	"context"
	"fmt"
	"sync"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/flashduty"
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
func clientFromContext(ctx context.Context) (*flashduty.Client, bool) {
	client, ok := ctx.Value(flashdutyClientKey).(*flashduty.Client)
	return client, ok
}

// contextWithClient adds the Flashduty client to the context.
func contextWithClient(ctx context.Context, client *flashduty.Client) context.Context {
	return context.WithValue(ctx, flashdutyClientKey, client)
}

var clientCache = &sync.Map{} // map[string]*flashduty.Client

// getClientFromContext is a helper function for tool handlers to get a flashduty client.
// It will try to get the client from the context first. If not found, it will create a new one
// based on the config in the context, and cache it in the context for future use in the same request.
// It falls back to the default config if no config is found in the context.
func getClient(ctx context.Context, defaultCfg FlashdutyConfig, version string) (context.Context, *flashduty.Client, error) {
	if client, ok := clientFromContext(ctx); ok {
		return ctx, client, nil
	}

	cfg, ok := ConfigFromContext(ctx)
	if !ok {
		cfg = defaultCfg
	}

	if cfg.APPKey == "" {
		return ctx, nil, fmt.Errorf("Flashduty APP key is not configured")
	}

	// Use APP key as cache key, assuming one key corresponds to one client configuration.
	if client, ok := clientCache.Load(cfg.APPKey); ok {
		return contextWithClient(ctx, client.(*flashduty.Client)), client.(*flashduty.Client), nil
	}

	userAgent := fmt.Sprintf("flashduty-mcp-server/%s", version)

	client, err := flashduty.NewClient(cfg.APPKey, cfg.BaseURL, userAgent)
	if err != nil {
		return ctx, nil, fmt.Errorf("failed to create Flashduty client: %w", err)
	}

	clientCache.Store(cfg.APPKey, client)
	ctx = contextWithClient(ctx, client)

	return ctx, client, nil
}
