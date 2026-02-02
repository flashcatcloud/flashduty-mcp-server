package flashduty

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/sync/errgroup"

	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

const queryChannelsDescription = `Query collaboration spaces (channels) by IDs or name. Returns channel info with team details.`

// QueryChannels creates a tool to query channels
func QueryChannels(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_channels",
			mcp.WithDescription(t("TOOL_QUERY_CHANNELS_DESCRIPTION", queryChannelsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_CHANNELS_USER_TITLE", "Query channels"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("channel_ids", mcp.Description("Comma-separated channel IDs for direct lookup. Max 1000 IDs.")),
			mcp.WithString("name", mcp.Description("Search by channel name (case-insensitive substring match).")),
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

				// Enrich channels with team and creator names
				enrichedChannels, err := client.enrichChannels(ctx, channels)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch team and creator details for channels: %v", err)), nil
				}

				return MarshalResult(map[string]any{
					"channels": enrichedChannels,
					"total":    len(enrichedChannels),
				}), nil
			}

			// List all channels
			resp, err := client.makeRequest(ctx, "POST", "/channel/list", map[string]interface{}{})
			if err != nil {
				return nil, fmt.Errorf("unable to list channels: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return mcp.NewToolResultError(handleAPIError(resp).Error()), nil
			}

			var result struct {
				Error *DutyError `json:"error,omitempty"`
				Data  *struct {
					Items []struct {
						ChannelID   int64  `json:"channel_id"`
						ChannelName string `json:"channel_name"`
						TeamID      int64  `json:"team_id,omitempty"`
						CreatorID   int64  `json:"creator_id,omitempty"`
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
						CreatorID:   ch.CreatorID,
					})
				}
			}

			// Enrich channels with team and creator names
			enrichedChannels, err := client.enrichChannels(ctx, channels)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch team and creator details for channels: %v", err)), nil
			}

			return MarshalResult(map[string]any{
				"channels": enrichedChannels,
				"total":    len(enrichedChannels),
			}), nil
		}
}

const queryEscalationRulesDescription = `Query escalation rules for a channel. Returns complete rules with notification layers, targets (persons/teams/schedules), webhooks, time filters and alert filters.`

