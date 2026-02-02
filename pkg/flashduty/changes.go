package flashduty

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/sync/errgroup"

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

			requestBody := map[string]interface{}{
				"p":     1,
				"limit": limit,
			}

			if changeIdsStr != "" {
				changeIDs := parseCommaSeparatedStrings(changeIdsStr)
				requestBody["change_ids"] = changeIDs
			}
			if channelID > 0 {
				requestBody["channel_id"] = channelID
			}
			if startTime > 0 {
				requestBody["start_time"] = startTime
			}
			if endTime > 0 {
				requestBody["end_time"] = endTime
			}
			if changeType != "" {
				requestBody["type"] = changeType
			}

			resp, err := client.makeRequest(ctx, "POST", "/change/list", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to query changes: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(handleAPIError(resp).Error()), nil
			}

			var result struct {
				Error *DutyError `json:"error,omitempty"`
				Data  *struct {
					Items []struct {
						ChangeID    string            `json:"change_id"`
						Title       string            `json:"title"`
						Description string            `json:"description,omitempty"`
						Type        string            `json:"type,omitempty"`
						Status      string            `json:"status,omitempty"`
						ChannelID   int64             `json:"channel_id,omitempty"`
						CreatorID   int64             `json:"creator_id,omitempty"`
						StartTime   int64             `json:"start_time,omitempty"`
						EndTime     int64             `json:"end_time,omitempty"`
						Labels      map[string]string `json:"labels,omitempty"`
					} `json:"items"`
					Total int `json:"total"`
				} `json:"data,omitempty"`
			}
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			if result.Data == nil || len(result.Data.Items) == 0 {
				return MarshalResult(map[string]any{
					"changes": []Change{},
					"total":   0,
				}), nil
			}

			// Collect IDs for enrichment
			channelIDs := make([]int64, 0)
			personIDs := make([]int64, 0)
			for _, item := range result.Data.Items {
				if item.ChannelID != 0 {
					channelIDs = append(channelIDs, item.ChannelID)
				}
				if item.CreatorID != 0 {
					personIDs = append(personIDs, item.CreatorID)
				}
			}

			// Fetch enrichment data concurrently
			var channelMap map[int64]ChannelInfo
			var personMap map[int64]PersonInfo
			g, gctx := errgroup.WithContext(ctx)

			g.Go(func() error {
				channelMap, _ = client.fetchChannelInfos(gctx, channelIDs)
				return nil
			})

			g.Go(func() error {
				personMap, _ = client.fetchPersonInfos(gctx, personIDs)
				return nil
			})

			_ = g.Wait() // Ignore errors for enrichment as it's best-effort

			// Build enriched changes
			changes := make([]Change, 0, len(result.Data.Items))
			for _, item := range result.Data.Items {
				change := Change{
					ChangeID:    item.ChangeID,
					Title:       item.Title,
					Description: item.Description,
					Type:        item.Type,
					Status:      item.Status,
					ChannelID:   item.ChannelID,
					CreatorID:   item.CreatorID,
					StartTime:   item.StartTime,
					EndTime:     item.EndTime,
					Labels:      item.Labels,
				}

				if ch, ok := channelMap[item.ChannelID]; ok {
					change.ChannelName = ch.ChannelName
				}
				if p, ok := personMap[item.CreatorID]; ok {
					change.CreatorName = p.PersonName
				}

				changes = append(changes, change)
			}

			return MarshalResult(map[string]any{
				"changes": changes,
				"total":   result.Data.Total,
			}), nil
		}
}
