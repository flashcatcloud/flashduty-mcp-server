package flashduty

import (
	"context"
	"fmt"

	sdk "github.com/flashcatcloud/flashduty-sdk"
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

			input := &sdk.ListFieldsInput{
				FieldName: fieldName,
			}

			if fieldIdsStr != "" {
				input.FieldIDs = parseCommaSeparatedStrings(fieldIdsStr)
			}

			output, err := client.ListFields(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve fields: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"fields": output.Fields,
				"total":  output.Total,
			}), nil
		}
}
