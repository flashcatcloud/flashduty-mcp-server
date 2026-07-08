package flashduty

import (
	"context"
	"fmt"

	flashduty "github.com/flashcatcloud/go-flashduty"
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
			mcp.WithNumber("limit", mcp.Description(LimitDescription), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
			mcp.WithNumber("page", mcp.Description(PageDescription), mcp.DefaultNumber(1), mcp.Min(1)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			personIdsStr, _ := OptionalParam[string](request, "person_ids")
			name, _ := OptionalParam[string](request, "name")
			email, _ := OptionalParam[string](request, "email")
			limit, page := optionalPaging(request, defaultQueryLimit)

			// Direct ID lookup uses /member/infos (PersonInfos), which returns
			// profiles without a separate total.
			if personIdsStr != "" {
				ids := parseCommaSeparatedInts(personIdsStr)
				personIDs := make([]uint64, 0, len(ids))
				for _, id := range ids {
					if id < 0 {
						continue
					}
					personIDs = append(personIDs, uint64(id))
				}
				if len(personIDs) == 0 {
					return mcp.NewToolResultError("person_ids must contain at least one valid ID when specified"), nil
				}
				out, _, err := client.New.Members.PersonInfos(ctx, &flashduty.PersonInfosRequest{PersonIDs: personIDs})
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve members: %v", err)), nil
				}
				count := len(out.Items)
				return MarshalResult(addTruncationHint(map[string]any{
					"members": out.Items,
					"total":   count,
				}, count, count)), nil
			}

			// Name/email search uses /member/list. go-flashduty exposes a single
			// free-text Query (no dedicated email field), so fold name/email into
			// it, preferring name when both are supplied.
			query := name
			if query == "" {
				query = email
			}
			memberReq := &flashduty.MemberListRequest{Query: query}
			memberReq.Limit = limit
			if page > 1 {
				memberReq.Page = page
			}
			out, _, err := client.New.Members.MemberList(ctx, memberReq)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve members: %v", err)), nil
			}

			total := int(out.Total)
			return MarshalResult(addPageHint(map[string]any{
				"members": out.Items,
				"total":   total,
			}, len(out.Items), total, page, limit)), nil
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
			mcp.WithNumber("limit", mcp.Description(LimitDescription), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
			mcp.WithNumber("page", mcp.Description(PageDescription), mcp.DefaultNumber(1), mcp.Min(1)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			teamIdsStr, _ := OptionalParam[string](request, "team_ids")
			name, _ := OptionalParam[string](request, "name")
			limit, page := optionalPaging(request, defaultQueryLimit)

			// Direct ID lookup uses /team/infos (ReadInfos) and preserves the
			// historical `items`-only response shape.
			if teamIdsStr != "" {
				ids := parseCommaSeparatedInts(teamIdsStr)
				teamIDs := make([]uint64, 0, len(ids))
				for _, id := range ids {
					if id < 0 {
						continue
					}
					teamIDs = append(teamIDs, uint64(id))
				}
				if len(teamIDs) == 0 {
					return mcp.NewToolResultError("team_ids must contain at least one valid ID when specified"), nil
				}
				out, _, err := client.New.Teams.ReadInfos(ctx, &flashduty.TeamInfosRequest{TeamIDs: teamIDs})
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve teams: %v", err)), nil
				}
				return MarshalResult(map[string]any{
					"items": out.Items,
				}), nil
			}

			teamReq := &flashduty.TeamListRequest{Query: name}
			teamReq.Limit = limit
			if page > 1 {
				teamReq.Page = page
			}
			out, _, err := client.New.Teams.ReadList(ctx, teamReq)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve teams: %v", err)), nil
			}

			total := int(out.Total)
			return MarshalResult(addPageHint(map[string]any{
				"teams": out.Items,
				"total": total,
			}, len(out.Items), total, page, limit)), nil
		}
}
