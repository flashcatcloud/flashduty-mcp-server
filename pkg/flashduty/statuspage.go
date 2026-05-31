package flashduty

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	flashduty "github.com/flashcatcloud/go-flashduty"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/internal/timeutil"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
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

			wanted := make(map[int64]struct{})
			if pageIdsStr != "" {
				for _, id := range parseCommaSeparatedInts(pageIdsStr) {
					wanted[int64(id)] = struct{}{}
				}
			}

			resp, _, err := client.New.StatusPages.ReadPageList(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list status pages: %v", err)), nil
			}

			// ReadPageList returns every page; honor the optional page_ids
			// filter client-side since the endpoint takes no filter param.
			pages := resp.Items
			if len(wanted) > 0 {
				filtered := pages[:0]
				for _, p := range pages {
					if _, ok := wanted[p.PageID]; ok {
						filtered = append(filtered, p)
					}
				}
				pages = filtered
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

			// This tool lists *active* changes via /status-page/change/active/list,
			// which go-flashduty does not expose (its ChangeList hits the general
			// /status-page/change/list, which requires a single mandatory status
			// and a time window — different semantics). Keep the legacy SDK until
			// go-flashduty adds the active-list endpoint.
			// TODO: 待 go-flashduty 覆盖 /status-page/change/active/list 后切换并删除老 SDK 依赖。
			output, err := client.Legacy.ListStatusChanges(ctx, &sdk.ListStatusChangesInput{
				PageID:     int64(pageID),
				ChangeType: changeType,
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list status changes: %v", err)), nil
			}

			return MarshalLegacyResult(addTruncationHint(map[string]any{
				"changes": output.Changes,
				"total":   output.Total,
			}, len(output.Changes), output.Total)), nil
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

			if status == "" {
				status = "investigating"
			}

			// The initial update mirrors the incident: same status, the message
			// (or the title when no message), and the affected components.
			update := flashduty.CreateStatusPageChangeRequestUpdatesItem{
				AtSeconds: time.Now().Unix(),
				Status:    status,
			}
			if message != "" {
				update.Description = message
			}
			update.ComponentChanges = parseAffectedComponents(affectedComponents)

			description := message
			if description == "" {
				description = title
			}

			out, _, err := client.New.StatusPages.ChangeCreate(ctx, &flashduty.CreateStatusPageChangeRequest{
				PageID:            int64(pageID),
				Title:             title,
				Type:              "incident",
				Status:            status,
				Description:       description,
				Updates:           []flashduty.CreateStatusPageChangeRequestUpdatesItem{update},
				NotifySubscribers: notifySubscribers,
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to create status incident: %v", err)), nil
			}

			return MarshalResult(out), nil
		}
}

// parseAffectedComponents parses the "id1:degraded,id2:partial_outage" syntax
// the create_status_incident tool accepts. A bare id (no ":status") defaults to
// partial_outage, matching the legacy behavior.
func parseAffectedComponents(s string) []flashduty.CreateStatusPageChangeRequestUpdatesItemComponentChangesItem {
	if s == "" {
		return nil
	}
	var changes []flashduty.CreateStatusPageChangeRequestUpdatesItemComponentChangesItem
	for _, part := range parseCommaSeparatedStrings(s) {
		kv := strings.SplitN(part, ":", 2)
		switch {
		case len(kv) == 2:
			changes = append(changes, flashduty.CreateStatusPageChangeRequestUpdatesItemComponentChangesItem{
				ComponentID: strings.TrimSpace(kv[0]),
				Status:      strings.TrimSpace(kv[1]),
			})
		case len(kv) == 1 && kv[0] != "":
			changes = append(changes, flashduty.CreateStatusPageChangeRequestUpdatesItemComponentChangesItem{
				ComponentID: strings.TrimSpace(kv[0]),
				Status:      "partial_outage",
			})
		}
	}
	return changes
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
			mcp.WithString("at", mcp.Description("Timestamp for update. Accepts: relative duration like \"5m\" (interpreted as now minus duration); absolute date \"2026-04-01\"; datetime \"2026-04-01 10:00:00\"; unix seconds \"1712000000\"; or \"now\". Defaults to current time when omitted.")),
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

			status, _ := OptionalParam[string](request, "status")
			componentChanges, _ := OptionalParam[string](request, "component_changes")

			atSeconds, err := timeutil.ParseAny(request.GetArguments()["at"])
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid at: %v", err)), nil
			}

			req := &flashduty.CreateStatusPageChangeTimelineRequest{
				PageID:      int64(pageID),
				ChangeID:    int64(changeID),
				Description: message,
				AtSeconds:   atSeconds,
				Status:      status,
			}

			if componentChanges != "" {
				if err := json.Unmarshal([]byte(componentChanges), &req.ComponentChanges); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("component_changes must be a valid JSON array: %v", err)), nil
				}
			}

			if _, _, err := client.New.StatusPages.ChangeTimelineCreate(ctx, req); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to create timeline: %v", err)), nil
			}

			return MarshalResult(map[string]string{
				"status":  "success",
				"message": "Timeline entry created",
			}), nil
		}
}
