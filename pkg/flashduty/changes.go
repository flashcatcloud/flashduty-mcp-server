package flashduty

import (
	"context"
	"fmt"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/internal/timeutil"
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
			mcp.WithString("channel_ids", mcp.Description("Comma-separated collaboration space IDs to filter by. Backend expects an array — singular channel_id is silently ignored.")),
			mcp.WithString("start_time", mcp.Description("Query start time. Accepts: relative duration like \"1h\", \"24h\", \"7d\" (interpreted as now minus duration); absolute date \"2026-04-01\"; datetime \"2026-04-01 10:00:00\"; unix seconds \"1712000000\"; or \"now\". Defaults to 1 hour ago. Max range: 31 days.")),
			mcp.WithString("end_time", mcp.Description("Query end time. Same formats as start_time, plus future durations like \"+24h\". Defaults to \"now\".")),
			mcp.WithString("type", mcp.Description("Filter by change type.")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return."), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			changeIdsStr, _ := OptionalParam[string](request, "change_ids")
			channelIdsStr, _ := OptionalParam[string](request, "channel_ids")
			startTimeStr, _ := OptionalParam[string](request, "start_time")
			endTimeStr, _ := OptionalParam[string](request, "end_time")
			changeType, _ := OptionalParam[string](request, "type")
			limit, _ := OptionalInt(request, "limit")

			if limit <= 0 {
				limit = 20
			}

			var startTime, endTime int64
			if startTimeStr != "" {
				v, err := timeutil.Parse(startTimeStr)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("invalid start_time: %v", err)), nil
				}
				startTime = v
			}
			if endTimeStr != "" {
				v, err := timeutil.Parse(endTimeStr)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("invalid end_time: %v", err)), nil
				}
				endTime = v
			}

			input := &sdk.ListChangesInput{
				StartTime: startTime,
				EndTime:   endTime,
				Type:      changeType,
				Limit:     limit,
			}

			if changeIdsStr != "" {
				input.ChangeIDs = parseCommaSeparatedStrings(changeIdsStr)
			}

			if channelIdsStr != "" {
				channelIDs := parseCommaSeparatedInts(channelIdsStr)
				if len(channelIDs) == 0 {
					return mcp.NewToolResultError("channel_ids must contain at least one valid ID when specified"), nil
				}
				input.ChannelIDs = make([]int64, len(channelIDs))
				for i, id := range channelIDs {
					input.ChannelIDs[i] = int64(id)
				}
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
