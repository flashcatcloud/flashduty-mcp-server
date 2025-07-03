package flashduty

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ChannelInfos creates a tool to get collaboration space information by channel IDs
func ChannelInfos(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_channels_infos",
			mcp.WithDescription(t("TOOL_FLASHDUTY_CHANNELS_INFOS_DESCRIPTION", "Get collaboration space information by channel IDs")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_CHANNELS_INFOS_USER_TITLE", "Get channel infos"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("channel_ids",
				mcp.Required(),
				mcp.Description("Comma-separated list of channel IDs to get information for. Example: '123,456,789'"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract channel_ids from request
			channelIdsStr, err := RequiredParam[string](request, "channel_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse comma-separated string to int slice
			var channelIdsInt []int
			if channelIdsStr != "" {
				parts := strings.Split(channelIdsStr, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" {
						id, err := strconv.Atoi(part)
						if err != nil {
							return mcp.NewToolResultError(fmt.Sprintf("Invalid channel_id: %s", part)), nil
						}
						channelIdsInt = append(channelIdsInt, id)
					}
				}
			}

			if len(channelIdsInt) == 0 {
				return mcp.NewToolResultError("channel_ids cannot be empty"), nil
			}

			// Get Flashduty client
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			// Build request body according to API specification
			requestBody := map[string]interface{}{
				"channel_ids": channelIdsInt,
			}

			// Make API request to /channel/infos endpoint
			resp, err := client.makeRequest(ctx, "POST", "/channel/infos", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to get channel infos: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("API request failed with status %d", resp.StatusCode)), nil
			}

			// Parse response
			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			// Check for API error
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(result.Data), nil
		}
}

func ListChannels(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_list_channels",
			mcp.WithDescription(t("TOOL_FLASHDUTY_LIST_CHANNELS_DESCRIPTION", "List all collaboration channels")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_LIST_CHANNELS_USER_TITLE", "List channels"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get flashduty client: %w", err)
			}
			resp, err := client.makeRequest(ctx, "GET", "/channel/list", nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get channel list: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("API request failed with status %d", resp.StatusCode)), nil
			}

			// Parse response
			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			// Check for API error
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(result.Data), nil
		}
}
