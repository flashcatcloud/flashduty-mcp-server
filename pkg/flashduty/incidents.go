package flashduty

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

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

			// Build SDK input
			input := &sdk.ListIncidentsInput{
				Progress:      progress,
				Severity:      severity,
				ChannelID:     int64(channelID),
				StartTime:     int64(startTime),
				EndTime:       int64(endTime),
				Title:         title,
				Limit:         limit,
				IncludeAlerts: includeAlerts,
			}

			if incidentIdsStr != "" {
				incidentIDs := parseCommaSeparatedStrings(incidentIdsStr)
				if len(incidentIDs) == 0 {
					return mcp.NewToolResultError("incident_ids must contain at least one valid ID when specified"), nil
				}
				input.IncidentIDs = incidentIDs
			} else if startTime == 0 || endTime == 0 {
				return mcp.NewToolResultError("Both start_time and end_time are required for time-based queries"), nil
			}

			output, err := client.ListIncidents(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve incidents: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"incidents": output.Incidents,
				"total":     output.Total,
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

			results, err := client.GetIncidentTimelines(ctx, incidentIDs)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve timeline: %v", err)), nil
			}

			// Build response matching expected JSON shape
			response := make([]map[string]any, 0, len(results))
			for _, r := range results {
				response = append(response, map[string]any{
					"incident_id": r.IncidentID,
					"timeline":    r.Timeline,
					"total":       r.Total,
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

			results, err := client.ListIncidentAlerts(ctx, incidentIDs, limit)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve alerts: %v", err)), nil
			}

			// Build response matching expected JSON shape
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
			assignedToStr, _ := OptionalParam[string](request, "assigned_to")

			input := &sdk.CreateIncidentInput{
				Title:       title,
				Severity:    severity,
				ChannelID:   int64(channelID),
				Description: description,
			}

			if assignedToStr != "" {
				input.AssignedTo = parseCommaSeparatedInts(assignedToStr)
			}

			result, err := client.CreateIncident(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to create incident: %v", err)), nil
			}

			return MarshalResult(result), nil
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

			input := &sdk.UpdateIncidentInput{
				IncidentID:  incidentID,
				Title:       title,
				Description: description,
				Severity:    severity,
			}

			// Parse custom fields JSON if provided
			if customFieldsStr != "" {
				var customFields map[string]any
				if err := json.Unmarshal([]byte(customFieldsStr), &customFields); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("custom_fields must be a valid JSON object: %v", err)), nil
				}
				if len(customFields) == 0 {
					return mcp.NewToolResultError("custom_fields must contain at least one field"), nil
				}
				input.CustomFields = customFields
			}

			updatedFields, err := client.UpdateIncident(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to update incident: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"status":         "success",
				"message":        "Incident updated successfully",
				"updated_fields": updatedFields,
			}), nil
		}
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

			if err := client.AckIncidents(ctx, incidentIDs); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to acknowledge incidents: %v", err)), nil
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

			if err := client.CloseIncidents(ctx, incidentIDs); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to close incidents: %v", err)), nil
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

			output, err := client.ListSimilarIncidents(ctx, incidentID, limit)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to find similar incidents: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"incidents": output.Incidents,
				"total":     output.Total,
			}), nil
		}
}
