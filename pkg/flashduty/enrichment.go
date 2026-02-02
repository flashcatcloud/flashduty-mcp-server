package flashduty

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/sync/errgroup"
)

// RawTimelineItem represents raw timeline data from API
type RawTimelineItem struct {
	Type      string         `json:"type"`
	CreatedAt int64          `json:"created_at"`
	PersonID  int64          `json:"person_id,omitempty"`
	Detail    map[string]any `json:"detail,omitempty"`
}

// fetchIncidentTimeline fetches timeline for a single incident
func (c *Client) fetchIncidentTimeline(ctx context.Context, incidentID string) ([]RawTimelineItem, error) {
	requestBody := map[string]interface{}{
		"incident_id": incidentID,
		"limit":       100,
		"asc":         true,
	}

	resp, err := c.makeRequest(ctx, "POST", "/incident/feed", requestBody)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch timeline: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, handleAPIError(resp)
	}

	var result struct {
		Error *DutyError `json:"error,omitempty"`
		Data  *struct {
			Items []RawTimelineItem `json:"items"`
		} `json:"data,omitempty"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}

	if result.Data == nil {
		return nil, nil
	}

	return result.Data.Items, nil
}

// fetchIncidentAlerts fetches alerts for a single incident
func (c *Client) fetchIncidentAlerts(ctx context.Context, incidentID string, limit int) ([]AlertPreview, int, error) {
	requestBody := map[string]interface{}{
		"incident_id": incidentID,
		"p":           1,
		"limit":       limit,
	}

	resp, err := c.makeRequest(ctx, "POST", "/incident/alert/list", requestBody)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to fetch alerts: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, handleAPIError(resp)
	}

	var result struct {
		Error *DutyError `json:"error,omitempty"`
		Data  *struct {
			Total int `json:"total"`
			Items []struct {
				AlertID     string            `json:"alert_id"`
				Title       string            `json:"title"`
				Severity    string            `json:"severity"`
				Status      string            `json:"status"`
				TriggerTime int64             `json:"trigger_time"`
				Labels      map[string]string `json:"labels,omitempty"`
			} `json:"items"`
		} `json:"data,omitempty"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, 0, err
	}
	if result.Error != nil {
		return nil, 0, fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}

	if result.Data == nil {
		return nil, 0, nil
	}

	alerts := make([]AlertPreview, 0, len(result.Data.Items))
	for _, item := range result.Data.Items {
		alerts = append(alerts, AlertPreview{
			AlertID:   item.AlertID,
			Title:     item.Title,
			Severity:  item.Severity,
			Status:    item.Status,
			StartTime: item.TriggerTime,
			Labels:    item.Labels,
		})
	}
	return alerts, result.Data.Total, nil
}

// fetchPersonInfos fetches person information by IDs
func (c *Client) fetchPersonInfos(ctx context.Context, personIDs []int64) (map[int64]PersonInfo, error) {
	if len(personIDs) == 0 {
		return make(map[int64]PersonInfo), nil
	}

	// Deduplicate person IDs
	idSet := make(map[int64]struct{})
	for _, id := range personIDs {
		if id != 0 {
			idSet[id] = struct{}{}
		}
	}
	uniqueIDs := make([]int64, 0, len(idSet))
	for id := range idSet {
		uniqueIDs = append(uniqueIDs, id)
	}

	if len(uniqueIDs) == 0 {
		return make(map[int64]PersonInfo), nil
	}

	requestBody := map[string]interface{}{
		"person_ids": uniqueIDs,
	}

	resp, err := c.makeRequest(ctx, "POST", "/person/infos", requestBody)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch person information: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, handleAPIError(resp)
	}

	var result struct {
		Error *DutyError `json:"error,omitempty"`
		Data  *struct {
			Items []struct {
				PersonID   int64  `json:"person_id"`
				PersonName string `json:"person_name"`
				Email      string `json:"email,omitempty"`
				Avatar     string `json:"avatar,omitempty"`
				As         string `json:"as,omitempty"`
			} `json:"items"`
		} `json:"data,omitempty"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}

	personMap := make(map[int64]PersonInfo)
	if result.Data != nil {
		for _, item := range result.Data.Items {
			personMap[item.PersonID] = PersonInfo{
				PersonID:   item.PersonID,
				PersonName: item.PersonName,
				Email:      item.Email,
				Avatar:     item.Avatar,
				As:         item.As,
			}
		}
	}
	return personMap, nil
}

// fetchTeamInfos fetches team information by IDs
func (c *Client) fetchTeamInfos(ctx context.Context, teamIDs []int64) (map[int64]TeamInfo, error) {
	if len(teamIDs) == 0 {
		return make(map[int64]TeamInfo), nil
	}

	// Deduplicate team IDs
	idSet := make(map[int64]struct{})
	for _, id := range teamIDs {
		if id != 0 {
			idSet[id] = struct{}{}
		}
	}
	uniqueIDs := make([]int64, 0, len(idSet))
	for id := range idSet {
		uniqueIDs = append(uniqueIDs, id)
	}

	if len(uniqueIDs) == 0 {
		return make(map[int64]TeamInfo), nil
	}

	requestBody := map[string]interface{}{
		"team_ids": uniqueIDs,
	}

	resp, err := c.makeRequest(ctx, "POST", "/team/infos", requestBody)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch team information: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, handleAPIError(resp)
	}

	var result struct {
		Error *DutyError `json:"error,omitempty"`
		Data  *struct {
			Items []struct {
				TeamID   int64  `json:"team_id"`
				TeamName string `json:"team_name"`
			} `json:"items"`
		} `json:"data,omitempty"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}

	teamMap := make(map[int64]TeamInfo)
	if result.Data != nil {
		for _, item := range result.Data.Items {
			teamMap[item.TeamID] = TeamInfo{
				TeamID:   item.TeamID,
				TeamName: item.TeamName,
			}
		}
	}
	return teamMap, nil
}

