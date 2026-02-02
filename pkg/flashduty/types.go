package flashduty

// EnrichedIncident contains full incident data with human-readable names
type EnrichedIncident struct {
	// Basic fields
	IncidentID  string `json:"incident_id" toon:"incident_id"`
	Title       string `json:"title" toon:"title"`
	Description string `json:"description,omitempty" toon:"description,omitempty"`
	Severity    string `json:"severity" toon:"severity"`
	Progress    string `json:"progress" toon:"progress"`

	// Time fields
	StartTime int64 `json:"start_time" toon:"start_time"`
	AckTime   int64 `json:"ack_time,omitempty" toon:"ack_time,omitempty"`
	CloseTime int64 `json:"close_time,omitempty" toon:"close_time,omitempty"`

	// Channel (enriched)
	ChannelID   int64  `json:"channel_id,omitempty" toon:"channel_id,omitempty"`
	ChannelName string `json:"channel_name,omitempty" toon:"channel_name,omitempty"`

	// Creator (enriched)
	CreatorID    int64  `json:"creator_id,omitempty" toon:"creator_id,omitempty"`
	CreatorName  string `json:"creator_name,omitempty" toon:"creator_name,omitempty"`
	CreatorEmail string `json:"creator_email,omitempty" toon:"creator_email,omitempty"`

	// Closer (enriched)
	CloserID   int64  `json:"closer_id,omitempty" toon:"closer_id,omitempty"`
	CloserName string `json:"closer_name,omitempty" toon:"closer_name,omitempty"`

	// Responders (enriched)
	Responders []EnrichedResponder `json:"responders,omitempty" toon:"responders,omitempty"`

	// Timeline (full)
	Timeline []TimelineEvent `json:"timeline,omitempty" toon:"timeline,omitempty"`

	// Alerts (preview)
	AlertsPreview []AlertPreview `json:"alerts_preview,omitempty" toon:"alerts_preview,omitempty"`
	AlertsTotal   int            `json:"alerts_total" toon:"alerts_total"`

	// Other
	Labels       map[string]string `json:"labels,omitempty" toon:"labels,omitempty"`
	CustomFields map[string]any    `json:"custom_fields,omitempty" toon:"custom_fields,omitempty"`
}

// EnrichedResponder contains responder info with human-readable names
type EnrichedResponder struct {
	PersonID       int64  `json:"person_id" toon:"person_id"`
	PersonName     string `json:"person_name" toon:"person_name"`
	Email          string `json:"email,omitempty" toon:"email,omitempty"`
	AssignedAt     int64  `json:"assigned_at,omitempty" toon:"assigned_at,omitempty"`
	AcknowledgedAt int64  `json:"acknowledged_at,omitempty" toon:"acknowledged_at,omitempty"`
}

// TimelineEvent represents an entry in incident timeline
type TimelineEvent struct {
	Type         string `json:"type" toon:"type"`
	Timestamp    int64  `json:"timestamp" toon:"timestamp"`
	OperatorID   int64  `json:"operator_id,omitempty" toon:"operator_id,omitempty"`
	OperatorName string `json:"operator_name,omitempty" toon:"operator_name,omitempty"`
	Detail       any    `json:"detail,omitempty" toon:"detail,omitempty"`
}

// AlertPreview represents a preview of an alert
type AlertPreview struct {
	AlertID   string            `json:"alert_id" toon:"alert_id"`
	Title     string            `json:"title" toon:"title"`
	Severity  string            `json:"severity" toon:"severity"`
	Status    string            `json:"status" toon:"status"`
	StartTime int64             `json:"start_time" toon:"start_time"`
	Labels    map[string]string `json:"labels,omitempty" toon:"labels,omitempty"`
}

// PersonInfo represents person information from /person/infos API
type PersonInfo struct {
	PersonID   int64  `json:"person_id" toon:"person_id"`
	PersonName string `json:"person_name" toon:"person_name"`
	Email      string `json:"email,omitempty" toon:"email,omitempty"`
	Avatar     string `json:"avatar,omitempty" toon:"avatar,omitempty"`
	As         string `json:"as,omitempty" toon:"as,omitempty"`
}

// ChannelInfo represents channel information with enriched fields
type ChannelInfo struct {
	ChannelID   int64  `json:"channel_id" toon:"channel_id"`
	ChannelName string `json:"channel_name" toon:"channel_name"`
	TeamID      int64  `json:"team_id,omitempty" toon:"team_id,omitempty"`
	TeamName    string `json:"team_name,omitempty" toon:"team_name,omitempty"`
	CreatorID   int64  `json:"creator_id,omitempty" toon:"creator_id,omitempty"`
	CreatorName string `json:"creator_name,omitempty" toon:"creator_name,omitempty"`
}

