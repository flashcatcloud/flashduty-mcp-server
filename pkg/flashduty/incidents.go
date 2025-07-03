package flashduty

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// IncidentInfos creates a tool to get incident information by incident IDs
func IncidentInfos(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_incidents_infos",
			mcp.WithDescription(t("TOOL_FLASHDUTY_INCIDENTS_INFOS_DESCRIPTION", "Get incident information by incident IDs")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_INCIDENTS_INFOS_USER_TITLE", "Get incident infos"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("incident_ids",
				mcp.Required(),
				mcp.Description("Comma-separated list of incident IDs to get information for. Example: 'id1,id2,id3'"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract incident_ids from request
			incidentIdsStr, err := RequiredParam[string](request, "incident_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse comma-separated string to string slice
			var incidentIds []string
			if incidentIdsStr != "" {
				parts := strings.Split(incidentIdsStr, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" {
						incidentIds = append(incidentIds, part)
					}
				}
			}

			if len(incidentIds) == 0 {
				return mcp.NewToolResultError("incident_ids cannot be empty"), nil
			}

			// Get Flashduty client
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			// Build request body according to API specification
			requestBody := map[string]interface{}{
				"incident_ids": incidentIds,
			}

			// Make API request to /incident/list-by-ids endpoint
			resp, err := client.makeRequest(ctx, "POST", "/incident/list-by-ids", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to get incident infos: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("API request failed with status %d", resp.StatusCode)), nil
			}

			// Parse response
			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			// Check for API error
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(result.Data), nil
		}
}

