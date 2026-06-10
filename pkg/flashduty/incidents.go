package flashduty

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	flashduty "github.com/flashcatcloud/go-flashduty"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/internal/timeutil"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const defaultQueryLimit = 20

const queryIncidentsDescription = `Query incidents by IDs, short ids (nums), time range, status, severity, channel, or free-text query. Returns the incident list with an alerts_total count per incident; for the actual alert objects of one or more incidents, call query_incident_alerts(incident_ids=...).`

// incidentSinceDescription extends the shared SinceDescription with
// query_incidents' optional-window behavior: omit BOTH bounds to query
// "current" / open incidents and the tool defaults to the last 30 days.
// (query_changes keeps the shared wording, where the window is mandatory.)
// Composed from the shared constant so the date-format guidance can't drift.
const incidentSinceDescription = SinceDescription +
	" You may omit BOTH since and until to query current/open incidents; the tool then defaults to the last 30 days."

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
			mcp.WithString("channel_ids", mcp.Description("Comma-separated collaboration space IDs to filter by. Backend expects an array — singular channel_id is silently ignored.")),
			WithSince(mcp.Description(incidentSinceDescription)),
			WithUntil(),
			mcp.WithString("query", mcp.Description("Free-text search across title, labels, and content (Doris full-text). A 24-char hex string is resolved as an incident ID; a 6-char string is resolved as an incident num. Prefer this over picking exact filter values when the user gives a fuzzy keyword."), mcp.MaxLength(200)),
			mcp.WithString("nums", mcp.Description("Comma-separated short incident ids (num — the 6-char id shown in the UI, e.g. 311510). Matched within the since/until window; the backend caps the list span at ~30 days, so incidents older than that must be looked up by their full incident_id.")),
			mcp.WithNumber("limit", mcp.Description(LimitDescription), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			args := request.GetArguments()
			incidentIdsStr, _ := OptionalParam[string](request, "incident_ids")
			progress, _ := OptionalParam[string](request, "progress")
			severity, _ := OptionalParam[string](request, "severity")
			channelIdsStr, _ := OptionalParam[string](request, "channel_ids")
			query, _ := OptionalParam[string](request, "query")
			nums, _ := OptionalParam[string](request, "nums")
			limit, _ := OptionalInt(request, "limit")

			startTime, err := timeutil.ParseAny(args["since"])
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid since: %v", err)), nil
			}
			endTime, err := parseUntilArg(args["until"])
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid until: %v", err)), nil
			}

			// "current open incidents" with no window is the common ask, so when
			// BOTH bounds are omitted default to the last 30 days (under the
			// 31-day backend cap) instead of rejecting the call. A bare `until`
			// is fine (it documents a "now" default, already applied by
			// parseUntilArg); a bare `since` is still a real mistake worth an
			// error. parseUntilArg has collapsed a missing `until` into "now", so
			// detect omission from the raw args, not the parsed values.
			sinceProvided := argProvided(args["since"])
			untilProvided := argProvided(args["until"])
			if !sinceProvided {
				if untilProvided {
					return mcp.NewToolResultError("`since` is required when `until` is set; omit both to default to the last 30 days, or pass a relative duration like \"30d\""), nil
				}
				endTime = time.Now().Unix()
				startTime = endTime - int64(DefaultIncidentWindow/time.Second)
			}

			if limit <= 0 {
				limit = defaultQueryLimit
			}

			// Direct ID lookup uses /incident/list-by-ids (ListByIDs), which does
			// not require a time window. Per the tool contract, when incident_ids
			// is provided every other filter is ignored.
			if incidentIdsStr != "" {
				incidentIDs := parseCommaSeparatedStrings(incidentIdsStr)
				if len(incidentIDs) == 0 {
					return mcp.NewToolResultError("incident_ids must contain at least one valid ID when specified"), nil
				}
				out, _, err := client.New.Incidents.ListByIDs(ctx, &flashduty.ListIncidentsByIDsRequest{
					IncidentIDs: incidentIDs,
				})
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve incidents: %v", err)), nil
				}
				total := int(out.Total)
				return MarshalResult(addTruncationHint(map[string]any{
					"incidents": out.Items,
					"total":     total,
				}, len(out.Items), total)), nil
			}

			// IncludeAlerts is intentionally not exposed: per-incident alert
			// payloads multiply across rows and routinely dominate the context
			// window. Callers that want alert details for specific incidents
			// should call query_incident_alerts(incident_ids=...) instead, which
			// accepts a comma-separated list and keeps the two concerns cleanly
			// separated. The alert_cnt count on each incident is enough to gauge
			// volume from this tool.
			req := &flashduty.ListIncidentsRequest{
				Progress:         progress,
				IncidentSeverity: severity,
				StartTime:        startTime,
				EndTime:          endTime,
				Query:            query,
			}
			req.Limit = limit

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

			if nums != "" {
				req.Nums = parseCommaSeparatedStrings(nums)
			}

			if err := validateTimeWindow(startTime, endTime); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			out, _, err := client.New.Incidents.List(ctx, req)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve incidents: %v", err)), nil
			}

			total := int(out.Total)
			return MarshalResult(addTruncationHint(map[string]any{
				"incidents": out.Items,
				"total":     total,
			}, len(out.Items), total)), nil
		}
}

