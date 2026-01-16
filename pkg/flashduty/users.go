package flashduty

import (
	"context"
	"fmt"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const defaultUsersQueryLimit = 20

const queryMembersDescription = `Query members (users) in the account.

**Parameters:**
- person_ids (optional): Comma-separated person IDs for direct lookup
- name (optional): Search by name (fuzzy match)
- email (optional): Search by email

**Returns:**
- Member list with ID, name, email, and team memberships`

// QueryMembers creates a tool to query members
func QueryMembers(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_members",
			mcp.WithDescription(t("TOOL_QUERY_MEMBERS_DESCRIPTION", queryMembersDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_MEMBERS_USER_TITLE", "Query members"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("person_ids", mcp.Description("Comma-separated person IDs")),
			mcp.WithString("name", mcp.Description("Search by name")),
			mcp.WithString("email", mcp.Description("Search by email")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			personIdsStr, _ := OptionalParam[string](request, "person_ids")
			name, _ := OptionalParam[string](request, "name")
			email, _ := OptionalParam[string](request, "email")

			// Query by person IDs
			if personIdsStr != "" {
				personIDs := parseCommaSeparatedInts(personIdsStr)
				if len(personIDs) == 0 {
					return mcp.NewToolResultError("person_ids must contain at least one valid ID when specified"), nil
				}

				int64IDs := make([]int64, len(personIDs))
				for i, id := range personIDs {
					int64IDs[i] = int64(id)
				}

				personMap, err := client.fetchPersonInfos(ctx, int64IDs)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve members: %v", err)), nil
				}

				members := make([]PersonInfo, 0, len(personMap))
				for _, p := range personMap {
					members = append(members, p)
				}

				return MarshalResult(map[string]any{
					"members": members,
					"total":   len(members),
				}), nil
			}

			// List all members with optional filters
			requestBody := map[string]interface{}{
				"p":     1,
				"limit": defaultUsersQueryLimit,
			}
			if name != "" {
				requestBody["member_name"] = name
			}
			if email != "" {
				requestBody["email"] = email
			}

			resp, err := client.makeRequest(ctx, "POST", "/member/list", requestBody)
			if err != nil {
				return nil, fmt.Errorf("unable to list members: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("API request failed with HTTP status %d", resp.StatusCode)), nil
			}

			var result MemberListResponse
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			members := []MemberItem{}
			total := 0
			if result.Data != nil {
				members = result.Data.Items
				total = result.Data.Total
			}

			return MarshalResult(map[string]any{
				"members": members,
				"total":   total,
			}), nil
		}
}

const queryTeamsDescription = `Query teams in the account.

**Parameters:**
- team_ids (optional): Comma-separated team IDs for direct lookup
- name (optional): Search by team name

**Returns:**
- Team list with members (names and emails)`

// QueryTeams creates a tool to query teams
func QueryTeams(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_teams",
			mcp.WithDescription(t("TOOL_QUERY_TEAMS_DESCRIPTION", queryTeamsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_TEAMS_USER_TITLE", "Query teams"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("team_ids", mcp.Description("Comma-separated team IDs")),
			mcp.WithString("name", mcp.Description("Search by team name")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			teamIdsStr, _ := OptionalParam[string](request, "team_ids")
			name, _ := OptionalParam[string](request, "name")

			// Query by team IDs
			if teamIdsStr != "" {
				teamIDs := parseCommaSeparatedInts(teamIdsStr)
				if len(teamIDs) == 0 {
					return mcp.NewToolResultError("team_ids must contain at least one valid ID when specified"), nil
				}

				requestBody := map[string]interface{}{
					"team_ids": teamIDs,
				}

				resp, err := client.makeRequest(ctx, "POST", "/team/infos", requestBody)
				if err != nil {
					return nil, fmt.Errorf("unable to retrieve teams: %w", err)
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusOK {
					return mcp.NewToolResultError(fmt.Sprintf("API request failed with HTTP status %d", resp.StatusCode)), nil
				}

				var result FlashdutyResponse
				if err := parseResponse(resp, &result); err != nil {
					return nil, err
				}
				if result.Error != nil {
					return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
				}

				return MarshalResult(result.Data), nil
			}

			// List all teams
			requestBody := map[string]interface{}{
				"p":     1,
				"limit": defaultUsersQueryLimit,
			}
			if name != "" {
				requestBody["team_name"] = name
			}

			resp, err := client.makeRequest(ctx, "POST", "/team/list", requestBody)
			if err != nil {
				return nil, fmt.Errorf("unable to list teams: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("API request failed with HTTP status %d", resp.StatusCode)), nil
			}

			var result struct {
				Error *DutyError `json:"error,omitempty"`
				Data  *struct {
					Items []struct {
						TeamID   int64  `json:"team_id"`
						TeamName string `json:"team_name"`
						Members  []struct {
							PersonID   int64  `json:"person_id"`
							PersonName string `json:"person_name"`
							Email      string `json:"email,omitempty"`
						} `json:"members,omitempty"`
					} `json:"items"`
					Total int `json:"total"`
				} `json:"data,omitempty"`
			}
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			teams := []TeamInfo{}
			total := 0
			if result.Data != nil {
				for _, t := range result.Data.Items {
					team := TeamInfo{
						TeamID:   t.TeamID,
						TeamName: t.TeamName,
					}
					if len(t.Members) > 0 {
						team.Members = make([]TeamMember, 0, len(t.Members))
						for _, m := range t.Members {
							team.Members = append(team.Members, TeamMember{
								PersonID:   m.PersonID,
								PersonName: m.PersonName,
								Email:      m.Email,
							})
						}
					}
					teams = append(teams, team)
				}
				total = result.Data.Total
			}

			return MarshalResult(map[string]any{
				"teams": teams,
				"total": total,
			}), nil
		}
}
