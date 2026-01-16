package flashduty

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const queryChannelsDescription = `Query collaboration spaces (channels).

**Parameters:**
- channel_ids (optional): Comma-separated channel IDs for direct lookup
- name (optional): Search by channel name

**Returns:**
- Channel list with team information`

// QueryChannels creates a tool to query channels
func QueryChannels(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_channels",
			mcp.WithDescription(t("TOOL_QUERY_CHANNELS_DESCRIPTION", queryChannelsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_CHANNELS_USER_TITLE", "Query channels"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("channel_ids", mcp.Description("Comma-separated channel IDs")),
			mcp.WithString("name", mcp.Description("Search by channel name")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			channelIdsStr, _ := OptionalParam[string](request, "channel_ids")
			name, _ := OptionalParam[string](request, "name")

			// Query by channel IDs
			if channelIdsStr != "" {
				channelIDs := parseCommaSeparatedInts(channelIdsStr)
				if len(channelIDs) == 0 {
					return mcp.NewToolResultError("channel_ids must contain at least one valid ID when specified"), nil
				}

				int64IDs := make([]int64, len(channelIDs))
				for i, id := range channelIDs {
					int64IDs[i] = int64(id)
				}

				channelMap, err := client.fetchChannelInfos(ctx, int64IDs)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve channels: %v", err)), nil
				}

				channels := make([]ChannelInfo, 0, len(channelMap))
				for _, ch := range channelMap {
					channels = append(channels, ch)
				}

				return MarshalResult(map[string]any{
					"channels": channels,
					"total":    len(channels),
				}), nil
			}

			// List all channels
			resp, err := client.makeRequest(ctx, "POST", "/channel/list", map[string]interface{}{})
			if err != nil {
				return nil, fmt.Errorf("unable to list channels: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("API request failed with HTTP status %d", resp.StatusCode)), nil
			}

			var result struct {
				Error *DutyError `json:"error,omitempty"`
				Data  *struct {
					Items []struct {
						ChannelID   int64  `json:"channel_id"`
						ChannelName string `json:"channel_name"`
						TeamID      int64  `json:"team_id,omitempty"`
					} `json:"items"`
				} `json:"data,omitempty"`
			}
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			channels := []ChannelInfo{}
			if result.Data != nil {
				for _, ch := range result.Data.Items {
					// Filter by name if provided (case-insensitive substring match)
					if name != "" && !strings.Contains(strings.ToLower(ch.ChannelName), strings.ToLower(name)) {
						continue
					}
					channels = append(channels, ChannelInfo{
						ChannelID:   ch.ChannelID,
						ChannelName: ch.ChannelName,
						TeamID:      ch.TeamID,
					})
				}
			}

			return MarshalResult(map[string]any{
				"channels": channels,
				"total":    len(channels),
			}), nil
		}
}

const queryEscalationRulesDescription = `Query escalation rules for a collaboration space.

**Parameters:**
- channel_id (required): Collaboration space ID

**Returns:**
- Escalation rules with layers and targets (enriched with names)`

// QueryEscalationRules creates a tool to query escalation rules
func QueryEscalationRules(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_escalation_rules",
			mcp.WithDescription(t("TOOL_QUERY_ESCALATION_RULES_DESCRIPTION", queryEscalationRulesDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_ESCALATION_RULES_USER_TITLE", "Query escalation rules"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithNumber("channel_id", mcp.Required(), mcp.Description("Collaboration space ID")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			channelID, err := RequiredInt(request, "channel_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			requestBody := map[string]interface{}{
				"channel_id": channelID,
			}

			resp, err := client.makeRequest(ctx, "POST", "/channel/escalate/rule/list", requestBody)
			if err != nil {
				return nil, fmt.Errorf("unable to query escalation rules: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("API request failed with HTTP status %d", resp.StatusCode)), nil
			}

			var result struct {
				Error *DutyError `json:"error,omitempty"`
				Data  *struct {
					Items []struct {
						RuleID      string `json:"rule_id"`
						RuleName    string `json:"rule_name"`
						Description string `json:"description,omitempty"`
						ChannelID   int64  `json:"channel_id"`
						Status      string `json:"status"`
						Layers      []struct {
							MaxTimes       int `json:"max_times"`
							NotifyStep     int `json:"notify_step"`
							EscalateWindow int `json:"escalate_window"`
							Target         struct {
								PersonIDs []int64 `json:"person_ids,omitempty"`
							} `json:"target,omitempty"`
						} `json:"layers,omitempty"`
					} `json:"items"`
				} `json:"data,omitempty"`
			}
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			rules := []EscalationRule{}
			if result.Data != nil {
				// Collect all person IDs for enrichment
				personIDs := make([]int64, 0)
				for _, r := range result.Data.Items {
					for _, l := range r.Layers {
						for _, pid := range l.Target.PersonIDs {
							if pid != 0 {
								personIDs = append(personIDs, pid)
							}
						}
					}
				}

				// Fetch person info (graceful degradation: continue without names if fetch fails)
				personMap, err := client.fetchPersonInfos(ctx, personIDs)
				if err != nil {
					personMap = make(map[int64]PersonInfo)
				}

				// Build enriched rules
				for _, r := range result.Data.Items {
					rule := EscalationRule{
						RuleID:      r.RuleID,
						RuleName:    r.RuleName,
						Description: r.Description,
						ChannelID:   r.ChannelID,
						Status:      r.Status,
					}

					if len(r.Layers) > 0 {
						rule.Layers = make([]EscalationLayer, 0, len(r.Layers))
						for idx, l := range r.Layers {
							layer := EscalationLayer{
								LayerIdx:       idx,
								Timeout:        l.EscalateWindow,
								NotifyInterval: l.NotifyStep,
								MaxTimes:       l.MaxTimes,
							}

							if len(l.Target.PersonIDs) > 0 {
								layer.Targets = make([]EscalationTarget, 0, len(l.Target.PersonIDs))
								for _, pid := range l.Target.PersonIDs {
									target := EscalationTarget{
										Type: "person",
										ID:   pid,
									}
									if p, ok := personMap[pid]; ok {
										target.Name = p.PersonName
									}
									layer.Targets = append(layer.Targets, target)
								}
							}

							rule.Layers = append(rule.Layers, layer)
						}
					}

					rules = append(rules, rule)
				}
			}

			return MarshalResult(map[string]any{
				"rules": rules,
				"total": len(rules),
			}), nil
		}
}