// ListIncidents creates a tool to list incidents with comprehensive filters
func ListIncidents(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_list_incidents",
			mcp.WithDescription(t("TOOL_FLASHDUTY_LIST_INCIDENTS_DESCRIPTION", "List incidents with comprehensive filters")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_LIST_INCIDENTS_USER_TITLE", "List incidents"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithNumber("p", mcp.Description("Page number (default: 1)")),
			mcp.WithNumber("limit", mcp.Description("Items per page (default: 20)")),
			mcp.WithString("title", mcp.Description("Search by incident title")),
			mcp.WithNumber("team_id", mcp.Description("Filter by team ID")),
			mcp.WithString("progress", mcp.Description("Filter by progress status (Triggered, Processing, Closed)")),
			mcp.WithNumber("start_time", mcp.Required(), mcp.Description("Start time (Unix timestamp, required)")),
			mcp.WithNumber("end_time", mcp.Required(), mcp.Description("End time (Unix timestamp, required)")),
			mcp.WithString("incident_severity", mcp.Description("Filter by severity (Info, Warning, Critical)")),
			mcp.WithNumber("channel_id", mcp.Description("Filter by channel ID")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Get pagination parameters
			p, err := OptionalInt(request, "p")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if p == 0 {
				p = 1
			}

			limit, err := OptionalInt(request, "limit")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if limit == 0 {
				limit = 20
			}

			// Get filter parameters
			title, _ := OptionalParam[string](request, "title")
			teamID, _ := OptionalInt(request, "team_id")
			progress, _ := OptionalParam[string](request, "progress")
			severity, _ := OptionalParam[string](request, "incident_severity")
			channelID, _ := OptionalInt(request, "channel_id")

			// Get required time parameters
			startTime, err := RequiredInt(request, "start_time")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			endTime, err := RequiredInt(request, "end_time")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			// Build request body with required time parameters
			requestBody := map[string]interface{}{
				"p":          p,
				"limit":      limit,
				"start_time": startTime,
				"end_time":   endTime,
			}
			if title != "" {
				requestBody["title"] = title
			}
			if teamID > 0 {
				requestBody["team_id"] = teamID
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

			resp, err := client.makeRequest(ctx, "POST", "/incident/list", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to list incidents: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(result.Data), nil
		}
}

// CreateIncident creates a tool to create a new incident
func CreateIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_create_incident",
			mcp.WithDescription(t("TOOL_FLASHDUTY_CREATE_INCIDENT_DESCRIPTION", "Create a new incident")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_CREATE_INCIDENT_USER_TITLE", "Create incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("title", mcp.Required(), mcp.Description("The title of the incident")),
			mcp.WithString("incident_severity", mcp.Required(), mcp.Description("The severity level (Info, Warning, Critical)")),
			mcp.WithNumber("channel_id", mcp.Description("The ID of the collaboration space (optional)")),
			mcp.WithString("description", mcp.Description("The description of the incident")),
			mcp.WithString("impact", mcp.Description("The impact of the incident")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			title, err := RequiredParam[string](request, "title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			severity, err := RequiredParam[string](request, "incident_severity")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			channelID, _ := OptionalInt(request, "channel_id")
			description, _ := OptionalParam[string](request, "description")
			impact, _ := OptionalParam[string](request, "impact")

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

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
			if impact != "" {
				requestBody["impact"] = impact
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/create", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to create incident: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(result.Data), nil
		}
}

// AckIncident creates a tool to acknowledge incidents
func AckIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_ack_incident",
			mcp.WithDescription(t("TOOL_FLASHDUTY_ACK_INCIDENT_DESCRIPTION", "Acknowledge incidents")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_ACK_INCIDENT_USER_TITLE", "Acknowledge incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_ids",
				mcp.Required(),
				mcp.Description("Comma-separated list of incident IDs to acknowledge. Example: 'id1,id2,id3'"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentIdsStr, err := RequiredParam[string](request, "incident_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse comma-separated string to string slice
			var incidentIds []string
			if incidentIdsStr != "" {
				parts := strings.Split(incidentIdsStr, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" {
						incidentIds = append(incidentIds, part)
					}
				}
			}

			if len(incidentIds) == 0 {
				return mcp.NewToolResultError("incident_ids cannot be empty"), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_ids": incidentIds,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/ack", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to acknowledge incident: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incidents acknowledged successfully"}), nil
		}
}

// ResolveIncident creates a tool to resolve incidents
func ResolveIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_resolve_incident",
			mcp.WithDescription(t("TOOL_FLASHDUTY_RESOLVE_INCIDENT_DESCRIPTION", "Resolve incidents")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_RESOLVE_INCIDENT_USER_TITLE", "Resolve incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_ids",
				mcp.Required(),
				mcp.Description("Comma-separated list of incident IDs to resolve. Example: 'id1,id2,id3'"),
			),
			mcp.WithString("root_cause", mcp.Description("Root cause of the incidents")),
			mcp.WithString("resolution", mcp.Description("Resolution description")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentIdsStr, err := RequiredParam[string](request, "incident_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			rootCause, _ := OptionalParam[string](request, "root_cause")
			resolution, _ := OptionalParam[string](request, "resolution")

			// Parse comma-separated string to string slice
			var incidentIds []string
			if incidentIdsStr != "" {
				parts := strings.Split(incidentIdsStr, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" {
						incidentIds = append(incidentIds, part)
					}
				}
			}

			if len(incidentIds) == 0 {
				return mcp.NewToolResultError("incident_ids cannot be empty"), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_ids": incidentIds,
			}
			if rootCause != "" {
				requestBody["root_cause"] = rootCause
			}
			if resolution != "" {
				requestBody["resolution"] = resolution
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/resolve", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve incident: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incidents resolved successfully"}), nil
		}
}

// ListPastIncidents creates a tool to list similar historical incidents
func ListPastIncidents(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_list_past_incidents",
			mcp.WithDescription(t("TOOL_FLASHDUTY_LIST_PAST_INCIDENTS_DESCRIPTION", "List similar historical incidents")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_LIST_PAST_INCIDENTS_USER_TITLE", "List past incidents"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to find similar historical incidents for")),
			mcp.WithNumber("p", mcp.Description("Page number (default: 1)")),
			mcp.WithNumber("limit", mcp.Description("Items per page (default: 20)")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			p, _ := OptionalInt(request, "p")
			if p == 0 {
				p = 1
			}

			limit, _ := OptionalInt(request, "limit")
			if limit == 0 {
				limit = 20
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"p":           p,
				"limit":       limit,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/past/list", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to list past incidents: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(result.Data), nil
		}
}

// GetIncidentTimeline creates a tool to get incident timeline and feed
func GetIncidentTimeline(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_get_incident_timeline",
			mcp.WithDescription(t("TOOL_FLASHDUTY_GET_INCIDENT_TIMELINE_DESCRIPTION", "Get incident timeline and feed")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_GET_INCIDENT_TIMELINE_USER_TITLE", "Get incident timeline"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to get timeline for")),
			mcp.WithString("types", mcp.Description("Comma-separated list of operation record types to filter (e.g., 'i_comm,i_notify,i_ack')")),
			mcp.WithNumber("p", mcp.Description("Page number (default: 1)")),
			mcp.WithNumber("limit", mcp.Description("Items per page (default: 20)")),
			mcp.WithBoolean("asc", mcp.Description("Whether to sort in ascending order (default: true)")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			typesStr, _ := OptionalParam[string](request, "types")
			p, _ := OptionalInt(request, "p")
			if p == 0 {
				p = 1
			}
			limit, _ := OptionalInt(request, "limit")
			if limit == 0 {
				limit = 20
			}
			asc, _ := OptionalParam[bool](request, "asc")

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"p":           p,
				"limit":       limit,
				"asc":         asc,
			}

			if typesStr != "" {
				parts := strings.Split(typesStr, ",")
				var types []string
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" {
						types = append(types, part)
					}
				}
				if len(types) > 0 {
					requestBody["types"] = types
				}
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/feed", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to get incident timeline: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(result.Data), nil
		}
}

// GetIncidentAlerts creates a tool to get alerts associated with incidents
func GetIncidentAlerts(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_get_incident_alerts",
			mcp.WithDescription(t("TOOL_FLASHDUTY_GET_INCIDENT_ALERTS_DESCRIPTION", "Get alerts associated with incidents")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_GET_INCIDENT_ALERTS_USER_TITLE", "Get incident alerts"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to get alerts for")),
			mcp.WithNumber("p", mcp.Description("Page number (default: 1)")),
			mcp.WithNumber("limit", mcp.Description("Items per page (default: 20)")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			p, _ := OptionalInt(request, "p")
			if p == 0 {
				p = 1
			}
			limit, _ := OptionalInt(request, "limit")
			if limit == 0 {
				limit = 20
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"p":           p,
				"limit":       limit,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/alert/list", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to get incident alerts: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(result.Data), nil
		}
}

// AssignIncident creates a tool to assign incidents to people or escalation rules
func AssignIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_assign_incident",
			mcp.WithDescription(t("TOOL_FLASHDUTY_ASSIGN_INCIDENT_DESCRIPTION", "Assign incidents to people or escalation rules")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_ASSIGN_INCIDENT_USER_TITLE", "Assign incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to assign")),
			mcp.WithString("person_ids", mcp.Description("Comma-separated list of person IDs to assign to (use this OR escalate_rule_id)")),
			mcp.WithString("escalate_rule_id", mcp.Description("Escalation rule ID to assign to (use this OR person_ids)")),
			mcp.WithString("escalate_rule_name", mcp.Description("Escalation rule name")),
			mcp.WithNumber("layer_idx", mcp.Description("Layer index when assigning to an escalation rule")),
			mcp.WithString("type", mcp.Description("Assignment type: assign, reassign, escalate, reopen (default: assign)")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			personIdsStr, _ := OptionalParam[string](request, "person_ids")
			escalateRuleID, _ := OptionalParam[string](request, "escalate_rule_id")
			escalateRuleName, _ := OptionalParam[string](request, "escalate_rule_name")
			layerIdx, _ := OptionalInt(request, "layer_idx")
			assignType, _ := OptionalParam[string](request, "type")

			if assignType == "" {
				assignType = "assign"
			}

			if personIdsStr == "" && escalateRuleID == "" {
				return mcp.NewToolResultError("Either person_ids or escalate_rule_id must be provided"), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			assignedTo := map[string]interface{}{
				"type": assignType,
			}

			if personIdsStr != "" {
				parts := strings.Split(personIdsStr, ",")
				var personIds []int
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" {
						var id int
						if _, parseErr := fmt.Sscanf(part, "%d", &id); parseErr == nil {
							personIds = append(personIds, id)
						}
					}
				}
				if len(personIds) > 0 {
					assignedTo["person_ids"] = personIds
				}
			}

			if escalateRuleID != "" {
				assignedTo["escalate_rule_id"] = escalateRuleID
				if escalateRuleName != "" {
					assignedTo["escalate_rule_name"] = escalateRuleName
				}
				if layerIdx > 0 {
					assignedTo["layer_idx"] = layerIdx
				}
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"assigned_to": assignedTo,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/assign", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to assign incident: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incident assigned successfully"}), nil
		}
}

