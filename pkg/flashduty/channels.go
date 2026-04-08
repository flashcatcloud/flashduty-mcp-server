package flashduty

import (
	"context"
	"fmt"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const queryChannelsDescription = `Query collaboration spaces (channels) by IDs or name. Returns channel info with team details.`

// QueryChannels creates a tool to query channels
func QueryChannels(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_channels",
			mcp.WithDescription(t("TOOL_QUERY_CHANNELS_DESCRIPTION", queryChannelsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_CHANNELS_USER_TITLE", "Query channels"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("channel_ids", mcp.Description("Comma-separated channel IDs for direct lookup. Max 1000 IDs.")),
			mcp.WithString("name", mcp.Description("Search by channel name (case-insensitive substring match).")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			channelIdsStr, _ := OptionalParam[string](request, "channel_ids")
			name, _ := OptionalParam[string](request, "name")

			input := &sdk.ListChannelsInput{
				Name: name,
			}

			// Parse channel IDs if provided
			if channelIdsStr != "" {
				channelIDs := parseCommaSeparatedInts(channelIdsStr)
				if len(channelIDs) == 0 {
					return mcp.NewToolResultError("channel_ids must contain at least one valid ID when specified"), nil
				}

				int64IDs := make([]int64, len(channelIDs))
				for i, id := range channelIDs {
					int64IDs[i] = int64(id)
				}
				input.ChannelIDs = int64IDs
			}

			output, err := client.ListChannels(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve channels: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"channels": output.Channels,
				"total":    output.Total,
			}), nil
		}
}

const queryEscalationRulesDescription = `Query escalation rules for a channel. Returns complete rules with notification layers, targets (persons/teams/schedules), webhooks, time filters and alert filters.`

// QueryEscalationRules creates a tool to query escalation rules
func QueryEscalationRules(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_escalation_rules",
			mcp.WithDescription(t("TOOL_QUERY_ESCALATION_RULES_DESCRIPTION", queryEscalationRulesDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_ESCALATION_RULES_USER_TITLE", "Query escalation rules"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithNumber("channel_id", mcp.Required(), mcp.Description("Collaboration space (channel) ID to query escalation rules for.")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			channelID, err := RequiredInt(request, "channel_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			output, err := client.ListEscalationRules(ctx, int64(channelID))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to query escalation rules: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"rules": output.Rules,
				"total": output.Total,
			}), nil
		}
}