// fetchScheduleInfos fetches schedule information by IDs
func (c *Client) fetchScheduleInfos(ctx context.Context, scheduleIDs []int64) (map[int64]ScheduleInfo, error) {
	if len(scheduleIDs) == 0 {
		return make(map[int64]ScheduleInfo), nil
	}

	// Deduplicate schedule IDs
	idSet := make(map[int64]struct{})
	for _, id := range scheduleIDs {
		if id != 0 {
			idSet[id] = struct{}{}
		}
	}
	uniqueIDs := make([]int64, 0, len(idSet))
	for id := range idSet {
		uniqueIDs = append(uniqueIDs, id)
	}

	if len(uniqueIDs) == 0 {
		return make(map[int64]ScheduleInfo), nil
	}

	requestBody := map[string]interface{}{
		"schedule_ids": uniqueIDs,
	}

	resp, err := c.makeRequest(ctx, "POST", "/schedule/infos", requestBody)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch schedule information: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, handleAPIError(resp)
	}

	// Schedule API returns: {"data": {"items": [{"id": 123, "name": "xxx"}, ...]}}
	// Note: id and name may be pointers in the model, but JSON numbers/strings decode fine
	var result struct {
		Error *DutyError `json:"error,omitempty"`
		Data  *struct {
			Items []struct {
				ID   *int64  `json:"id"`
				Name *string `json:"name"`
			} `json:"items"`
		} `json:"data,omitempty"`
	}
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}

	scheduleMap := make(map[int64]ScheduleInfo)
	if result.Data != nil {
		for _, item := range result.Data.Items {
			if item.ID != nil {
				info := ScheduleInfo{ScheduleID: *item.ID}
				if item.Name != nil {
					info.ScheduleName = *item.Name
				}
				scheduleMap[*item.ID] = info
			}
		}
	}
	return scheduleMap, nil
}

// fetchChannelInfos fetches channel information by IDs
func (c *Client) fetchChannelInfos(ctx context.Context, channelIDs []int64) (map[int64]ChannelInfo, error) {
	if len(channelIDs) == 0 {
		return make(map[int64]ChannelInfo), nil
	}

	// Deduplicate channel IDs
	idSet := make(map[int64]struct{})
	for _, id := range channelIDs {
		if id != 0 {
			idSet[id] = struct{}{}
		}
	}
	uniqueIDs := make([]int64, 0, len(idSet))
	for id := range idSet {
		uniqueIDs = append(uniqueIDs, id)
	}

	if len(uniqueIDs) == 0 {
		return make(map[int64]ChannelInfo), nil
	}

	requestBody := map[string]interface{}{
		"channel_ids": uniqueIDs,
	}

	resp, err := c.makeRequest(ctx, "POST", "/channel/infos", requestBody)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch channel information: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, handleAPIError(resp)
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
		return nil, fmt.Errorf("API error: %s - %s", result.Error.Code, result.Error.Message)
	}

	channelMap := make(map[int64]ChannelInfo)
	if result.Data != nil {
		for _, item := range result.Data.Items {
			channelMap[item.ChannelID] = ChannelInfo{
				ChannelID:   item.ChannelID,
				ChannelName: item.ChannelName,
				TeamID:      item.TeamID,
				CreatorID:   item.CreatorID,
			}
		}
	}
	return channelMap, nil
}

