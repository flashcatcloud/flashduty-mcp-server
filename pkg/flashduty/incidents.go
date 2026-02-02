package flashduty

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/sync/errgroup"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const defaultQueryLimit = 20

const queryIncidentsDescription = `Query incidents by IDs, time range, status, severity, or channel. Returns enriched data with names.`

// QueryIncidents creates a tool to query incidents with enriched data
func QueryIncidents(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_incidents",
			mcp.WithDescription(t("TOOL_QUERY_INCIDENTS_DESCRIPTION", queryIncidentsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_INCIDENTS_USER_TITLE", "Query incidents"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("incident_ids", mcp.Description("Comma-separated incident IDs for direct lookup. If provided, other filters are ignored.")),
			mcp.WithString("progress", mcp.Description("Filter by status. Valid values: Triggered, Processing, Closed. Comma-separated for multiple."), mcp.Enum("Triggered", "Processing", "Closed", "Triggered,Processing", "Processing,Closed", "Triggered,Closed", "Triggered,Processing,Closed")),
			mcp.WithString("severity", mcp.Description("Filter by severity level. Valid values: Info, Warning, Critical."), mcp.Enum("Info", "Warning", "Critical")),
			mcp.WithNumber("channel_id", mcp.Description("Filter by collaboration space ID.")),
			mcp.WithNumber("start_time", mcp.Description("Query start time in Unix timestamp (seconds). Required if no incident_ids. Must be < end_time. Max range: 31 days.")),
			mcp.WithNumber("end_time", mcp.Description("Query end time in Unix timestamp (seconds). Required if no incident_ids. Must be within data retention period.")),
			mcp.WithString("title", mcp.Description("Keyword search in incident title.")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return."), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
			mcp.WithBoolean("include_alerts", mcp.Description("Whether to include alerts preview (first 20 alerts with total count)."), mcp.DefaultBool(true)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			// Extract parameters
			incidentIdsStr, _ := OptionalParam[string](request, "incident_ids")
			progress, _ := OptionalParam[string](request, "progress")
			severity, _ := OptionalParam[string](request, "severity")
			channelID, _ := OptionalInt(request, "channel_id")
			startTime, _ := OptionalInt(request, "start_time")
			endTime, _ := OptionalInt(request, "end_time")
			title, _ := OptionalParam[string](request, "title")
			limit, _ := OptionalInt(request, "limit")

			// Default include_alerts to true if not explicitly set to false
			includeAlerts := true
			if v, ok := request.GetArguments()["include_alerts"].(bool); ok {
				includeAlerts = v
			}

			if limit <= 0 {
				limit = defaultQueryLimit
			}

			var rawIncidents []RawIncident

			// Query by IDs or by filters
			if incidentIdsStr != "" {
				incidentIDs := parseCommaSeparatedStrings(incidentIdsStr)
				if len(incidentIDs) == 0 {
					return mcp.NewToolResultError("incident_ids must contain at least one valid ID when specified"), nil
				}
				rawIncidents, err = client.fetchIncidentsByIDs(ctx, incidentIDs)
			} else {
				if startTime == 0 || endTime == 0 {
					return mcp.NewToolResultError("Both start_time and end_time are required for time-based queries"), nil
				}
				rawIncidents, err = client.fetchIncidentsByFilters(ctx, progress, severity, channelID, startTime, endTime, title, limit)
			}

			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve incidents: %v", err)), nil
			}

			if len(rawIncidents) == 0 {
				return MarshalResult(map[string]any{
					"incidents": []EnrichedIncident{},
					"total":     0,
				}), nil
			}

			// Enrich incidents with person/channel names
			enrichedIncidents, err := client.enrichIncidents(ctx, rawIncidents)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to load additional incident details: %v", err)), nil
			}

			// Fetch alerts concurrently if requested
			if includeAlerts && len(enrichedIncidents) > 0 {
				g, gctx := errgroup.WithContext(ctx)
				for i := range enrichedIncidents {
					i := i
					incidentID := enrichedIncidents[i].IncidentID
					g.Go(func() error {
						alerts, total, err := client.fetchIncidentAlerts(gctx, incidentID, defaultQueryLimit)
						if err != nil {
							return err
						}
						enrichedIncidents[i].AlertsPreview = alerts
						enrichedIncidents[i].AlertsTotal = total
						return nil
					})
				}
				if err := g.Wait(); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve alerts: %v", err)), nil
				}
			}

			return MarshalResult(map[string]any{
				"incidents": enrichedIncidents,
				"total":     len(enrichedIncidents),
			}), nil
		}
}

