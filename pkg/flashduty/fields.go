package flashduty

import (
	"context"
	"fmt"
	"net/http"

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

			// List all fields
			resp, err := client.makeRequest(ctx, "POST", "/field/list", map[string]any{})
			if err != nil {
				return nil, fmt.Errorf("failed to list fields: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(handleAPIError(resp).Error()), nil
			}

			var result struct {
				Error *DutyError `json:"error,omitempty"`
				Data  *struct {
					Items []struct {
						FieldID      string   `json:"field_id"`
						FieldName    string   `json:"field_name"`
						DisplayName  string   `json:"display_name"`
						FieldType    string   `json:"field_type"`
						ValueType    string   `json:"value_type"`
						Options      []string `json:"options,omitempty"`
						DefaultValue any      `json:"default_value,omitempty"`
					} `json:"items"`
				} `json:"data,omitempty"`
			}
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			// Parse filter IDs
			var filterIDs []string
			if fieldIdsStr != "" {
				filterIDs = parseCommaSeparatedStrings(fieldIdsStr)
			}

			fields := []FieldInfo{}
			if result.Data != nil {
				for _, f := range result.Data.Items {
					// Filter by ID if provided
					if len(filterIDs) > 0 {
						found := false
						for _, id := range filterIDs {
							if id == f.FieldID {
								found = true
								break
							}
						}
						if !found {
							continue
						}
					}

					// Filter by name if provided
					if fieldName != "" && f.FieldName != fieldName {
						continue
					}

					fields = append(fields, FieldInfo{
						FieldID:      f.FieldID,
						FieldName:    f.FieldName,
						DisplayName:  f.DisplayName,
						FieldType:    f.FieldType,
						ValueType:    f.ValueType,
						Options:      f.Options,
						DefaultValue: f.DefaultValue,
					})
				}
			}

			return MarshalResult(map[string]any{
				"fields": fields,
				"total":  len(fields),
			}), nil
		}
}