// TeamInfo represents team information
type TeamInfo struct {
	TeamID   int64        `json:"team_id" toon:"team_id"`
	TeamName string       `json:"team_name" toon:"team_name"`
	Members  []TeamMember `json:"members,omitempty" toon:"members,omitempty"`
}

// TeamMember represents a team member
type TeamMember struct {
	PersonID   int64  `json:"person_id" toon:"person_id"`
	PersonName string `json:"person_name" toon:"person_name"`
	Email      string `json:"email,omitempty" toon:"email,omitempty"`
}

// FieldInfo represents custom field definition
type FieldInfo struct {
	FieldID      string   `json:"field_id" toon:"field_id"`
	FieldName    string   `json:"field_name" toon:"field_name"`
	DisplayName  string   `json:"display_name" toon:"display_name"`
	FieldType    string   `json:"field_type" toon:"field_type"`
	ValueType    string   `json:"value_type" toon:"value_type"`
	Options      []string `json:"options,omitempty" toon:"options,omitempty"`
	DefaultValue any      `json:"default_value,omitempty" toon:"default_value,omitempty"`
}

// EscalationRule represents an escalation rule with full details
type EscalationRule struct {
	RuleID      string            `json:"rule_id" toon:"rule_id"`
	RuleName    string            `json:"rule_name" toon:"rule_name"`
	Description string            `json:"description,omitempty" toon:"description,omitempty"`
	ChannelID   int64             `json:"channel_id" toon:"channel_id"`
	ChannelName string            `json:"channel_name,omitempty" toon:"channel_name,omitempty"`
	Status      string            `json:"status,omitempty" toon:"status,omitempty"`
	Priority    int               `json:"priority" toon:"priority"`
	AggrWindow  int               `json:"aggr_window" toon:"aggr_window"`
	Layers      []EscalationLayer `json:"layers,omitempty" toon:"layers,omitempty"`
	TimeFilters []TimeFilter      `json:"time_filters,omitempty" toon:"time_filters,omitempty"`
	Filters     AlertFilters      `json:"filters,omitempty" toon:"filters,omitempty"`
}

// EscalationLayer represents a layer in an escalation rule
type EscalationLayer struct {
	LayerIdx       int               `json:"layer_idx" toon:"layer_idx"`
	Timeout        int               `json:"timeout" toon:"timeout"`
	NotifyInterval float64           `json:"notify_interval,omitempty" toon:"notify_interval,omitempty"`
	MaxTimes       int               `json:"max_times,omitempty" toon:"max_times,omitempty"`
	ForceEscalate  bool              `json:"force_escalate,omitempty" toon:"force_escalate,omitempty"`
	Target         *EscalationTarget `json:"target,omitempty" toon:"target,omitempty"`
}

// EscalationTarget represents the complete target configuration for a layer
type EscalationTarget struct {
	Persons   []PersonTarget   `json:"persons,omitempty" toon:"persons,omitempty"`
	Teams     []TeamTarget     `json:"teams,omitempty" toon:"teams,omitempty"`
	Schedules []ScheduleTarget `json:"schedules,omitempty" toon:"schedules,omitempty"`
	NotifyBy  *NotifyBy        `json:"notify_by,omitempty" toon:"notify_by,omitempty"`
	Webhooks  []WebhookConfig  `json:"webhooks,omitempty" toon:"webhooks,omitempty"`
}

// PersonTarget represents a person in escalation target
type PersonTarget struct {
	PersonID   int64  `json:"person_id" toon:"person_id"`
	PersonName string `json:"person_name,omitempty" toon:"person_name,omitempty"`
	Email      string `json:"email,omitempty" toon:"email,omitempty"`
}

// TeamTarget represents a team in escalation target with members
type TeamTarget struct {
	TeamID   int64          `json:"team_id" toon:"team_id"`
	TeamName string         `json:"team_name,omitempty" toon:"team_name,omitempty"`
	Members  []PersonTarget `json:"members,omitempty" toon:"members,omitempty"`
}

// ScheduleTarget represents a schedule in escalation target
type ScheduleTarget struct {
	ScheduleID   int64   `json:"schedule_id" toon:"schedule_id"`
	ScheduleName string  `json:"schedule_name,omitempty" toon:"schedule_name,omitempty"`
	RoleIDs      []int64 `json:"role_ids,omitempty" toon:"role_ids,omitempty"`
}

// NotifyBy represents direct message notification configuration
// When follow_preference is true, notifications follow each person's preference settings
// When follow_preference is false, use severity-specific methods (critical/warning/info)
type NotifyBy struct {
	FollowPreference bool     `json:"follow_preference" toon:"follow_preference"`
	Critical         []string `json:"critical,omitempty" toon:"critical,omitempty"`
	Warning          []string `json:"warning,omitempty" toon:"warning,omitempty"`
	Info             []string `json:"info,omitempty" toon:"info,omitempty"`
}