const queryIncidentTimelineDescription = `Query timeline events for incidents. Returns events like created, assigned, acknowledged, resolved, notifications.`

// QueryIncidentTimeline creates a tool to query incident timeline
func QueryIncidentTimeline(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_incident_timeline",
			mcp.WithDescription(t("TOOL_QUERY_INCIDENT_TIMELINE_DESCRIPTION", queryIncidentTimelineDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_INCIDENT_TIMELINE_USER_TITLE", "Query incident timeline"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("incident_ids", mcp.Required(), mcp.Description("Comma-separated incident IDs to query timeline for. Event types: i_new (created), i_assign (assigned), i_ack (acknowledged), i_rslv (resolved), i_notify (notification), i_comm (comment), i_r_* (field updates).")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			incidentIdsStr, err := RequiredParam[string](request, "incident_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			incidentIDs := parseCommaSeparatedStrings(incidentIdsStr)
			if len(incidentIDs) == 0 {
				return mcp.NewToolResultError("incident_ids must contain at least one valid ID"), nil
			}

			// Fetch all timelines concurrently
			type timelineResult struct {
				IncidentID string
				Items      []RawTimelineItem
			}
			results := make([]timelineResult, len(incidentIDs))
			allPersonIDs := make([]int64, 0)

			g, gctx := errgroup.WithContext(ctx)
			for i, id := range incidentIDs {
				i, id := i, id
				g.Go(func() error {
					items, err := client.fetchIncidentTimeline(gctx, id)
					if err != nil {
						return err
					}
					results[i] = timelineResult{IncidentID: id, Items: items}
					return nil
				})
			}

			if err := g.Wait(); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve timeline: %v", err)), nil
			}

			// Collect all person IDs from all timelines
			for _, r := range results {
				allPersonIDs = append(allPersonIDs, collectTimelinePersonIDs(r.Items)...)
			}

			// Batch fetch person info (use original ctx, not errgroup's ctx)
			personMap, err := client.fetchPersonInfos(ctx, allPersonIDs)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to load person details: %v", err)), nil
			}

			// Build enriched response
			response := make([]map[string]any, 0, len(results))
			for _, r := range results {
				enrichedEvents := enrichTimelineItems(r.Items, personMap)
				response = append(response, map[string]any{
					"incident_id": r.IncidentID,
					"timeline":    enrichedEvents,
					"total":       len(enrichedEvents),
				})
			}

			return MarshalResult(map[string]any{
				"results": response,
			}), nil
		}
}

const queryIncidentAlertsDescription = `Query alerts for incidents. Returns alerts with title, severity, status, and labels.`

