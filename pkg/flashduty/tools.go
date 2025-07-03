package flashduty

import (
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/toolsets"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

// DefaultTools is the default list of enabled Flashduty toolsets
var DefaultTools = []string{"flashduty_members", "flashduty_teams", "flashduty_channels", "flashduty_incidents"}

// DefaultToolsetGroup returns the default toolset group for Flashduty
func DefaultToolsetGroup(getClient GetFlashdutyClientFn, readOnly bool, t translations.TranslationHelperFunc) *toolsets.ToolsetGroup {
	group := toolsets.NewToolsetGroup(readOnly)

	// Members toolset
	members := toolsets.NewToolset("flashduty_members", "Flashduty member management tools").
		AddReadTools(
			toolsets.NewServerTool(MemberInfos(getClient, t)),
		)
	group.AddToolset(members)

	// Teams toolset
	teams := toolsets.NewToolset("flashduty_teams", "Flashduty team management tools").
		AddReadTools(
			toolsets.NewServerTool(TeamInfos(getClient, t)),
		)
	group.AddToolset(teams)

	// Channels toolset
	channels := toolsets.NewToolset("flashduty_channels", "Flashduty collaboration channel management tools").
		AddReadTools(
			toolsets.NewServerTool(ChannelInfos(getClient, t)),
		)
	group.AddToolset(channels)

	// Incidents toolset
	incidents := toolsets.NewToolset("flashduty_incidents", "Flashduty incident management tools").
		AddReadTools(
			toolsets.NewServerTool(IncidentInfos(getClient, t)),
			toolsets.NewServerTool(ListIncidents(getClient, t)),
			toolsets.NewServerTool(ListPastIncidents(getClient, t)),
			toolsets.NewServerTool(GetIncidentTimeline(getClient, t)),
			toolsets.NewServerTool(GetIncidentAlerts(getClient, t)),
		).
		AddWriteTools(
			toolsets.NewServerTool(CreateIncident(getClient, t)),
			toolsets.NewServerTool(AckIncident(getClient, t)),
			toolsets.NewServerTool(ResolveIncident(getClient, t)),
			toolsets.NewServerTool(AssignIncident(getClient, t)),
			toolsets.NewServerTool(AddResponder(getClient, t)),
			toolsets.NewServerTool(SnoozeIncident(getClient, t)),
			toolsets.NewServerTool(MergeIncident(getClient, t)),
			toolsets.NewServerTool(CommentIncident(getClient, t)),
			toolsets.NewServerTool(UpdateIncidentTitle(getClient, t)),
			toolsets.NewServerTool(UpdateIncidentDescription(getClient, t)),
			toolsets.NewServerTool(UpdateIncidentImpact(getClient, t)),
			toolsets.NewServerTool(UpdateIncidentRootCause(getClient, t)),
			toolsets.NewServerTool(UpdateIncidentResolution(getClient, t)),
			toolsets.NewServerTool(UpdateIncidentSeverity(getClient, t)),
			toolsets.NewServerTool(UpdateIncidentFields(getClient, t)),
		)
	group.AddToolset(incidents)

	return group
}
