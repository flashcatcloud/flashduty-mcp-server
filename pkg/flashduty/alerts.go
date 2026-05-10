package flashduty

import (
	"context"
	"fmt"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

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
