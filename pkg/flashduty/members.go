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

// MemberInfos creates a tool to get member information by person IDs
func MemberInfos(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("flashduty_member_infos",
			mcp.WithDescription(t("TOOL_FLASHDUTY_MEMBER_INFOS_DESCRIPTION", "Get member information by person IDs")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FLASHDUTY_MEMBER_INFOS_USER_TITLE", "Get member infos"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("person_ids",
				mcp.Required(),
				mcp.Description("Comma-separated list of person IDs to get information for. Persons can be accounts or members. Example: '123,456,789'"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract person_ids from request
			personIdsStr, err := RequiredParam[string](request, "person_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse comma-separated string to int slice
			var personIdsInt []int
			if personIdsStr != "" {
				parts := strings.Split(personIdsStr, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" {
						id, err := strconv.Atoi(part)
						if err != nil {
							return mcp.NewToolResultError(fmt.Sprintf("Invalid person_id: %s", part)), nil
						}
						personIdsInt = append(personIdsInt, id)
					}
				}
			}

			if len(personIdsInt) == 0 {
				return mcp.NewToolResultError("person_ids cannot be empty"), nil
			}

			// Get Flashduty client
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			// Build request body according to API specification
			requestBody := map[string]interface{}{
				"person_ids": personIdsInt,
			}

			// Make API request to /person/infos endpoint
			resp, err := client.makeRequest(ctx, "POST", "/person/infos", requestBody)
			if err != nil {
				return nil, fmt.Errorf("failed to get member infos: %w", err)
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
