package flashduty

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
	sdk "github.com/flashcatcloud/flashduty-sdk"
)

const queryStatusPagesDescription = `Query status pages with components. Lists all pages or filter by IDs.`

// QueryStatusPages creates a tool to query status pages
func QueryStatusPages(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_status_pages",
			mcp.WithDescription(t("TOOL_QUERY_STATUS_PAGES_DESCRIPTION", queryStatusPagesDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_STATUS_PAGES_USER_TITLE", "Query status pages"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("page_ids", mcp.Description("Comma-separated status page IDs for direct lookup. If not provided, returns all pages.")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			pageIdsStr, _ := OptionalParam[string](request, "page_ids")

			var pageIDs []int64
			if pageIdsStr != "" {
				for _, id := range parseCommaSeparatedInts(pageIdsStr) {
					pageIDs = append(pageIDs, int64(id))
				}
			}

			pages, err := client.ListStatusPages(ctx, pageIDs)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list status pages: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"pages": pages,
				"total": len(pages),
			}), nil
		}
}

const listStatusChangesDescription = `List active incidents or maintenances on a status page. Returns non-resolved/non-completed events.`

// ListStatusChanges creates a tool to list status page changes
func ListStatusChanges(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_status_changes",
			mcp.WithDescription(t("TOOL_LIST_STATUS_CHANGES_DESCRIPTION", listStatusChangesDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_STATUS_CHANGES_USER_TITLE", "List status changes"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithNumber("page_id", mcp.Required(), mcp.Description("Status page ID to query changes for.")),
			mcp.WithString("type", mcp.Required(), mcp.Description("Type of change events to list."), mcp.Enum("incident", "maintenance")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			pageID, err := RequiredInt(request, "page_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			changeType, err := RequiredParam[string](request, "type")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if changeType != "incident" && changeType != "maintenance" {
				return mcp.NewToolResultError("type must be 'incident' or 'maintenance'"), nil
			}

			output, err := client.ListStatusChanges(ctx, &sdk.ListStatusChangesInput{
				PageID:     int64(pageID),
				ChangeType: changeType,
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list status changes: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"changes": output.Changes,
				"total":   output.Total,
			}), nil
		}
}

const createStatusIncidentDescription = `Create an incident on a status page with affected components and status updates.`

// CreateStatusIncident creates a tool to create status page incident
func CreateStatusIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_status_incident",
			mcp.WithDescription(t("TOOL_CREATE_STATUS_INCIDENT_DESCRIPTION", createStatusIncidentDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_STATUS_INCIDENT_USER_TITLE", "Create status incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithNumber("page_id", mcp.Required(), mcp.Description("Status page ID to create incident on.")),
			mcp.WithString("title", mcp.Required(), mcp.Description("Incident title. Max 255 characters."), mcp.MaxLength(255)),
			mcp.WithString("message", mcp.Description("Initial update message describing the incident.")),
			mcp.WithString("status", mcp.Description("Initial incident status."), mcp.Enum("investigating", "identified", "monitoring", "resolved"), mcp.DefaultString("investigating")),
			mcp.WithString("affected_components", mcp.Description("Comma-separated component IDs with status. Format: id1:degraded,id2:partial_outage. Valid statuses: degraded, partial_outage, full_outage.")),
			mcp.WithBoolean("notify_subscribers", mcp.Description("Whether to notify page subscribers."), mcp.DefaultBool(true)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			pageID, err := RequiredInt(request, "page_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			title, err := RequiredParam[string](request, "title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			message, _ := OptionalParam[string](request, "message")
			status, _ := OptionalParam[string](request, "status")
			affectedComponents, _ := OptionalParam[string](request, "affected_components")
			notifySubscribers, _ := OptionalParam[bool](request, "notify_subscribers")

			data, err := client.CreateStatusIncident(ctx, &sdk.CreateStatusIncidentInput{
				PageID:             int64(pageID),
				Title:              title,
				Message:            message,
				Status:             status,
				AffectedComponents: affectedComponents,
				NotifySubscribers:  notifySubscribers,
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to create status incident: %v", err)), nil
			}

			return MarshalResult(data), nil
		}
}

const createChangeTimelineDescription = `Add a timeline update to a status page incident or maintenance. Update status and affected components.`

// CreateChangeTimeline creates a tool to add timeline entry to status change
func CreateChangeTimeline(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_change_timeline",
			mcp.WithDescription(t("TOOL_CREATE_CHANGE_TIMELINE_DESCRIPTION", createChangeTimelineDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_CHANGE_TIMELINE_USER_TITLE", "Create change timeline"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithNumber("page_id", mcp.Required(), mcp.Description("Status page ID.")),
			mcp.WithNumber("change_id", mcp.Required(), mcp.Description("Change event ID (incident or maintenance) to update.")),
			mcp.WithString("message", mcp.Required(), mcp.Description("Update message describing the timeline entry.")),
			mcp.WithNumber("at", mcp.Description("Timestamp for update in Unix seconds. Defaults to current time.")),
			mcp.WithString("status", mcp.Description("New status. For incidents: investigating, identified, monitoring, resolved. For maintenances: scheduled, ongoing, completed.")),
			mcp.WithString("component_changes", mcp.Description("JSON array of component status changes. Format: [{\"component_id\":\"xxx\",\"status\":\"degraded\"}]. Valid statuses: operational, degraded, partial_outage, full_outage.")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			pageID, err := RequiredInt(request, "page_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			changeID, err := RequiredInt(request, "change_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			message, err := RequiredParam[string](request, "message")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			at, _ := OptionalInt(request, "at")
			status, _ := OptionalParam[string](request, "status")
			componentChanges, _ := OptionalParam[string](request, "component_changes")

			err = client.CreateChangeTimeline(ctx, &sdk.CreateChangeTimelineInput{
				PageID:           int64(pageID),
				ChangeID:         int64(changeID),
				Message:          message,
				AtSeconds:        int64(at),
				Status:           status,
				ComponentChanges: componentChanges,
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to create timeline: %v", err)), nil
			}

			return MarshalResult(map[string]string{
				"status":  "success",
				"message": "Timeline entry created",
			}), nil
		}
}