// AddResponder creates a tool to add responders to incidents
func AddResponder(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_add_responder",
			mcp.WithDescription(t("TOOL_FLASHDUTY_ADD_RESPONDER_DESCRIPTION", "Add responders to incidents")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_ADD_RESPONDER_USER_TITLE", "Add responder"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to add responders to")),
			mcp.WithString("person_ids", mcp.Required(), mcp.Description("Comma-separated list of person IDs to add as responders")),
			mcp.WithBoolean("follow_preference", mcp.Description("Whether to follow personal notification preferences (default: true)")),
			mcp.WithString("personal_channels", mcp.Description("Comma-separated list of personal notification channels (email, sms, voice)")),
			mcp.WithString("template_id", mcp.Description("Notification template ID")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			personIdsStr, err := RequiredParam[string](request, "person_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			followPreference, _ := OptionalParam[bool](request, "follow_preference")
			personalChannelsStr, _ := OptionalParam[string](request, "personal_channels")
			templateID, _ := OptionalParam[string](request, "template_id")

			// Parse person IDs
			parts := strings.Split(personIdsStr, ",")
			var personIds []int
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					var id int
					if _, parseErr := fmt.Sscanf(part, "%d", &id); parseErr == nil {
						personIds = append(personIds, id)
					}
				}
			}

			if len(personIds) == 0 {
				return mcp.NewToolResultError("person_ids cannot be empty"), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"person_ids":  personIds,
			}

			// Add notification settings if provided
			if followPreference || personalChannelsStr != "" || templateID != "" {
				notify := map[string]interface{}{}
				if followPreference {
					notify["follow_preference"] = true
				}
				if personalChannelsStr != "" {
					channels := strings.Split(personalChannelsStr, ",")
					var personalChannels []string
					for _, ch := range channels {
						ch = strings.TrimSpace(ch)
						if ch != "" {
							personalChannels = append(personalChannels, ch)
						}
					}
					if len(personalChannels) > 0 {
						notify["personal_channels"] = personalChannels
					}
				}
				if templateID != "" {
					notify["template_id"] = templateID
				}
				if len(notify) > 0 {
					requestBody["notify"] = notify
				}
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/responder/add", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to add responder: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Responder added successfully"}), nil
		}
}

