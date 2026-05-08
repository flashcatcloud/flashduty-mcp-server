package flashduty

import (
	"context"
	"fmt"
	"time"

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
			WithSince(),
			WithUntil(),
			mcp.WithString("type", mcp.Description("Filter by change type.")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return."), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			args := request.GetArguments()
			changeIdsStr, _ := OptionalParam[string](request, "change_ids")
			channelIdsStr, _ := OptionalParam[string](request, "channel_ids")
			changeType, _ := OptionalParam[string](request, "type")
			limit, _ := OptionalInt(request, "limit")

			if limit <= 0 {
				limit = 20
			}

			startTime, err := timeutil.ParseAny(args["since"])
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid since: %v", err)), nil
			}
			endTime, err := timeutil.ParseAny(args["until"])
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid until: %v", err)), nil
			}
			// Honor the "defaults to last hour" contract advertised in the old
			// description: backend rejects 0/0, so we have to apply it ourselves.
			if endTime == 0 {
				endTime = time.Now().Unix()
			}
			if startTime == 0 {
				startTime = endTime - 3600
			}
			if err := validateTimeWindow(startTime, endTime); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
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