const queryIncidentTimelineDescription = `Query timeline events for incidents. Returns events like created, assigned, acknowledged, resolved, notifications. Each event includes created_at (RFC3339) and creator_id (the actor's numeric ID, 0 = system); resolve creator_id to a display name with query_members when you need the actor's name.`

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

			// go-flashduty's Incidents.Feed returns one incident's timeline per
			// call, so fan out across the requested IDs. Match the legacy
			// asc/limit defaults the old SDK used for timeline fetches.
			response := make([]map[string]any, 0, len(incidentIDs))
			for _, id := range incidentIDs {
				feedReq := &flashduty.ListIncidentFeedRequest{IncidentID: id, Asc: true}
				feedReq.Limit = 100
				out, _, err := client.New.Incidents.Feed(ctx, feedReq)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve timeline for %s: %v", id, err)), nil
				}
				response = append(response, map[string]any{
					"incident_id": id,
					"timeline":    out.Items,
					"total":       len(out.Items),
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

			// go-flashduty's Incidents.AlertList returns one incident's alerts
			// per call, so fan out across the requested IDs.
			response := make([]map[string]any, 0, len(incidentIDs))
			for _, id := range incidentIDs {
				alertReq := &flashduty.ListIncidentAlertsRequest{IncidentID: id}
				alertReq.Limit = limit
				out, _, err := client.New.Incidents.AlertList(ctx, alertReq)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve alerts for %s: %v", id, err)), nil
				}
				response = append(response, map[string]any{
					"incident_id": id,
					"alerts":      out.Items,
					"total":       int(out.Total),
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

			req := &flashduty.CreateIncidentRequest{
				Title:            title,
				IncidentSeverity: severity,
				ChannelID:        int64(channelID),
				Description:      description,
			}

			if assignedToStr != "" {
				personIDs := parseCommaSeparatedInts(assignedToStr)
				req.AssignedTo.PersonIDs = make([]int64, len(personIDs))
				for i, id := range personIDs {
					req.AssignedTo.PersonIDs[i] = int64(id)
				}
			}

			out, _, err := client.New.Incidents.Create(ctx, req)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to create incident: %v", err)), nil
			}

			return MarshalResult(out), nil
		}
}

const updateIncidentDescription = `Update incident built-in fields (title, description, severity, impact, root_cause, resolution) and/or custom fields. Only provided fields are updated. Built-in fields are sent in a single round-trip.`

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
			mcp.WithString("description", mcp.Description("New incident description. Length: 3-6144 characters."), mcp.MinLength(3), mcp.MaxLength(6144)),
			mcp.WithString("severity", mcp.Description("New severity level."), mcp.Enum("Info", "Warning", "Critical")),
			mcp.WithString("impact", mcp.Description("Business/user impact statement. Length: 3-6144 characters."), mcp.MinLength(3), mcp.MaxLength(6144)),
			mcp.WithString("root_cause", mcp.Description("Root cause of the incident. Length: 3-6144 characters."), mcp.MinLength(3), mcp.MaxLength(6144)),
			mcp.WithString("resolution", mcp.Description("How the incident was resolved. Length: 3-6144 characters."), mcp.MinLength(3), mcp.MaxLength(6144)),
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
			impact, _ := OptionalParam[string](request, "impact")
			rootCause, _ := OptionalParam[string](request, "root_cause")
			resolution, _ := OptionalParam[string](request, "resolution")
			customFieldsStr, _ := OptionalParam[string](request, "custom_fields")

			// Parse custom fields JSON up front so a bad payload fails before any
			// write hits the backend.
			var customFields map[string]any
			if customFieldsStr != "" {
				customFieldsStr = strings.TrimSpace(customFieldsStr)
				if customFieldsStr == "" {
					return mcp.NewToolResultError("custom_fields must be a valid JSON object, not empty"), nil
				}
				if err := json.Unmarshal([]byte(customFieldsStr), &customFields); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("custom_fields must be a valid JSON object: %v", err)), nil
				}
				if len(customFields) == 0 {
					return mcp.NewToolResultError("custom_fields must contain at least one field"), nil
				}
				// Validate field names locally so users get fast, clear errors before round-tripping to the API.
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
			}

			updatedFields := make([]string, 0)

			// Built-in fields go through one /incident/reset call. The backend
			// ignores empty strings, so only set the fields the caller provided
			// and record their canonical names for the response.
			resetReq := &flashduty.UpdateIncidentFieldsRequest{IncidentID: incidentID}
			if title != "" {
				resetReq.Title = title
				updatedFields = append(updatedFields, "title")
			}
			if description != "" {
				resetReq.Description = description
				updatedFields = append(updatedFields, "description")
			}
			if severity != "" {
				resetReq.IncidentSeverity = severity
				updatedFields = append(updatedFields, "severity")
			}
			if impact != "" {
				resetReq.Impact = impact
				updatedFields = append(updatedFields, "impact")
			}
			if rootCause != "" {
				resetReq.RootCause = rootCause
				updatedFields = append(updatedFields, "root_cause")
			}
			if resolution != "" {
				resetReq.Resolution = resolution
				updatedFields = append(updatedFields, "resolution")
			}

			if len(updatedFields) > 0 {
				if _, err := client.New.Incidents.Reset(ctx, resetReq); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to update incident: %v", err)), nil
				}
			}

			// Custom fields are one /incident/field/reset call each. The backend
			// accepts an arbitrary JSON value for field_value, sent as the raw
			// value the API expects.
			for name, value := range customFields {
				if _, err := client.New.Incidents.FieldReset(ctx, &flashduty.ResetIncidentFieldRequest{
					IncidentID: incidentID,
					FieldName:  name,
					FieldValue: value,
				}); err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to update custom fields: %v", err)), nil
				}
				updatedFields = append(updatedFields, name)
			}

			if len(updatedFields) == 0 {
				return mcp.NewToolResultError("no fields specified to update"), nil
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

			if _, err := client.New.Incidents.Ack(ctx, &flashduty.AckIncidentRequest{IncidentIDs: incidentIDs}); err != nil {
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

			if _, err := client.New.Incidents.Resolve(ctx, &flashduty.ResolveIncidentRequest{IncidentIDs: incidentIDs}); err != nil {
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

			out, _, err := client.New.Incidents.PastList(ctx, &flashduty.ListPastIncidentsRequest{
				IncidentID: incidentID,
				Limit:      flashduty.Int64(int64(limit)),
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to find similar incidents: %v", err)), nil
			}

			// PastList returns the full similar set without a separate total, so
			// the count is the slice length.
			total := len(out.Items)
			return MarshalResult(addTruncationHint(map[string]any{
				"incidents": out.Items,
				"total":     total,
			}, total, total)), nil
		}
}