// enrichChannels enriches channel information with team and creator names
func (c *Client) enrichChannels(ctx context.Context, channels []ChannelInfo) ([]ChannelInfo, error) {
	if len(channels) == 0 {
		return channels, nil
	}

	// Collect all team IDs and creator IDs
	teamIDs := make([]int64, 0)
	personIDs := make([]int64, 0)
	for _, ch := range channels {
		if ch.TeamID != 0 {
			teamIDs = append(teamIDs, ch.TeamID)
		}
		if ch.CreatorID != 0 {
			personIDs = append(personIDs, ch.CreatorID)
		}
	}

	// Fetch team and person info concurrently
	var teamMap map[int64]TeamInfo
	var personMap map[int64]PersonInfo
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		teamMap, err = c.fetchTeamInfos(gctx, teamIDs)
		if err != nil {
			// Graceful degradation: continue without team names
			teamMap = make(map[int64]TeamInfo)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		personMap, err = c.fetchPersonInfos(gctx, personIDs)
		if err != nil {
			// Graceful degradation: continue without creator names
			personMap = make(map[int64]PersonInfo)
		}
		return nil
	})

	_ = g.Wait()

	// Enrich channels
	enriched := make([]ChannelInfo, len(channels))
	for i, ch := range channels {
		enriched[i] = ch
		if t, ok := teamMap[ch.TeamID]; ok {
			enriched[i].TeamName = t.TeamName
		}
		if p, ok := personMap[ch.CreatorID]; ok {
			enriched[i].CreatorName = p.PersonName
		}
	}

	return enriched, nil
}

// RawIncident represents raw incident data from API
type RawIncident struct {
	IncidentID  string            `json:"incident_id"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Severity    string            `json:"incident_severity"`
	Progress    string            `json:"progress"`
	StartTime   int64             `json:"start_time"`
	AckTime     int64             `json:"ack_time,omitempty"`
	CloseTime   int64             `json:"close_time,omitempty"`
	ChannelID   int64             `json:"channel_id,omitempty"`
	CreatorID   int64             `json:"creator_id,omitempty"`
	CloserID    int64             `json:"closer_id,omitempty"`
	Responders  []RawResponder    `json:"responders,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Fields      map[string]any    `json:"fields,omitempty"`
}

// RawResponder represents raw responder data from API
type RawResponder struct {
	PersonID       int64 `json:"person_id"`
	AssignedAt     int64 `json:"assigned_at,omitempty"`
	AcknowledgedAt int64 `json:"acknowledged_at,omitempty"`
}

