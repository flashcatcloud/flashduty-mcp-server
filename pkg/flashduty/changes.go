package flashduty

import (
	"context"
	"fmt"
	"time"

	flashduty "github.com/flashcatcloud/go-flashduty"
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
			mcp.WithNumber("limit", mcp.Description(LimitDescription), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
			mcp.WithNumber("page", mcp.Description(PageDescription), mcp.DefaultNumber(1), mcp.Min(1)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			args := request.GetArguments()
			changeIdsStr, _ := OptionalParam[string](request, "change_ids")
			channelIdsStr, _ := OptionalParam[string](request, "channel_ids")
			changeType, _ := OptionalParam[string](request, "type")
			limit, page := optionalPaging(request, defaultQueryLimit)

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

			req := &flashduty.ListChangeRequest{
				StartTime: startTime,
				EndTime:   endTime,
				Query:     changeType,
			}
			req.Limit = limit
			if page > 1 {
				req.Page = page
			}

			if channelIdsStr != "" {
				channelIDs := parseCommaSeparatedInts(channelIdsStr)
				if len(channelIDs) == 0 {
					return mcp.NewToolResultError("channel_ids must contain at least one valid ID when specified"), nil
				}
				req.ChannelIDs = make([]int64, len(channelIDs))
				for i, id := range channelIDs {
					req.ChannelIDs[i] = int64(id)
				}
			}

			resp, _, err := client.New.Changes.List(ctx, req)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve changes: %v", err)), nil
			}

			changes := resp.Items
			// /change/list has no server-side change_ids filter; honor the
			// direct-lookup param by filtering the fetched page client-side.
			// This is a best-effort single-page lookup — paging doesn't map onto
			// it, so report the found count as the total and skip the page hint.
			// (Feeding the filtered count with the window's unfiltered total into
			// addPageHint would mislead: "Showing 0 of 5000, request page:2",
			// inviting the agent to page the whole window chasing a few IDs.)
			if changeIdsStr != "" {
				wanted := make(map[string]struct{})
				for _, id := range parseCommaSeparatedStrings(changeIdsStr) {
					wanted[id] = struct{}{}
				}
				filtered := changes[:0]
				for _, ch := range changes {
					if _, ok := wanted[ch.ChangeID]; ok {
						filtered = append(filtered, ch)
					}
				}
				return MarshalResult(map[string]any{
					"changes": filtered,
					"total":   len(filtered),
				}), nil
			}

			return MarshalResult(addPageHint(map[string]any{
				"changes": changes,
				"total":   resp.Total,
			}, len(changes), int(resp.Total), page, limit)), nil
		}
}
