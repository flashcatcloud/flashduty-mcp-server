package flashduty

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const queryStatusPagesDescription = `Query status pages with full configuration.

**Parameters:**
- page_ids (optional): Comma-separated page IDs for direct lookup. If not provided, lists all pages.

**Returns:**
- Status pages with sections, components, and overall status`

// QueryStatusPages creates a tool to query status pages
func QueryStatusPages(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_status_pages",
			mcp.WithDescription(t("TOOL_QUERY_STATUS_PAGES_DESCRIPTION", queryStatusPagesDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_STATUS_PAGES_USER_TITLE", "Query status pages"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("page_ids", mcp.Description("Comma-separated status page IDs")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			pageIdsStr, _ := OptionalParam[string](request, "page_ids")

			// List all pages first
			resp, err := client.makeRequest(ctx, "GET", "/status-page/list", nil)
			if err != nil {
				return nil, fmt.Errorf("failed to list status pages: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(handleAPIError(resp).Error()), nil
			}

			var result struct {
				Error *DutyError `json:"error,omitempty"`
				Data  *struct {
					Items []struct {
						PageID      int64  `json:"page_id"`
						PageName    string `json:"name"`
						URLName     string `json:"url_name,omitempty"`
						Description string `json:"description,omitempty"`
						Components  []struct {
							ComponentID string `json:"component_id"`
							Name        string `json:"name"`
						} `json:"components,omitempty"`
					} `json:"items"`
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
					"pages": []StatusPage{},
					"total": 0,
				}), nil
			}

			// Filter by page_ids if provided
			var pageIDs []int64
			if pageIdsStr != "" {
				for _, id := range parseCommaSeparatedInts(pageIdsStr) {
					pageIDs = append(pageIDs, int64(id))
				}
			}

			pages := make([]StatusPage, 0)
			for _, item := range result.Data.Items {
				// Skip if filtering and not in list
				if len(pageIDs) > 0 {
					found := false
					for _, id := range pageIDs {
						if id == item.PageID {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				}

				page := StatusPage{
					PageID:      item.PageID,
					PageName:    item.PageName,
					Slug:        item.URLName,
					Description: item.Description,
				}

				// Convert components and calculate overall status
				worstStatus := "operational"
				if len(item.Components) > 0 {
					page.Components = make([]StatusComponent, 0, len(item.Components))
					for _, comp := range item.Components {
						page.Components = append(page.Components, StatusComponent{
							ComponentID:   comp.ComponentID,
							ComponentName: comp.Name,
							Status:        "operational", // Default status
						})
					}
				}
				page.OverallStatus = worstStatus

				pages = append(pages, page)
			}

			return MarshalResult(map[string]any{
				"pages": pages,
				"total": len(pages),
			}), nil
		}
}

const listStatusChangesDescription = `List active change events (incidents/maintenances) on a status page.

**Parameters:**
- page_id (required): Status page ID
- type (required): Change type - "incident" or "maintenance"

**Returns:**
- List of active change events (non-resolved/completed)
- For incidents: returns those with status investigating/identified/monitoring (not resolved)
- For maintenances: returns those with status scheduled/ongoing (not completed)`

// ListStatusChanges creates a tool to list status page changes
func ListStatusChanges(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_status_changes",
			mcp.WithDescription(t("TOOL_LIST_STATUS_CHANGES_DESCRIPTION", listStatusChangesDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_STATUS_CHANGES_USER_TITLE", "List status changes"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithNumber("page_id", mcp.Required(), mcp.Description("Status page ID")),
			mcp.WithString("type", mcp.Required(), mcp.Description("Change type: incident or maintenance")),
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

			// Use GET for active list endpoint
			resp, err := client.makeRequest(ctx, "GET", fmt.Sprintf("/status-page/change/active/list?page_id=%d&type=%s", pageID, changeType), nil)
			if err != nil {
				return nil, fmt.Errorf("failed to list status changes: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(handleAPIError(resp).Error()), nil
			}

			var result struct {
				Error *DutyError `json:"error,omitempty"`
				Data  *struct {
					Items []StatusChange `json:"items"`
					Total int            `json:"total"`
				} `json:"data,omitempty"`
			}
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			changes := []StatusChange{}
			total := 0
			if result.Data != nil {
				changes = result.Data.Items
				total = result.Data.Total
			}

			return MarshalResult(map[string]any{
				"changes": changes,
				"total":   total,
			}), nil
		}
}

const createStatusIncidentDescription = `Create a new incident on a status page.

**Parameters:**
- page_id (required): Status page ID
- title (required): Incident title (max 255 characters)
- message (optional): Initial update message describing the incident (required for the incident description)
- status (optional): Status - investigating, identified, monitoring, resolved (default: investigating)
- affected_components (optional): Comma-separated component IDs with status, format: "id1:degraded,id2:partial_outage"
  - Component statuses for incidents: degraded, partial_outage, full_outage
  - At least one component change is required
- notify_subscribers (optional): Whether to notify subscribers (default: true)

**Returns:**
- Created change event ID

**Notes:**
- The message/description field is required for creating an incident`

// CreateStatusIncident creates a tool to create status page incident
func CreateStatusIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_status_incident",
			mcp.WithDescription(t("TOOL_CREATE_STATUS_INCIDENT_DESCRIPTION", createStatusIncidentDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_STATUS_INCIDENT_USER_TITLE", "Create status incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithNumber("page_id", mcp.Required(), mcp.Description("Status page ID")),
			mcp.WithString("title", mcp.Required(), mcp.Description("Incident title")),
			mcp.WithString("message", mcp.Description("Initial update message")),
			mcp.WithString("status", mcp.Description("Status: investigating, identified, monitoring, resolved")),
			mcp.WithString("affected_components", mcp.Description("Component IDs with status: id1:degraded,id2:partial_outage")),
			mcp.WithBoolean("notify_subscribers", mcp.Description("Notify subscribers (default: true)")),
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

			// Build the initial update
			update := map[string]interface{}{
				"at_seconds": time.Now().Unix(),
				"status":     status,
			}
			if message != "" {
				update["description"] = message
			}

			// Parse component changes if provided (format: "id1:status1,id2:status2")
			if affectedComponents != "" {
				var componentChanges []map[string]string
				parts := parseCommaSeparatedStrings(affectedComponents)
				for _, part := range parts {
					kv := strings.SplitN(part, ":", 2)
					if len(kv) == 2 {
						componentChanges = append(componentChanges, map[string]string{
							"component_id": strings.TrimSpace(kv[0]),
							"status":       strings.TrimSpace(kv[1]),
						})
					} else if len(kv) == 1 && kv[0] != "" {
						// Default to partial_outage if no status specified
						componentChanges = append(componentChanges, map[string]string{
							"component_id": strings.TrimSpace(kv[0]),
							"status":       "partial_outage",
						})
					}
				}
				if len(componentChanges) > 0 {
					update["component_changes"] = componentChanges
				}
			}

			// Use message as both change description and first update description
			description := message
			if description == "" {
				description = title // Fallback to title if no message provided
			}

			requestBody := map[string]interface{}{
				"page_id":     pageID,
				"title":       title,
				"type":        "incident",
				"status":      status,
				"description": description,
				"updates":     []map[string]interface{}{update},
			}

			// Default notify_subscribers to true
			requestBody["notify_subscribers"] = notifySubscribers

			resp, err := client.makeRequest(ctx, "POST", "/status-page/change/create", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to create status incident: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalResult(result.Data), nil
		}
}

const createChangeTimelineDescription = `Add a timeline update to a status page change event.

**Parameters:**
- page_id (required): Status page ID
- change_id (required): Change event ID to update
- message (required): Update message describing the change
- at (optional): Timestamp for the update (Unix timestamp in seconds, default: now)
- status (optional): New status for incidents - investigating, identified, monitoring, resolved
  - For maintenances use: scheduled, ongoing, completed
- component_changes (optional): JSON array of component status changes, e.g. [{"component_id":"xxx","status":"degraded"}]
  - Component statuses: operational, degraded, partial_outage, full_outage

**Use cases:**
- Post investigation updates
- Mark incident as resolved
- Update affected components

**Returns:**
- Success confirmation`

// CreateChangeTimeline creates a tool to add timeline entry to status change
func CreateChangeTimeline(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_change_timeline",
			mcp.WithDescription(t("TOOL_CREATE_CHANGE_TIMELINE_DESCRIPTION", createChangeTimelineDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_CHANGE_TIMELINE_USER_TITLE", "Create change timeline"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithNumber("page_id", mcp.Required(), mcp.Description("Status page ID")),
			mcp.WithNumber("change_id", mcp.Required(), mcp.Description("Change event ID")),
			mcp.WithString("message", mcp.Required(), mcp.Description("Update message (required)")),
			mcp.WithNumber("at", mcp.Description("Timestamp (Unix timestamp, default: now)")),
			mcp.WithString("status", mcp.Description("New status: investigating, identified, monitoring, resolved")),
			mcp.WithString("component_changes", mcp.Description("JSON array: [{\"component_id\":\"xxx\",\"status\":\"degraded\"}]")),
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

			requestBody := map[string]interface{}{
				"page_id":     pageID,
				"change_id":   changeID,
				"description": message,
			}
			if at > 0 {
				requestBody["at_seconds"] = at
			}
			if status != "" {
				requestBody["status"] = status
			}
			if componentChanges != "" {
				// Parse JSON array if provided
				var changes []map[string]string
				if err := json.Unmarshal([]byte(componentChanges), &changes); err == nil {
					requestBody["component_changes"] = changes
				}
			}

			resp, err := client.makeRequest(ctx, "POST", "/status-page/change/timeline/create", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to create timeline: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalResult(map[string]string{
				"status":  "success",
				"message": "Timeline entry created",
			}), nil
		}
}
