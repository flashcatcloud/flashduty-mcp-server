// This is a new file
package flashduty

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TeamInfos creates a tool to get team information by team IDs
func TeamInfos(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_teams_infos",
			mcp.WithDescription(t("TOOL_FLASHDUTY_TEAMS_INFOS_DESCRIPTION", "Get team information by team IDs")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_TEAMS_INFOS_USER_TITLE", "Get team infos"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("team_ids",
				mcp.Required(),
				mcp.Description("Comma-separated list of team IDs to get information for. Example: '123,456,789'"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract team_ids from request
			teamIdsStr, err := RequiredParam[string](request, "team_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse comma-separated string to int slice
			var teamIdsInt []int
			if teamIdsStr != "" {
				parts := strings.Split(teamIdsStr, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" {
						id, err := strconv.Atoi(part)
						if err != nil {
							return mcp.NewToolResultError(fmt.Sprintf("Invalid team_id: %s", part)), nil
						}
						teamIdsInt = append(teamIdsInt, id)
					}
				}
			}

			if len(teamIdsInt) == 0 {
				return mcp.NewToolResultError("team_ids cannot be empty"), nil
			}

			// Get Flashduty client
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			// Build request body according to API specification
			requestBody := map[string]interface{}{
				"team_ids": teamIdsInt,
			}

			// Make API request to /team/infos endpoint
			resp, err := client.makeRequest(ctx, "POST", "/team/infos", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to get team infos: %w", err)
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