// SnoozeIncident creates a tool to snooze incidents for a period
func SnoozeIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_snooze_incident",
			mcp.WithDescription(t("TOOL_FLASHDUTY_SNOOZE_INCIDENT_DESCRIPTION", "Snooze incidents for a period")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_SNOOZE_INCIDENT_USER_TITLE", "Snooze incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_ids", mcp.Required(), mcp.Description("Comma-separated list of incident IDs to snooze")),
			mcp.WithNumber("minutes", mcp.Required(), mcp.Description("Number of minutes to snooze (1-1440)")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentIdsStr, err := RequiredParam[string](request, "incident_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			minutes, err := RequiredParam[float64](request, "minutes")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if minutes < 1 || minutes > 1440 {
				return mcp.NewToolResultError("minutes must be between 1 and 1440"), nil
			}

			// Parse incident IDs
			parts := strings.Split(incidentIdsStr, ",")
			var incidentIds []string
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					incidentIds = append(incidentIds, part)
				}
			}

			if len(incidentIds) == 0 {
				return mcp.NewToolResultError("incident_ids cannot be empty"), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_ids": incidentIds,
				"minutes":      int(minutes),
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/snooze", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to snooze incident: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incidents snoozed successfully"}), nil
		}
}

// MergeIncident creates a tool to merge multiple incidents into one
func MergeIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_merge_incident",
			mcp.WithDescription(t("TOOL_FLASHDUTY_MERGE_INCIDENT_DESCRIPTION", "Merge multiple incidents into one")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_MERGE_INCIDENT_USER_TITLE", "Merge incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("source_incident_ids", mcp.Required(), mcp.Description("Comma-separated list of source incident IDs to merge")),
			mcp.WithString("target_incident_id", mcp.Required(), mcp.Description("Target incident ID to merge into")),
			mcp.WithString("title", mcp.Description("New title for the merged incident")),
			mcp.WithString("comment", mcp.Description("Comment for the merge operation")),
			mcp.WithBoolean("remove_source_incidents", mcp.Required(), mcp.Description("Whether to remove source incidents after merge")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sourceIncidentIdsStr, err := RequiredParam[string](request, "source_incident_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			targetIncidentID, err := RequiredParam[string](request, "target_incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			title, _ := OptionalParam[string](request, "title")
			comment, _ := OptionalParam[string](request, "comment")
			removeSource, err := RequiredParam[bool](request, "remove_source_incidents")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse source incident IDs
			parts := strings.Split(sourceIncidentIdsStr, ",")
			var sourceIncidentIds []string
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					sourceIncidentIds = append(sourceIncidentIds, part)
				}
			}

			if len(sourceIncidentIds) == 0 {
				return mcp.NewToolResultError("source_incident_ids cannot be empty"), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"source_incident_ids":     sourceIncidentIds,
				"target_incident_id":      targetIncidentID,
				"remove_source_incidents": removeSource,
			}

			if title != "" {
				requestBody["title"] = title
			}
			if comment != "" {
				requestBody["comment"] = comment
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/merge", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to merge incident: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incidents merged successfully"}), nil
		}
}

