package flashduty

import (
	"context"
	"fmt"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const queryChangesDescription = `Query change records (deployments, configurations). Useful for correlating changes with incidents.`

// QueryChanges creates a tool to query change records
func QueryChanges(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_changes",
			mcp.WithDescription(t("TOOL_QUERY_CHANGES_DESCRIPTION", queryChangesDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_CHANGES_USER_TITLE", "Query changes"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("change_ids", mcp.Description("Comma-separated change IDs for direct lookup.")),
			mcp.WithNumber("channel_id", mcp.Description("Filter by collaboration space ID.")),
			mcp.WithNumber("start_time", mcp.Description("Query start time in Unix timestamp (seconds). Must be < end_time. Max range: 31 days. Defaults to 1 hour ago.")),
			mcp.WithNumber("end_time", mcp.Description("Query end time in Unix timestamp (seconds). Defaults to now.")),
			mcp.WithString("type", mcp.Description("Filter by change type.")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return."), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			changeIdsStr, _ := OptionalParam[string](request, "change_ids")
			channelID, _ := OptionalInt(request, "channel_id")
			startTime, _ := OptionalInt(request, "start_time")
			endTime, _ := OptionalInt(request, "end_time")
			changeType, _ := OptionalParam[string](request, "type")
			limit, _ := OptionalInt(request, "limit")

			if limit <= 0 {
				limit = 20
			}

			input := &sdk.ListChangesInput{
				ChannelID: int64(channelID),
				StartTime: int64(startTime),
				EndTime:   int64(endTime),
				Type:      changeType,
				Limit:     limit,
			}

			if changeIdsStr != "" {
				input.ChangeIDs = parseCommaSeparatedStrings(changeIdsStr)
			}

			output, err := client.ListChanges(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve changes: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"changes": output.Changes,
				"total":   output.Total,
			}), nil
		}
}
