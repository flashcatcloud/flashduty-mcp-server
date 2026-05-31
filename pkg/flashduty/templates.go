package flashduty

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	flashduty "github.com/flashcatcloud/go-flashduty"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

// presetTemplateID addresses the built-in preset notification template in
// go-flashduty's /template/info endpoint.
const presetTemplateID = "000000000000000000000001"

// --- Tool 1: get_preset_template ---

const getPresetTemplateDescription = `Fetch the preset (default) notification template for a specific channel. Returns the Go template code used as the starting point for customization.`

func sortedChannelEnumValues() []string {
	channels := append([]string(nil), sdk.ChannelEnumValues()...)
	slices.Sort(channels)
	return channels
}

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
				mcp.Enum(sortedChannelEnumValues()...),
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

			fieldName, ok := sdk.TemplateChannels[channel]
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("unknown channel: %s", channel)), nil
			}

			item, _, err := client.New.NotificationTemplates.ReadInfo(ctx, &flashduty.TemplateIDRequest{
				TemplateID: presetTemplateID,
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to fetch preset template: %v", err)), nil
			}

			// ReadInfo returns every channel's template on one TemplateItem;
			// pluck the requested channel's code by its JSON field name.
			templateCode := templateCodeForChannel(item, fieldName)
			if templateCode == "" {
				return mcp.NewToolResultError(fmt.Sprintf("no preset template found for channel: %s", channel)), nil
			}

			return MarshalResult(map[string]any{
				"channel":       channel,
				"field_name":    fieldName,
				"template_code": templateCode,
			}), nil
		}
}

// templateCodeForChannel extracts the per-channel template source from a
// TemplateItem by the channel's JSON field name (e.g. "dingtalk", "email").
func templateCodeForChannel(item *flashduty.TemplateItem, fieldName string) string {
	b, err := json.Marshal(item)
	if err != nil {
		return ""
	}
	var fields map[string]any
	if err := json.Unmarshal(b, &fields); err != nil {
		return ""
	}
	if v, ok := fields[fieldName].(string); ok {
		return v
	}
	return ""
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
				mcp.Enum(sortedChannelEnumValues()...),
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

			fieldName, ok := sdk.TemplateChannels[channel]
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("unknown channel: %s", channel)), nil
			}

			// /template/preview renders the template; the wire `type` is the
			// channel identifier itself (e.g. "dingtalk").
			out, _, err := client.New.NotificationTemplates.ReadPreview(ctx, &flashduty.PreviewTemplateRequest{
				Content:    templateCode,
				IncidentID: incidentID,
				Type:       channel,
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to validate template: %v", err)), nil
			}

			// The raw endpoint only renders + reports parse errors. Size-limit
			// validation (errors/warnings, rendered_size, size_limit) is tool
			// logic the legacy SDK used to fold in; reproduce it here so the
			// output shape stays identical post-migration.
			renderedPreview := out.Content
			renderedSize := len(renderedPreview)
			sizeLimit := sdk.ChannelSizeLimits[channel]

			errs := []string{}
			warnings := []string{}
			if !out.Success {
				errs = append(errs, out.Message)
			}
			if sizeLimit > 0 {
				if renderedSize > sizeLimit {
					sizeWarning := fmt.Sprintf("Rendered output is %d bytes, exceeding the %d byte limit for %s.", renderedSize, sizeLimit, channel)
					switch channel {
					case "telegram":
						sizeWarning += " CRITICAL: Telegram will silently drop this message."
					case "teams_app":
						sizeWarning += " Teams will return an error for this message."
					}
					errs = append(errs, sizeWarning)
				} else if renderedSize > int(float64(sizeLimit)*0.8) {
					warnings = append(warnings, fmt.Sprintf("Rendered output is %d/%d bytes (%.0f%% of limit).", renderedSize, sizeLimit, float64(renderedSize)/float64(sizeLimit)*100))
				}
			}

			return MarshalResult(map[string]any{
				"channel":          channel,
				"field_name":       fieldName,
				"template_code":    templateCode,
				"success":          out.Success && len(errs) == 0,
				"rendered_preview": renderedPreview,
				"rendered_size":    renderedSize,
				"size_limit":       sizeLimit,
				"errors":           errs,
				"warnings":         warnings,
			}), nil
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
