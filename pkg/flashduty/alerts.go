package flashduty

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/internal/timeutil"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const queryAlertsDescription = `Query alerts by time range and filters. Returns enriched data with channel/integration names. Useful for finding active or historical alerts that fed into incidents.`

// QueryAlerts creates a tool to query alerts with enriched data.
func QueryAlerts(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_alerts",
			mcp.WithDescription(t("TOOL_QUERY_ALERTS_DESCRIPTION", queryAlertsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_ALERTS_USER_TITLE", "Query alerts"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			WithSince(mcp.Required()),
			WithUntil(mcp.Required()),
			mcp.WithString("severity", mcp.Description("Filter by alert severity."), mcp.Enum("Info", "Warning", "Critical")),
			mcp.WithBoolean("is_active", mcp.Description("If true, only return alerts that are currently active (Triggered or Processing). If false, only inactive (Closed). If omitted, returns all.")),
			mcp.WithString("channel_ids", mcp.Description("Comma-separated collaboration space IDs to filter by.")),
			mcp.WithString("integration_ids", mcp.Description("Comma-separated integration IDs to filter by.")),
			mcp.WithString("alert_keys", mcp.Description("Comma-separated alert dedup keys for direct lookup.")),
			mcp.WithBoolean("ever_muted", mcp.Description("If true, only return alerts that were ever muted by a routing rule.")),
			mcp.WithString("title", mcp.Description("Keyword search in alert title.")),
			mcp.WithString("labels", mcp.Description("JSON object of label key-value pairs to match. Format: {\"resource\":\"web-01\",\"region\":\"us-west\"}.")),
			mcp.WithNumber("limit", mcp.Description(LimitDescription), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			args := request.GetArguments()

			startTime, err := timeutil.ParseAny(args["since"])
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid since: %v", err)), nil
			}
			endTime, err := timeutil.ParseAny(args["until"])
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid until: %v", err)), nil
			}
			if err := validateTimeWindow(startTime, endTime); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			severity, _ := OptionalParam[string](request, "severity")
			channelIdsStr, _ := OptionalParam[string](request, "channel_ids")
			integrationIdsStr, _ := OptionalParam[string](request, "integration_ids")
			alertKeysStr, _ := OptionalParam[string](request, "alert_keys")
			title, _ := OptionalParam[string](request, "title")
			labelsStr, _ := OptionalParam[string](request, "labels")
			limit, _ := OptionalInt(request, "limit")
			if limit <= 0 {
				limit = defaultQueryLimit
			}

			input := &sdk.ListAlertsInput{
				StartTime:     startTime,
				EndTime:       endTime,
				AlertSeverity: severity,
				Title:         title,
				Limit:         limit,
			}

			if v, ok := args["is_active"].(bool); ok {
				input.IsActive = &v
			}
			if v, ok := args["ever_muted"].(bool); ok {
				input.EverMuted = &v
			}

			if channelIdsStr != "" {
				ids := parseCommaSeparatedInts(channelIdsStr)
				if len(ids) == 0 {
					return mcp.NewToolResultError("channel_ids must contain at least one valid ID when specified"), nil
				}
				input.ChannelIDs = make([]int64, len(ids))
				for i, id := range ids {
					input.ChannelIDs[i] = int64(id)
				}
			}
			if integrationIdsStr != "" {
				ids := parseCommaSeparatedInts(integrationIdsStr)
				if len(ids) == 0 {
					return mcp.NewToolResultError("integration_ids must contain at least one valid ID when specified"), nil
				}
				input.IntegrationIDs = make([]int64, len(ids))
				for i, id := range ids {
					input.IntegrationIDs[i] = int64(id)
				}
			}
			if alertKeysStr != "" {
				input.AlertKeys = parseCommaSeparatedStrings(alertKeysStr)
			}
			if labelsStr != "" {
				labels := map[string]string{}
				if err := json.Unmarshal([]byte(labelsStr), &labels); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("invalid labels JSON: %v", err)), nil
				}
				if len(labels) > 0 {
					input.Labels = labels
				}
			}

			output, err := client.ListAlerts(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve alerts: %v", err)), nil
			}

			return MarshalResult(addTruncationHint(map[string]any{
				"alerts":           output.Alerts,
				"total":            output.Total,
				"has_next_page":    output.HasNextPage,
				"search_after_ctx": output.SearchAfterCtx,
			}, len(output.Alerts), output.Total)), nil
		}
}

const queryAlertEventsDescription = `Query raw events for a single alert. Returns the upstream event stream that produced the alert (e.g. each individual Prometheus firing).`

// QueryAlertEvents creates a tool to query raw events of a single alert.
func QueryAlertEvents(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_alert_events",
			mcp.WithDescription(t("TOOL_QUERY_ALERT_EVENTS_DESCRIPTION", queryAlertEventsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_ALERT_EVENTS_USER_TITLE", "Query alert events"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("alert_id", mcp.Required(), mcp.Description("Alert ID whose raw events should be returned.")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			alertID, err := RequiredParam[string](request, "alert_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			output, err := client.ListAlertEvents(ctx, &sdk.ListAlertEventsInput{AlertID: alertID})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve alert events: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"alert_events": output.AlertEvents,
			}), nil
		}
}
