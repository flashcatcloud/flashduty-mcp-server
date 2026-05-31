package flashduty

import (
	"context"
	"fmt"

	flashduty "github.com/flashcatcloud/go-flashduty"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const queryFieldsDescription = `Query custom field definitions. Use to discover available fields before updating incidents.`

// QueryFields creates a tool to query custom field definitions
func QueryFields(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_fields",
			mcp.WithDescription(t("TOOL_QUERY_FIELDS_DESCRIPTION", queryFieldsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_FIELDS_USER_TITLE", "Query fields"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("field_ids", mcp.Description("Comma-separated field IDs for direct lookup.")),
			mcp.WithString("field_name", mcp.Description("Search by exact field name. Field names must match pattern: ^[a-z][a-z0-9_]*$")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			fieldIdsStr, _ := OptionalParam[string](request, "field_ids")
			fieldName, _ := OptionalParam[string](request, "field_name")

			// Direct ID lookup: go-flashduty exposes only single-field /field/info,
			// so fan out across the requested IDs.
			if fieldIdsStr != "" {
				fieldIDs := parseCommaSeparatedStrings(fieldIdsStr)
				if len(fieldIDs) == 0 {
					return mcp.NewToolResultError("field_ids must contain at least one valid ID when specified"), nil
				}
				fields := make([]*flashduty.FieldItem, 0, len(fieldIDs))
				for _, id := range fieldIDs {
					item, _, err := client.New.AlertEnrichment.FieldReadInfo(ctx, &flashduty.FieldInfoRequest{FieldID: id})
					if err != nil {
						return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve field %s: %v", id, err)), nil
					}
					fields = append(fields, item)
				}
				return MarshalResult(map[string]any{
					"fields": fields,
					"total":  len(fields),
				}), nil
			}

			// Name search maps to the Query regex filter (matches field_name and
			// display_name); an exact name matches literally.
			out, _, err := client.New.AlertEnrichment.FieldReadList(ctx, &flashduty.FieldListRequest{Query: fieldName})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve fields: %v", err)), nil
			}

			// /field/list returns all matching fields without pagination.
			total := len(out.Items)
			return MarshalResult(addTruncationHint(map[string]any{
				"fields": out.Items,
				"total":  total,
			}, total, total)), nil
		}
}