// enrichIncidents enriches incidents with person and channel names (without timeline/alerts)
func (c *Client) enrichIncidents(ctx context.Context, rawIncidents []RawIncident) ([]EnrichedIncident, error) {
	// Collect all person IDs and channel IDs
	personIDs := make([]int64, 0)
	channelIDs := make([]int64, 0)

	for _, inc := range rawIncidents {
		if inc.CreatorID != 0 {
			personIDs = append(personIDs, inc.CreatorID)
		}
		if inc.CloserID != 0 {
			personIDs = append(personIDs, inc.CloserID)
		}
		for _, r := range inc.Responders {
			if r.PersonID != 0 {
				personIDs = append(personIDs, r.PersonID)
			}
		}
		if inc.ChannelID != 0 {
			channelIDs = append(channelIDs, inc.ChannelID)
		}
	}

	// Fetch person and channel info concurrently
	var personMap map[int64]PersonInfo
	var channelMap map[int64]ChannelInfo
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		personMap, err = c.fetchPersonInfos(ctx, personIDs)
		return err
	})

	g.Go(func() error {
		var err error
		channelMap, err = c.fetchChannelInfos(ctx, channelIDs)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Build enriched incidents
	enriched := make([]EnrichedIncident, 0, len(rawIncidents))
	for _, raw := range rawIncidents {
		inc := EnrichedIncident{
			IncidentID:   raw.IncidentID,
			Title:        raw.Title,
			Description:  raw.Description,
			Severity:     raw.Severity,
			Progress:     raw.Progress,
			StartTime:    raw.StartTime,
			AckTime:      raw.AckTime,
			CloseTime:    raw.CloseTime,
			ChannelID:    raw.ChannelID,
			CreatorID:    raw.CreatorID,
			CloserID:     raw.CloserID,
			Labels:       raw.Labels,
			CustomFields: raw.Fields,
		}

		// Enrich channel name
		if ch, ok := channelMap[raw.ChannelID]; ok {
			inc.ChannelName = ch.ChannelName
		}

		// Enrich creator
		if p, ok := personMap[raw.CreatorID]; ok {
			inc.CreatorName = p.PersonName
			inc.CreatorEmail = p.Email
		}

		// Enrich closer
		if p, ok := personMap[raw.CloserID]; ok {
			inc.CloserName = p.PersonName
		}

		// Enrich responders
		if len(raw.Responders) > 0 {
			inc.Responders = make([]EnrichedResponder, 0, len(raw.Responders))
			for _, r := range raw.Responders {
				er := EnrichedResponder{
					PersonID:       r.PersonID,
					AssignedAt:     r.AssignedAt,
					AcknowledgedAt: r.AcknowledgedAt,
				}
				if p, ok := personMap[r.PersonID]; ok {
					er.PersonName = p.PersonName
					er.Email = p.Email
				}
				inc.Responders = append(inc.Responders, er)
			}
		}

		enriched = append(enriched, inc)
	}

	return enriched, nil
}

// collectTimelinePersonIDs extracts all person IDs from timeline items (including nested IDs in detail)
func collectTimelinePersonIDs(items []RawTimelineItem) []int64 {
	personIDs := make([]int64, 0)

	for _, item := range items {
		if item.PersonID != 0 {
			personIDs = append(personIDs, item.PersonID)
		}

		if item.Detail == nil {
			continue
		}

		switch item.Type {
		case "i_assign", "i_a_rspd":
			personIDs = extractPersonIDsFromDetail(item.Detail, "to", personIDs)
			personIDs = extractPersonIDsFromDetail(item.Detail, "person_ids", personIDs)
		case "i_notify":
			personIDs = extractPersonIDsFromDetail(item.Detail, "to", personIDs)
		}
	}

	return personIDs
}

// extractPersonIDsFromDetail extracts person IDs from a detail map field
func extractPersonIDsFromDetail(detail map[string]any, field string, personIDs []int64) []int64 {
	if values, ok := detail[field].([]interface{}); ok {
		for _, v := range values {
			if id, ok := toInt64(v); ok && id != 0 {
				personIDs = append(personIDs, id)
			}
		}
	}
	return personIDs
}

// toInt64 converts interface{} to int64
func toInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

// enrichTimelineItems enriches raw timeline items with person names
func enrichTimelineItems(items []RawTimelineItem, personMap map[int64]PersonInfo) []TimelineEvent {
	events := make([]TimelineEvent, 0, len(items))

	for _, item := range items {
		event := TimelineEvent{
			Type:       item.Type,
			Timestamp:  item.CreatedAt,
			OperatorID: item.PersonID,
		}

		// Enrich operator name
		if p, ok := personMap[item.PersonID]; ok {
			event.OperatorName = p.PersonName
		}

		// Build enriched detail based on event type
		event.Detail = enrichTimelineDetail(item.Type, item.Detail, personMap)

		events = append(events, event)
	}

	return events
}

// enrichTimelineDetail enriches the detail field based on event type
func enrichTimelineDetail(eventType string, detail map[string]any, personMap map[int64]PersonInfo) any {
	if detail == nil {
		return nil
	}

	enriched := copyMap(detail)

	switch eventType {
	case "i_notify":
		enrichPersonIDsField(enriched, "to", personMap)
	case "i_assign", "i_a_rspd":
		enrichPersonIDsField(enriched, "to", personMap)
		enrichPersonIDsField(enriched, "person_ids", personMap)
	}

	return enriched
}

// copyMap creates a shallow copy of a map
func copyMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// enrichPersonIDsField enriches a field containing person IDs with person names
func enrichPersonIDsField(enriched map[string]any, field string, personMap map[int64]PersonInfo) {
	values, ok := enriched[field].([]interface{})
	if !ok {
		return
	}

	enrichedValues := make([]map[string]any, 0, len(values))
	for _, v := range values {
		id, ok := toInt64(v)
		if !ok {
			continue
		}

		entry := map[string]any{"person_id": id}
		if p, ok := personMap[id]; ok {
			entry["person_name"] = p.PersonName
		}
		enrichedValues = append(enrichedValues, entry)
	}
	enriched[field] = enrichedValues
}