// rawEscalationRule represents the raw API response structure for escalation rules
type rawEscalationRule struct {
	RuleID      string `json:"rule_id"`
	RuleName    string `json:"rule_name"`
	Description string `json:"description,omitempty"`
	ChannelID   int64  `json:"channel_id"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	AggrWindow  int    `json:"aggr_window"`
	Layers      []struct {
		MaxTimes       int     `json:"max_times"`
		NotifyStep     float64 `json:"notify_step"`
		EscalateWindow int     `json:"escalate_window"`
		ForceEscalate  bool    `json:"force_escalate"`
		Target         *struct {
			PersonIDs         []int64           `json:"person_ids,omitempty"`
			TeamIDs           []int64           `json:"team_ids,omitempty"`
			ScheduleToRoleIDs map[int64][]int64 `json:"schedule_to_role_ids,omitempty"`
			By                *struct {
				FollowPreference bool     `json:"follow_preference"`
				Critical         []string `json:"critical,omitempty"`
				Warning          []string `json:"warning,omitempty"`
				Info             []string `json:"info,omitempty"`
			} `json:"by,omitempty"`
			Webhooks []struct {
				Type     string         `json:"type"`
				Settings map[string]any `json:"settings,omitempty"`
			} `json:"webhooks,omitempty"`
		} `json:"target,omitempty"`
	} `json:"layers,omitempty"`
	TimeFilters []struct {
		Start  string `json:"start"`
		End    string `json:"end"`
		Repeat []int  `json:"repeat,omitempty"`
		CalID  string `json:"cal_id,omitempty"`
		IsOff  bool   `json:"is_off,omitempty"`
	} `json:"time_filters,omitempty"`
	// Filters is []AndFilters where AndFilters is []*Filter
	Filters [][]struct {
		Key  string   `json:"key"`
		Oper string   `json:"oper"`
		Vals []string `json:"vals"`
	} `json:"filters,omitempty"`
}

// QueryEscalationRules creates a tool to query escalation rules
func QueryEscalationRules(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_escalation_rules",
			mcp.WithDescription(t("TOOL_QUERY_ESCALATION_RULES_DESCRIPTION", queryEscalationRulesDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_ESCALATION_RULES_USER_TITLE", "Query escalation rules"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithNumber("channel_id", mcp.Required(), mcp.Description("Collaboration space (channel) ID to query escalation rules for.")),
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
				return mcp.NewToolResultError(handleAPIError(resp).Error()), nil
			}

			var result struct {
				Error *DutyError `json:"error,omitempty"`
				Data  *struct {
					Items []rawEscalationRule `json:"items"`
				} `json:"data,omitempty"`
			}
			if err := parseResponse(resp, &result); err != nil {
				return nil, err
			}
			if result.Error != nil {
				return mcp.NewToolResultError(fmt.Sprintf("API error: %s - %s", result.Error.Code, result.Error.Message)), nil
			}

			rules := []EscalationRule{}
			if result.Data == nil || len(result.Data.Items) == 0 {
				return MarshalResult(map[string]any{
					"rules": rules,
					"total": 0,
				}), nil
			}

			// Collect all IDs for enrichment
			personIDs := make([]int64, 0)
			teamIDs := make([]int64, 0)
			scheduleIDs := make([]int64, 0)

			for _, r := range result.Data.Items {
				for _, l := range r.Layers {
					if l.Target == nil {
						continue
					}
					for _, pid := range l.Target.PersonIDs {
						if pid != 0 {
							personIDs = append(personIDs, pid)
						}
					}
					for _, tid := range l.Target.TeamIDs {
						if tid != 0 {
							teamIDs = append(teamIDs, tid)
						}
					}
					for sid := range l.Target.ScheduleToRoleIDs {
						if sid != 0 {
							scheduleIDs = append(scheduleIDs, sid)
						}
					}
				}
			}

			// Fetch enrichment info concurrently (graceful degradation on errors)
			var personMap map[int64]PersonInfo
			var teamMap map[int64]TeamInfo
			var scheduleMap map[int64]ScheduleInfo
			var channelMap map[int64]ChannelInfo

			g, gctx := errgroup.WithContext(ctx)

			g.Go(func() error {
				var err error
				personMap, err = client.fetchPersonInfos(gctx, personIDs)
				if err != nil {
					personMap = make(map[int64]PersonInfo)
				}
				return nil
			})

			g.Go(func() error {
				var err error
				teamMap, err = client.fetchTeamInfos(gctx, teamIDs)
				if err != nil {
					teamMap = make(map[int64]TeamInfo)
				}
				return nil
			})

			g.Go(func() error {
				var err error
				scheduleMap, err = client.fetchScheduleInfos(gctx, scheduleIDs)
				if err != nil {
					// Log error for debugging, but continue with empty map (graceful degradation)
					slog.Warn("failed to fetch schedule infos", "error", err, "schedule_ids", scheduleIDs)
					scheduleMap = make(map[int64]ScheduleInfo)
				}
				return nil
			})

			g.Go(func() error {
				var err error
				channelMap, err = client.fetchChannelInfos(gctx, []int64{int64(channelID)})
				if err != nil {
					channelMap = make(map[int64]ChannelInfo)
				}
				return nil
			})

			_ = g.Wait()

			// Build enriched rules
			for _, r := range result.Data.Items {
				rule := EscalationRule{
					RuleID:      r.RuleID,
					RuleName:    r.RuleName,
					Description: r.Description,
					ChannelID:   r.ChannelID,
					Status:      r.Status,
					Priority:    r.Priority,
					AggrWindow:  r.AggrWindow,
				}

				// Enrich channel name
				if ch, ok := channelMap[r.ChannelID]; ok {
					rule.ChannelName = ch.ChannelName
				}

				// Build time filters
				if len(r.TimeFilters) > 0 {
					rule.TimeFilters = make([]TimeFilter, 0, len(r.TimeFilters))
					for _, tf := range r.TimeFilters {
						rule.TimeFilters = append(rule.TimeFilters, TimeFilter{
							Start:  tf.Start,
							End:    tf.End,
							Repeat: tf.Repeat,
							CalID:  tf.CalID,
							IsOff:  tf.IsOff,
						})
					}
				}

				// Build alert filters (OR groups of AND conditions)
				if len(r.Filters) > 0 {
					rule.Filters = make(AlertFilters, 0, len(r.Filters))
					for _, andGroup := range r.Filters {
						group := make(AlertFilterGroup, 0, len(andGroup))
						for _, cond := range andGroup {
							group = append(group, AlertCondition{
								Key:  cond.Key,
								Oper: cond.Oper,
								Vals: cond.Vals,
							})
						}
						rule.Filters = append(rule.Filters, group)
					}
				}

				// Build layers
				if len(r.Layers) > 0 {
					rule.Layers = make([]EscalationLayer, 0, len(r.Layers))
					for idx, l := range r.Layers {
						layer := EscalationLayer{
							LayerIdx:       idx,
							Timeout:        l.EscalateWindow,
							NotifyInterval: l.NotifyStep,
							MaxTimes:       l.MaxTimes,
							ForceEscalate:  l.ForceEscalate,
						}

						if l.Target != nil {
							target := &EscalationTarget{}

							// Build persons list with names
							if len(l.Target.PersonIDs) > 0 {
								target.Persons = make([]PersonTarget, 0, len(l.Target.PersonIDs))
								for _, pid := range l.Target.PersonIDs {
									pt := PersonTarget{PersonID: pid}
									if p, ok := personMap[pid]; ok {
										pt.PersonName = p.PersonName
										pt.Email = p.Email
									}
									target.Persons = append(target.Persons, pt)
								}
							}

							// Build teams list with names
							if len(l.Target.TeamIDs) > 0 {
								target.Teams = make([]TeamTarget, 0, len(l.Target.TeamIDs))
								for _, tid := range l.Target.TeamIDs {
									tt := TeamTarget{TeamID: tid}
									if team, ok := teamMap[tid]; ok {
										tt.TeamName = team.TeamName
									}
									target.Teams = append(target.Teams, tt)
								}
							}

							// Build schedules list with names
							if len(l.Target.ScheduleToRoleIDs) > 0 {
								target.Schedules = make([]ScheduleTarget, 0, len(l.Target.ScheduleToRoleIDs))
								for sid, roleIDs := range l.Target.ScheduleToRoleIDs {
									st := ScheduleTarget{
										ScheduleID: sid,
										RoleIDs:    roleIDs,
									}
									if s, ok := scheduleMap[sid]; ok {
										st.ScheduleName = s.ScheduleName
									}
									target.Schedules = append(target.Schedules, st)
								}
							}

							// Build notify by (direct message configuration)
							if l.Target.By != nil {
								target.NotifyBy = &NotifyBy{
									FollowPreference: l.Target.By.FollowPreference,
									Critical:         l.Target.By.Critical,
									Warning:          l.Target.By.Warning,
									Info:             l.Target.By.Info,
								}
							}

							// Build webhooks
							if len(l.Target.Webhooks) > 0 {
								target.Webhooks = make([]WebhookConfig, 0, len(l.Target.Webhooks))
								for _, wh := range l.Target.Webhooks {
									whConfig := WebhookConfig{
										Type:     wh.Type,
										Settings: wh.Settings,
									}
									// Extract alias from settings if available
									if wh.Settings != nil {
										if alias, ok := wh.Settings["alias"].(string); ok {
											whConfig.Alias = alias
										}
									}
									target.Webhooks = append(target.Webhooks, whConfig)
								}
							}

							layer.Target = target
						}

						rule.Layers = append(rule.Layers, layer)
					}
				}

				rules = append(rules, rule)
			}

			return MarshalResult(map[string]any{
				"rules": rules,
				"total": len(rules),
			}), nil
		}
}
