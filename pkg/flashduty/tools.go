package flashduty

import (
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/toolsets"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

// DefaultTools is the default list of enabled Flashduty toolsets
var DefaultTools = []string{"incidents", "changes", "status_page", "users", "channels", "fields"}

// DefaultToolsetGroup returns the default toolset group for Flashduty
func DefaultToolsetGroup(getClient GetFlashdutyClientFn, readOnly bool, t translations.TranslationHelperFunc) *toolsets.ToolsetGroup {
	group := toolsets.NewToolsetGroup(readOnly)

	// Incidents toolset (8 tools)
	incidents := toolsets.NewToolset("incidents", "Incident lifecycle management tools").
		AddReadTools(
			toolsets.NewServerTool(QueryIncidents(getClient, t)),
			toolsets.NewServerTool(QueryIncidentTimeline(getClient, t)),
			toolsets.NewServerTool(QueryIncidentAlerts(getClient, t)),
			toolsets.NewServerTool(ListSimilarIncidents(getClient, t)),
		).
		AddWriteTools(
			toolsets.NewServerTool(CreateIncident(getClient, t)),
			toolsets.NewServerTool(UpdateIncident(getClient, t)),
			toolsets.NewServerTool(AckIncident(getClient, t)),
			toolsets.NewServerTool(CloseIncident(getClient, t)),
		)
	group.AddToolset(incidents)

	// Changes toolset (1 tool)
	changes := toolsets.NewToolset("changes", "Change record query tools").
		AddReadTools(
			toolsets.NewServerTool(QueryChanges(getClient, t)),
		)
	group.AddToolset(changes)

	// Status Page toolset (4 tools)
	statusPage := toolsets.NewToolset("status_page", "Status page management tools").
		AddReadTools(
			toolsets.NewServerTool(QueryStatusPages(getClient, t)),
			toolsets.NewServerTool(ListStatusChanges(getClient, t)),
		).
		AddWriteTools(
			toolsets.NewServerTool(CreateStatusIncident(getClient, t)),
			toolsets.NewServerTool(CreateChangeTimeline(getClient, t)),
		)
	group.AddToolset(statusPage)

	// Users toolset (2 tools)
	users := toolsets.NewToolset("users", "Member and team query tools").
		AddReadTools(
			toolsets.NewServerTool(QueryMembers(getClient, t)),
			toolsets.NewServerTool(QueryTeams(getClient, t)),
		)
	group.AddToolset(users)

	// Channels toolset (2 tools)
	channelsToolset := toolsets.NewToolset("channels", "Collaboration space and escalation rule tools").
		AddReadTools(
			toolsets.NewServerTool(QueryChannels(getClient, t)),
			toolsets.NewServerTool(QueryEscalationRules(getClient, t)),
		)
	group.AddToolset(channelsToolset)

	// Fields toolset (1 tool)
	fields := toolsets.NewToolset("fields", "Custom field definition query tools").
		AddReadTools(
			toolsets.NewServerTool(QueryFields(getClient, t)),
		)
	group.AddToolset(fields)

	return group
}