// QueryIncidentAlerts creates a tool to query incident alerts
func QueryIncidentAlerts(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_incident_alerts",
			mcp.WithDescription(t("TOOL_QUERY_INCIDENT_ALERTS_DESCRIPTION", queryIncidentAlertsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_INCIDENT_ALERTS_USER_TITLE", "Query incident alerts"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("incident_ids", mcp.Required(), mcp.Description("Comma-separated incident IDs to query alerts for.")),
			mcp.WithNumber("limit", mcp.Description("Maximum alerts per incident."), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			incidentIdsStr, err := RequiredParam[string](request, "incident_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			incidentIDs := parseCommaSeparatedStrings(incidentIdsStr)
			if len(incidentIDs) == 0 {
				return mcp.NewToolResultError("incident_ids must contain at least one valid ID"), nil
			}

			limit, _ := OptionalInt(request, "limit")
			if limit <= 0 {
				limit = defaultQueryLimit
			}

			// Fetch all alerts concurrently
			type alertsResult struct {
				IncidentID string
				Alerts     []AlertPreview
				Total      int
			}
			results := make([]alertsResult, len(incidentIDs))

			g, gctx := errgroup.WithContext(ctx)
			for i, id := range incidentIDs {
				i, id := i, id
				g.Go(func() error {
					alerts, total, err := client.fetchIncidentAlerts(gctx, id, limit)
					if err != nil {
						return err
					}
					results[i] = alertsResult{IncidentID: id, Alerts: alerts, Total: total}
					return nil
				})
			}

			if err := g.Wait(); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve alerts: %v", err)), nil
			}

			// Build response
			response := make([]map[string]any, 0, len(results))
			for _, r := range results {
				response = append(response, map[string]any{
					"incident_id": r.IncidentID,
					"alerts":      r.Alerts,
					"total":       r.Total,
				})
			}

			return MarshalResult(map[string]any{
				"results": response,
			}), nil
		}
}

// fetchIncidentsByIDs fetches incidents by their IDs
func (c *Client) fetchIncidentsByIDs(ctx context.Context, incidentIDs []string) ([]RawIncident, error) {
	requestBody := map[string]interface{}{
		"incident_ids": incidentIDs,
	}

	resp, err := c.makeRequest(ctx, "POST", "/incident/list-by-ids", requestBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, handleAPIError(resp)
	}

	var result struct {
		Error *DutyError `json:"error,omitempty"`
		Data  *struct {
			Items []RawIncident `json:"items"`
		} `json:"data,omitempty"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}
	if result.Data == nil {
		return nil, nil
	}
	return result.Data.Items, nil
}

// fetchIncidentsByFilters fetches incidents by filters
func (c *Client) fetchIncidentsByFilters(ctx context.Context, progress, severity string, channelID int, startTime, endTime int, title string, limit int) ([]RawIncident, error) {
	requestBody := map[string]interface{}{
		"p":          1,
		"limit":      limit,
		"start_time": startTime,
		"end_time":   endTime,
	}

	if progress != "" {
		requestBody["progress"] = progress
	}
	if severity != "" {
		requestBody["incident_severity"] = severity
	}
	if channelID > 0 {
		requestBody["channel_id"] = channelID
	}
	if title != "" {
		requestBody["title"] = title
	}

	resp, err := c.makeRequest(ctx, "POST", "/incident/list", requestBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, handleAPIError(resp)
	}

	var result struct {
		Error *DutyError `json:"error,omitempty"`
		Data  *struct {
			Items []RawIncident `json:"items"`
		} `json:"data,omitempty"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}
	if result.Data == nil {
		return nil, nil
	}
	return result.Data.Items, nil
}

const createIncidentDescription = `Create a new incident with title and severity. Optionally assign to channel or responders.`

// CreateIncident creates a tool to create a new incident
func CreateIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_incident",
			mcp.WithDescription(t("TOOL_CREATE_INCIDENT_DESCRIPTION", createIncidentDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_INCIDENT_USER_TITLE", "Create incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("title", mcp.Required(), mcp.Description("Incident title. Length: 3-200 characters."), mcp.MinLength(3), mcp.MaxLength(200)),
			mcp.WithString("severity", mcp.Required(), mcp.Description("Incident severity level."), mcp.Enum("Info", "Warning", "Critical")),
			mcp.WithNumber("channel_id", mcp.Description("Collaboration space ID to associate the incident with.")),
			mcp.WithString("description", mcp.Description("Incident description. Max 6144 characters."), mcp.MaxLength(6144)),
			mcp.WithString("assigned_to", mcp.Description("Comma-separated person IDs to assign as responders. Use query_members to find IDs.")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			title, err := RequiredParam[string](request, "title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			severity, err := RequiredParam[string](request, "severity")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			channelID, _ := OptionalInt(request, "channel_id")
			description, _ := OptionalParam[string](request, "description")
			assignedTo, _ := OptionalParam[string](request, "assigned_to")

			requestBody := map[string]interface{}{
				"title":             title,
				"incident_severity": severity,
			}
			if channelID > 0 {
				requestBody["channel_id"] = channelID
			}
			if description != "" {
				requestBody["description"] = description
			}
			if assignedTo != "" {
				personIDs := parseCommaSeparatedInts(assignedTo)
				if len(personIDs) > 0 {
					requestBody["assigned_to"] = map[string]interface{}{
						"type":       "assign",
						"person_ids": personIDs,
					}
				}
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/create", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to create incident: %w", err)
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

const updateIncidentDescription = `Update incident title, description, severity, or custom fields. Only provided fields are updated.`

// UpdateIncident creates a tool to update an incident
func UpdateIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("update_incident",
			mcp.WithDescription(t("TOOL_UPDATE_INCIDENT_DESCRIPTION", updateIncidentDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_UPDATE_INCIDENT_USER_TITLE", "Update incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to update.")),
			mcp.WithString("title", mcp.Description("New incident title. Length: 3-200 characters."), mcp.MinLength(3), mcp.MaxLength(200)),
			mcp.WithString("description", mcp.Description("New incident description. Max 6144 characters."), mcp.MaxLength(6144)),
			mcp.WithString("severity", mcp.Description("New severity level."), mcp.Enum("Info", "Warning", "Critical")),
			mcp.WithString("custom_fields", mcp.Description("JSON object of custom field updates. Format: {\"field_name\": \"value\"}. Field names must match ^[a-z][a-z0-9_]*$. Use query_fields to discover available fields.")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			title, _ := OptionalParam[string](request, "title")
			description, _ := OptionalParam[string](request, "description")
			severity, _ := OptionalParam[string](request, "severity")
			customFieldsStr, _ := OptionalParam[string](request, "custom_fields")

			updatedFields := make([]string, 0)

			// Update title
			if title != "" {
				if err := client.updateIncidentField(ctx, incidentID, "/incident/title/reset", "title", title); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to update title: %v", err)), nil
				}
				updatedFields = append(updatedFields, "title")
			}

			// Update description
			if description != "" {
				if err := client.updateIncidentField(ctx, incidentID, "/incident/description/reset", "description", description); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to update description: %v", err)), nil
				}
				updatedFields = append(updatedFields, "description")
			}

			// Update severity
			if severity != "" {
				if err := client.updateIncidentField(ctx, incidentID, "/incident/severity/reset", "incident_severity", severity); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to update severity: %v", err)), nil
				}
				updatedFields = append(updatedFields, "severity")
			}

			// Update custom fields
			if customFieldsStr != "" {
				customFieldsStr = strings.TrimSpace(customFieldsStr)
				if customFieldsStr == "" {
					return mcp.NewToolResultError("custom_fields must be a valid JSON object, not empty"), nil
				}

				var customFields map[string]any
				if err := json.Unmarshal([]byte(customFieldsStr), &customFields); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("custom_fields must be a valid JSON object: %v", err)), nil
				}

				if len(customFields) == 0 {
					return mcp.NewToolResultError("custom_fields must contain at least one field"), nil
				}

				// Validate field names (alphanumeric and underscore only)
				for fieldName := range customFields {
					if fieldName == "" {
						return mcp.NewToolResultError("custom_fields contains an empty field name"), nil
					}
					for _, c := range fieldName {
						isValid := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
						if !isValid {
							return mcp.NewToolResultError(fmt.Sprintf("custom field name '%s' contains invalid characters (only alphanumeric and underscore allowed)", fieldName)), nil
						}
					}
				}

				for fieldName, fieldValue := range customFields {
					if err := client.updateCustomField(ctx, incidentID, fieldName, fieldValue); err != nil {
						return mcp.NewToolResultError(fmt.Sprintf("Unable to update custom field '%s': %v", fieldName, err)), nil
					}
					updatedFields = append(updatedFields, fieldName)
				}
			}

			if len(updatedFields) == 0 {
				return mcp.NewToolResultError("No fields specified to update"), nil
			}

			return MarshalResult(map[string]any{
				"status":         "success",
				"message":        "Incident updated successfully",
				"updated_fields": updatedFields,
			}), nil
		}
}

// updateIncidentField is a helper to update a single incident field
func (c *Client) updateIncidentField(ctx context.Context, incidentID, endpoint, fieldName, fieldValue string) error {
	requestBody := map[string]interface{}{
		"incident_id": incidentID,
		fieldName:     fieldValue,
	}

	resp, err := c.makeRequest(ctx, "POST", endpoint, requestBody)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	var result FlashdutyResponse
	if err := parseResponse(resp, &result); err != nil {
		return err
	}
	if result.Error != nil {
		return fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}
	return nil
}

// updateCustomField is a helper to update a custom field
func (c *Client) updateCustomField(ctx context.Context, incidentID, fieldName string, fieldValue any) error {
	requestBody := map[string]interface{}{
		"incident_id": incidentID,
		"field_name":  fieldName,
		"field_value": fieldValue,
	}

	resp, err := c.makeRequest(ctx, "POST", "/incident/field/reset", requestBody)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return handleAPIError(resp)
	}

	var result FlashdutyResponse
	if err := parseResponse(resp, &result); err != nil {
		return err
	}
	if result.Error != nil {
		return fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}
	return nil
}

const ackIncidentDescription = `Acknowledge incidents. Moves status from Triggered to Processing.`

// AckIncident creates a tool to acknowledge incidents
func AckIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("ack_incident",
			mcp.WithDescription(t("TOOL_ACK_INCIDENT_DESCRIPTION", ackIncidentDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_ACK_INCIDENT_USER_TITLE", "Acknowledge incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_ids", mcp.Required(), mcp.Description("Comma-separated incident IDs to acknowledge. Records acknowledging user in timeline.")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			incidentIdsStr, err := RequiredParam[string](request, "incident_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			incidentIDs := parseCommaSeparatedStrings(incidentIdsStr)
			if len(incidentIDs) == 0 {
				return mcp.NewToolResultError("incident_ids must contain at least one valid ID"), nil
			}

			requestBody := map[string]interface{}{
				"incident_ids": incidentIDs,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/ack", requestBody)
			if err != nil {
				return nil, fmt.Errorf("unable to acknowledge incidents: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(handleAPIError(resp).Error()), nil
			}

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalResult(map[string]string{
				"status":  "success",
				"message": fmt.Sprintf("%d incident(s) acknowledged", len(incidentIDs)),
			}), nil
		}
}

const closeIncidentDescription = `Close (resolve) incidents. Moves status to Closed.`

// CloseIncident creates a tool to close incidents
func CloseIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("close_incident",
			mcp.WithDescription(t("TOOL_CLOSE_INCIDENT_DESCRIPTION", closeIncidentDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CLOSE_INCIDENT_USER_TITLE", "Close incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_ids", mcp.Required(), mcp.Description("Comma-separated incident IDs to close/resolve. Records closing user in timeline.")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			incidentIdsStr, err := RequiredParam[string](request, "incident_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			incidentIDs := parseCommaSeparatedStrings(incidentIdsStr)
			if len(incidentIDs) == 0 {
				return mcp.NewToolResultError("incident_ids must contain at least one valid ID"), nil
			}

			requestBody := map[string]interface{}{
				"incident_ids": incidentIDs,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/resolve", requestBody)
			if err != nil {
				return nil, fmt.Errorf("unable to close incidents: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(handleAPIError(resp).Error()), nil
			}

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalResult(map[string]string{
				"status":  "success",
				"message": fmt.Sprintf("%d incident(s) closed", len(incidentIDs)),
			}), nil
		}
}

const listSimilarIncidentsDescription = `Find similar historical incidents. Useful for reviewing past resolutions and identifying recurring issues.`

// ListSimilarIncidents creates a tool to find similar incidents
func ListSimilarIncidents(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_similar_incidents",
			mcp.WithDescription(t("TOOL_LIST_SIMILAR_INCIDENTS_DESCRIPTION", listSimilarIncidentsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_SIMILAR_INCIDENTS_USER_TITLE", "List similar incidents"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("Reference incident ID to find similar historical incidents for.")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of similar incidents to return."), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			limit, _ := OptionalInt(request, "limit")
			if limit <= 0 {
				limit = defaultQueryLimit
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"p":           1,
				"limit":       limit,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/past/list", requestBody)
			if err != nil {
				return nil, fmt.Errorf("unable to find similar incidents: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(handleAPIError(resp).Error()), nil
			}

			var result struct {
				Error *DutyError `json:"error,omitempty"`
				Data  *struct {
					Items []RawIncident `json:"items"`
					Total int           `json:"total"`
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
					"incidents": []EnrichedIncident{},
					"total":     0,
				}), nil
			}

			// Enrich similar incidents
			enrichedIncidents, err := client.enrichIncidents(ctx, result.Data.Items)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to load additional incident details: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"incidents": enrichedIncidents,
				"total":     result.Data.Total,
			}), nil
		}
}

// Helper functions

func parseCommaSeparatedStrings(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseCommaSeparatedInts(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err == nil {
			result = append(result, id)
		}
	}
	return result
}
