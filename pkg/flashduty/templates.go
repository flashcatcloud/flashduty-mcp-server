package flashduty

import (
	"context"
	"fmt"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

// --- Tool 1: get_preset_template ---

const getPresetTemplateDescription = `Fetch the preset (default) notification template for a specific channel. Returns the Go template code used as the starting point for customization.`

// GetPresetTemplate creates a tool to fetch the preset template for a channel.
func GetPresetTemplate(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_preset_template",
			mcp.WithDescription(t("TOOL_GET_PRESET_TEMPLATE_DESCRIPTION", getPresetTemplateDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_PRESET_TEMPLATE_USER_TITLE", "Get preset template"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("channel",
				mcp.Required(),
				mcp.Description("The notification channel to get the preset template for."),
				mcp.Enum(sdk.ChannelEnumValues()...),
			),
			mcp.WithString("locale",
				mcp.Description("Locale for the preset template. Defaults to zh-CN."),
				mcp.Enum("zh-CN", "en-US"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			channel, err := RequiredParam[string](request, "channel")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			locale, _ := OptionalParam[string](request, "locale")
			if locale == "" {
				locale = "zh-CN"
			}

			input := &sdk.GetPresetTemplateInput{
				Channel: channel,
				Locale:  locale,
			}

			output, err := client.GetPresetTemplate(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to fetch preset template: %v", err)), nil
			}

			return MarshalResult(output), nil
		}
}

// --- Tool 2: validate_template ---

const validateTemplateDescription = `Validate a notification template by parsing it and rendering with incident data. Returns the rendered preview, validation status, and size information. Supports both mock data (default) and real incident preview via incident_id.`

// ValidateTemplate creates a tool to validate and preview a template.
func ValidateTemplate(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("validate_template",
			mcp.WithDescription(t("TOOL_VALIDATE_TEMPLATE_DESCRIPTION", validateTemplateDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_VALIDATE_TEMPLATE_USER_TITLE", "Validate template"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("channel",
				mcp.Required(),
				mcp.Description("The notification channel this template is for."),
				mcp.Enum(sdk.ChannelEnumValues()...),
			),
			mcp.WithString("template_code",
				mcp.Required(),
				mcp.Description("The Go template code to validate and preview."),
			),
			mcp.WithString("incident_id",
				mcp.Description("Optional incident ID for real data preview. If omitted, uses mock data."),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			channel, err := RequiredParam[string](request, "channel")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			templateCode, err := RequiredParam[string](request, "template_code")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			incidentID, _ := OptionalParam[string](request, "incident_id")

			input := &sdk.ValidateTemplateInput{
				Channel:      channel,
				TemplateCode: templateCode,
				IncidentID:   incidentID,
			}

			output, err := client.ValidateTemplate(ctx, input)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to validate template: %v", err)), nil
			}

			return MarshalResult(output), nil
		}
}

// --- Tool 3: list_template_variables ---

const listTemplateVariablesDescription = `List all available template variables that can be used in notification templates. Returns typed variable schema with descriptions and example values.`

// ListTemplateVariables creates a tool that returns the available template variable schema.
func ListTemplateVariables(_ GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_template_variables",
			mcp.WithDescription(t("TOOL_LIST_TEMPLATE_VARIABLES_DESCRIPTION", listTemplateVariablesDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_TEMPLATE_VARIABLES_USER_TITLE", "List template variables"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
		), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			variables := sdk.TemplateVariables()
			return MarshalResult(map[string]any{
				"variables": variables,
				"total":     len(variables),
			}), nil
		}
}

// --- Tool 4: list_template_functions ---

const listTemplateFunctionsDescription = `List all available template functions that can be used in notification templates. Includes custom FlashDuty functions and commonly used Sprig functions.`

// ListTemplateFunctions creates a tool that returns the available template functions.
func ListTemplateFunctions(_ GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_template_functions",
			mcp.WithDescription(t("TOOL_LIST_TEMPLATE_FUNCTIONS_DESCRIPTION", listTemplateFunctionsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_TEMPLATE_FUNCTIONS_USER_TITLE", "List template functions"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
		), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return MarshalResult(map[string]any{
				"custom_functions": sdk.TemplateCustomFunctions(),
				"sprig_functions":  sdk.TemplateSprigFunctions(),
			}), nil
		}
}