// WebhookConfig represents a webhook configuration in escalation target
type WebhookConfig struct {
	Type     string         `json:"type" toon:"type"`
	Alias    string         `json:"alias,omitempty" toon:"alias,omitempty"`
	Settings map[string]any `json:"settings,omitempty" toon:"settings,omitempty"`
}

// TimeFilter represents time-based filter for rule activation
type TimeFilter struct {
	Start  string `json:"start" toon:"start"`
	End    string `json:"end" toon:"end"`
	Repeat []int  `json:"repeat,omitempty" toon:"repeat,omitempty"`
	CalID  string `json:"cal_id,omitempty" toon:"cal_id,omitempty"`
	IsOff  bool   `json:"is_off,omitempty" toon:"is_off,omitempty"`
}

// AlertFilters represents alert filter conditions as OR groups of AND conditions
// Structure: [[{key,oper,vals}, ...], ...] where outer array is OR, inner array is AND
type AlertFilters []AlertFilterGroup

// AlertFilterGroup represents AND conditions within an OR group
type AlertFilterGroup []AlertCondition

// AlertCondition represents a single filter condition
// Oper can be "IN" (match) or "NOTIN" (not match)
type AlertCondition struct {
	Key  string   `json:"key" toon:"key"`
	Oper string   `json:"oper" toon:"oper"`
	Vals []string `json:"vals" toon:"vals"`
}

// ScheduleInfo represents schedule information from /schedule/infos API
type ScheduleInfo struct {
	ScheduleID   int64  `json:"schedule_id" toon:"schedule_id"`
	ScheduleName string `json:"schedule_name" toon:"schedule_name"`
}

// StatusPage represents a status page
type StatusPage struct {
	PageID        int64             `json:"page_id" toon:"page_id"`
	PageName      string            `json:"page_name" toon:"page_name"`
	Slug          string            `json:"slug,omitempty" toon:"slug,omitempty"`
	Description   string            `json:"description,omitempty" toon:"description,omitempty"`
	Sections      []StatusSection   `json:"sections,omitempty" toon:"sections,omitempty"`
	Components    []StatusComponent `json:"components,omitempty" toon:"components,omitempty"`
	OverallStatus string            `json:"overall_status,omitempty" toon:"overall_status,omitempty"`
}

// StatusSection represents a section in status page
type StatusSection struct {
	SectionID   string `json:"section_id" toon:"section_id"`
	SectionName string `json:"section_name" toon:"section_name"`
}

// StatusComponent represents a component in status page
type StatusComponent struct {
	ComponentID   string `json:"component_id" toon:"component_id"`
	ComponentName string `json:"component_name" toon:"component_name"`
	Status        string `json:"status" toon:"status"`
	SectionID     string `json:"section_id,omitempty" toon:"section_id,omitempty"`
}

// StatusChange represents a change event on status page
type StatusChange struct {
	ChangeID    int64            `json:"change_id" toon:"change_id"`
	PageID      int64            `json:"page_id" toon:"page_id"`
	Title       string           `json:"title" toon:"title"`
	Description string           `json:"description,omitempty" toon:"description,omitempty"`
	Type        string           `json:"type" toon:"type"` // incident or maintenance
	Status      string           `json:"status" toon:"status"`
	CreatedAt   int64            `json:"created_at" toon:"created_at"`
	UpdatedAt   int64            `json:"updated_at,omitempty" toon:"updated_at,omitempty"`
	Timelines   []ChangeTimeline `json:"timelines,omitempty" toon:"timelines,omitempty"`
}

// ChangeTimeline represents a timeline entry in status change
type ChangeTimeline struct {
	TimelineID  int64  `json:"timeline_id" toon:"timeline_id"`
	At          int64  `json:"at" toon:"at"`
	Status      string `json:"status,omitempty" toon:"status,omitempty"`
	Description string `json:"description,omitempty" toon:"description,omitempty"`
}

// Change represents a change record
type Change struct {
	ChangeID    string            `json:"change_id" toon:"change_id"`
	Title       string            `json:"title" toon:"title"`
	Description string            `json:"description,omitempty" toon:"description,omitempty"`
	Type        string            `json:"type,omitempty" toon:"type,omitempty"`
	Status      string            `json:"status,omitempty" toon:"status,omitempty"`
	ChannelID   int64             `json:"channel_id,omitempty" toon:"channel_id,omitempty"`
	ChannelName string            `json:"channel_name,omitempty" toon:"channel_name,omitempty"`
	CreatorID   int64             `json:"creator_id,omitempty" toon:"creator_id,omitempty"`
	CreatorName string            `json:"creator_name,omitempty" toon:"creator_name,omitempty"`
	StartTime   int64             `json:"start_time,omitempty" toon:"start_time,omitempty"`
	EndTime     int64             `json:"end_time,omitempty" toon:"end_time,omitempty"`
	Labels      map[string]string `json:"labels,omitempty" toon:"labels,omitempty"`
}