// CommentIncident creates a tool to add comments to incidents
func CommentIncident(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_comment_incident",
			mcp.WithDescription(t("TOOL_FLASHDUTY_COMMENT_INCIDENT_DESCRIPTION", "Add comments to incidents")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_COMMENT_INCIDENT_USER_TITLE", "Comment incident"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_ids", mcp.Required(), mcp.Description("Comma-separated list of incident IDs to comment on")),
			mcp.WithString("comment", mcp.Required(), mcp.Description("The comment to add to the incidents")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentIdsStr, err := RequiredParam[string](request, "incident_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			comment, err := RequiredParam[string](request, "comment")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse incident IDs
			parts := strings.Split(incidentIdsStr, ",")
			var incidentIds []string
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					incidentIds = append(incidentIds, part)
				}
			}

			if len(incidentIds) == 0 {
				return mcp.NewToolResultError("incident_ids cannot be empty"), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_ids": incidentIds,
				"comment":      comment,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/comment", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to comment incident: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Comment added successfully"}), nil
		}
}

// UpdateIncidentTitle creates a tool to update incident title
func UpdateIncidentTitle(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_update_incident_title",
			mcp.WithDescription(t("TOOL_FLASHDUTY_UPDATE_INCIDENT_TITLE_DESCRIPTION", "Update incident title")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_UPDATE_INCIDENT_TITLE_USER_TITLE", "Update incident title"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to update")),
			mcp.WithString("title", mcp.Required(), mcp.Description("The new title for the incident")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			title, err := RequiredParam[string](request, "title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"title":       title,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/title/reset", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to update incident title: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incident title updated successfully"}), nil
		}
}

// UpdateIncidentDescription creates a tool to update incident description
func UpdateIncidentDescription(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_update_incident_description",
			mcp.WithDescription(t("TOOL_FLASHDUTY_UPDATE_INCIDENT_DESCRIPTION_DESCRIPTION", "Update incident description")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_UPDATE_INCIDENT_DESCRIPTION_USER_TITLE", "Update incident description"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to update")),
			mcp.WithString("description", mcp.Required(), mcp.Description("The new description for the incident")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			description, err := RequiredParam[string](request, "description")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"description": description,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/description/reset", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to update incident description: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incident description updated successfully"}), nil
		}
}

