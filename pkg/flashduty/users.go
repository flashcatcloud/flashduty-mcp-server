package flashduty

import (
	"context"
	"fmt"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const queryMembersDescription = `Query members (users) by IDs, name, or email. Returns member info with team memberships.`

// QueryMembers creates a tool to query members
func QueryMembers(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_members",
			mcp.WithDescription(t("TOOL_QUERY_MEMBERS_DESCRIPTION", queryMembersDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_MEMBERS_USER_TITLE", "Query members"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("person_ids", mcp.Description("Comma-separated person IDs for direct lookup.")),
			mcp.WithString("name", mcp.Description("Search by member name (fuzzy match).")),
			mcp.WithString("email", mcp.Description("Search by email address.")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			personIdsStr, _ := OptionalParam[string](request, "person_ids")
			name, _ := OptionalParam[string](request, "name")
			email, _ := OptionalParam[string](request, "email")

			input := &sdk.ListMembersInput{
				Name:  name,
				Email: email,
			}

			if personIdsStr != "" {
				personIDs := parseCommaSeparatedInts(personIdsStr)
				if len(personIDs) == 0 {
					return mcp.NewToolResultError("person_ids must contain at least one valid ID when specified"), nil
				}
				int64IDs := make([]int64, len(personIDs))
				for i, id := range personIDs {
					int64IDs[i] = int64(id)
				}
				input.PersonIDs = int64IDs
			}

			output, err := client.ListMembers(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve members: %v", err)), nil
			}

			members := any(output.Members)
			if len(output.PersonInfos) > 0 {
				members = output.PersonInfos
			}

			return MarshalResult(map[string]any{
				"members": members,
				"total":   output.Total,
			}), nil
		}
}

const queryTeamsDescription = `Query teams by IDs or name. Returns team info with member details.`

// QueryTeams creates a tool to query teams
func QueryTeams(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_teams",
			mcp.WithDescription(t("TOOL_QUERY_TEAMS_DESCRIPTION", queryTeamsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_TEAMS_USER_TITLE", "Query teams"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("team_ids", mcp.Description("Comma-separated team IDs for direct lookup.")),
			mcp.WithString("name", mcp.Description("Search by team name.")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			teamIdsStr, _ := OptionalParam[string](request, "team_ids")
			name, _ := OptionalParam[string](request, "name")

			input := &sdk.ListTeamsInput{
				Name: name,
			}

			if teamIdsStr != "" {
				teamIDs := parseCommaSeparatedInts(teamIdsStr)
				if len(teamIDs) == 0 {
					return mcp.NewToolResultError("team_ids must contain at least one valid ID when specified"), nil
				}
				int64IDs := make([]int64, len(teamIDs))
				for i, id := range teamIDs {
					int64IDs[i] = int64(id)
				}
				input.TeamIDs = int64IDs
			}

			output, err := client.ListTeams(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve teams: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"teams": output.Teams,
				"total": output.Total,
			}), nil
		}
}