// UpdateIncidentImpact creates a tool to update incident impact
func UpdateIncidentImpact(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_update_incident_impact",
			mcp.WithDescription(t("TOOL_FLASHDUTY_UPDATE_INCIDENT_IMPACT_DESCRIPTION", "Update incident impact")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_UPDATE_INCIDENT_IMPACT_USER_TITLE", "Update incident impact"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to update")),
			mcp.WithString("impact", mcp.Required(), mcp.Description("The new impact description for the incident")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			impact, err := RequiredParam[string](request, "impact")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"impact":      impact,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/impact/reset", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to update incident impact: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incident impact updated successfully"}), nil
		}
}

// UpdateIncidentRootCause creates a tool to update incident root cause
func UpdateIncidentRootCause(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_update_incident_root_cause",
			mcp.WithDescription(t("TOOL_FLASHDUTY_UPDATE_INCIDENT_ROOT_CAUSE_DESCRIPTION", "Update incident root cause")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_UPDATE_INCIDENT_ROOT_CAUSE_USER_TITLE", "Update incident root cause"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to update")),
			mcp.WithString("root_cause", mcp.Required(), mcp.Description("The new root cause description for the incident")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			rootCause, err := RequiredParam[string](request, "root_cause")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"root_cause":  rootCause,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/root-cause/reset", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to update incident root cause: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incident root cause updated successfully"}), nil
		}
}

// UpdateIncidentResolution creates a tool to update incident resolution
func UpdateIncidentResolution(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_update_incident_resolution",
			mcp.WithDescription(t("TOOL_FLASHDUTY_UPDATE_INCIDENT_RESOLUTION_DESCRIPTION", "Update incident resolution")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_UPDATE_INCIDENT_RESOLUTION_USER_TITLE", "Update incident resolution"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to update")),
			mcp.WithString("resolution", mcp.Required(), mcp.Description("The new resolution description for the incident")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			resolution, err := RequiredParam[string](request, "resolution")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"resolution":  resolution,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/resolution/reset", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to update incident resolution: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incident resolution updated successfully"}), nil
		}
}

// UpdateIncidentSeverity creates a tool to update incident severity
func UpdateIncidentSeverity(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_update_incident_severity",
			mcp.WithDescription(t("TOOL_FLASHDUTY_UPDATE_INCIDENT_SEVERITY_DESCRIPTION", "Update incident severity")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_UPDATE_INCIDENT_SEVERITY_USER_TITLE", "Update incident severity"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to update")),
			mcp.WithString("severity", mcp.Required(), mcp.Description("The new severity level for the incident (e.g., Info, Warning, Critical)")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			severity, err := RequiredParam[string](request, "severity")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"severity":    severity,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/severity/reset", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to update incident severity: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incident severity updated successfully"}), nil
		}
}

// UpdateIncidentFields creates a tool to update custom fields
func UpdateIncidentFields(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_update_incident_fields",
			mcp.WithDescription(t("TOOL_FLASHDUTY_UPDATE_INCIDENT_FIELDS_DESCRIPTION", "Update custom fields")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_UPDATE_INCIDENT_FIELDS_USER_TITLE", "Update incident fields"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("incident_id", mcp.Required(), mcp.Description("The incident ID to update")),
			mcp.WithString("field_name", mcp.Required(), mcp.Description("The name of the custom field to update")),
			mcp.WithString("field_value", mcp.Required(), mcp.Description("The new value for the custom field")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			incidentID, err := RequiredParam[string](request, "incident_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			fieldName, err := RequiredParam[string](request, "field_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			fieldValue, err := RequiredParam[string](request, "field_value")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			requestBody := map[string]interface{}{
				"incident_id": incidentID,
				"field_name":  fieldName,
				"field_value": fieldValue,
			}

			resp, err := client.makeRequest(ctx, "POST", "/incident/field/reset", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to update incident fields: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			var result FlashdutyResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			return MarshalledTextResult(map[string]string{"status": "success", "message": "Incident fields updated successfully"}), nil
		}
}
