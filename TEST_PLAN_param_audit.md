# MCP Server — Parameter Mismatch Test Plan

Hunt for silent parameter mismatches between MCP tool request bodies and backend input structs. Same class of bug as the `channel_id` → `channel_ids` issue fixed in PR #43.

## Scope

20 tools across 6 toolsets in `pkg/flashduty/`:

| Toolset | Tools |
|---|---|
| incidents | QueryIncidents, QueryIncidentTimeline, QueryIncidentAlerts, ListSimilarIncidents, CreateIncident, UpdateIncident, AckIncident, CloseIncident |
| changes | QueryChanges |
| status_page | QueryStatusPages, ListStatusChanges, CreateStatusIncident, CreateChangeTimeline |
| users | QueryMembers, QueryTeams |
| channels | QueryChannels, QueryEscalationRules |
| fields | QueryFields |

## Confirmed bugs (same class as PR #43)

### 1. `query_members` — `member_name` dropped
- **File:** `pkg/flashduty/users.go:73`
- **Sends:** `{"p": 1, "limit": 20, "member_name": "<name>"}`
- **Backend expects** (`fc-pgy/cmd/server/controller/member/member.go:38-45`):
  ```go
  type memberListInput struct {
      RoleID  uint64 `json:"role_id"`
      Page    int    `json:"p"`
      Limit   int    `json:"limit"`
      Orderby string `json:"orderby"`
      Asc     bool   `json:"asc"`
      Query   string `json:"query"`
  }
  ```
- **Impact:** Search-by-name returns all members; `member_name` is silently ignored.
- **Fix:** Send `query: <name>` instead of `member_name: <name>`.

### 2. `query_teams` — `team_name` dropped
- **File:** `pkg/flashduty/users.go:170`
- **Sends:** `{"p": 1, "limit": 20, "team_name": "<name>"}`
- **Backend expects** (`fc-pgy/cmd/server/controller/team/team.go:311-318`):
  ```go
  type listTeamInput struct {
      Page     int    `json:"p"`
      Limit    int    `json:"limit"`
      Orderby  string `json:"orderby"`
      Asc      bool   `json:"asc"`
      PersonID uint64 `json:"person_id"`
      Query    string `json:"query"`
  }
  ```
- **Impact:** Search-by-name returns all teams; `team_name` is silently ignored.
- **Fix:** Send `query: <name>` instead of `team_name: <name>`.
- **Bonus:** backend also supports `person_id` filter the MCP tool doesn't currently expose.

## Suspected / needs runtime verification

### 3. `create_incident` — `assigned_to` shape
- **File:** `pkg/flashduty/incidents.go:413-416`
- **Sends:** `{"assigned_to": {"type": "assign", "person_ids": [123, 456]}}`
- Verify the backend accepts the nested `{type, person_ids}` wrapper vs expecting flat `person_ids` at the top level.

## Runtime test procedure

For each tool, log the outbound HTTP body (look for `msg=duty request` lines in server logs) and confirm the JSON keys match the backend struct tags.

### Confirmed-bug tests
| # | MCP call | Expected outbound body | Red flag if body contains |
|---|---|---|---|
| 1 | `query_members {name: "alice"}` | `{"p":1, "limit":20, "query":"alice"}` | `"member_name"` |
| 2 | `query_teams {name: "backend"}` | `{"p":1, "limit":20, "query":"backend"}` | `"team_name"` |

### Regression tests (post-PR #43)
| # | MCP call | Expected outbound body |
|---|---|---|
| 3 | `query_incidents {channel_ids: "100", start_time: T0, end_time: T1}` | `{"channel_ids":[100], ...}` |
| 4 | `query_incidents {channel_ids: "100,200", ...}` | `{"channel_ids":[100,200], ...}` |
| 5 | `query_incidents {...no channel_ids...}` | body MUST NOT contain `channel_ids` |
| 6 | `query_changes {channel_ids: "100"}` | `{"channel_ids":[100], ...}` |

### Smoke tests for the other 17 tools
Auditor flagged these as clean; verify once by calling each with minimal args and confirming no `400 invalid parameter` from backend:

Incidents: `query_incident_timeline`, `query_incident_alerts`, `list_similar_incidents`, `create_incident`, `update_incident`, `ack_incident`, `close_incident`.
Status page: `query_status_pages`, `list_status_changes`, `create_status_incident`, `create_change_timeline`.
Channels: `query_channels`, `query_escalation_rules`.
Fields: `query_fields`.

### Special case — `create_incident assigned_to`
Call `create_incident` with `assigned_to: "100,101"`. Fetch the created incident and confirm persons 100 and 101 are actually listed as responders. If they're not, the wrapper shape is wrong.

## Exit criteria

- Tests 1–2: fail → open follow-up PR renaming `member_name`/`team_name` → `query`.
- Tests 3–6: pass → PR #43 fix is correct end-to-end.
- Smoke tests: 17/17 return 200 or a documented business error (not `400 invalid parameter`).
- Test for `assigned_to`: responders actually assigned.
- Glossary pass (section below): tool schemas use canonical terms only (no "collaboration space").

## Audit method reproducibility

To re-audit after new tools are added:
1. For each `mcp.NewTool("<name>", ...)` call in `pkg/flashduty/*.go`, capture the `mcp.With*` schema.
2. Locate the corresponding `makeRequest(ctx, "POST", "/<endpoint>", requestBody)`.
3. Grep the backend repos (`fc-event`, `fc-pgy`, `monit-webapi`) for the handler input struct tagged with matching `json:` fields.
4. Compare field names, types (singular vs plural), and required fields.
5. Flag any MCP-side key that doesn't appear as a backend `json:` tag — that key is silently dropped.

## Glossary pass — canonical terminology

Goal: keep user-visible strings (tool descriptions, parameter descriptions, READMEs) aligned with the canonical
Flashduty glossary at `flashduty-docs/glossary.md` so agents and humans see consistent wording.

### Terms checked (from `flashduty-docs/glossary.md`)

| Chinese | Canonical English | Misnomers to hunt |
|---|---|---|
| 协作空间 | channel (lowercase in prose; "Channel" as class/section title) | collaboration space, collab space |
| 故障 | incident | outage, alarm |
| 告警 / 报警 | alert | alarm |
| 集成来源 / 数据源 | integration | data source |
| 分派策略 | escalation rule | — |
| 成员 | member | — (wire uses `person_id`; see "Skipped" below) |
| 处理人员 / 响应人员 | responder | acker, assignee |

zh README (`README_zh.md`) is source of truth and already uses 协作空间; no zh-side edits were required.

### Files edited in this pass

- `pkg/flashduty/channels.go` — `queryChannelsDescription`, `channel_id` param description
- `pkg/flashduty/incidents.go` — `channel_ids` and `channel_id` param descriptions
- `pkg/flashduty/changes.go` — `channel_ids` param description
- `pkg/flashduty/tools.go` — `channels` toolset description
- `README.md` — toolset table + `channels` section heading + `query_channels` bullet
- `e2e/README.md` — TestQueryChannels comment + limitations bullet

### Verification

1. `go build ./...` compiles cleanly.
2. `rg -i "collaboration"` in the repo returns no hits.
3. Start the server and call `list_tools` via MCP; confirm the `query_channels`, `query_escalation_rules`,
   `query_incidents`, `query_changes`, and `create_incident` schemas describe channels using "channel" (not "collaboration space").
4. Run existing snapshot tests (`go test ./pkg/flashduty/...`) — snapshots for these tools do not embed tool
   descriptions, so they should still pass without updates.

### Skipped / uncertain

- `person_ids` parameter and its description in `pkg/flashduty/users.go:26` and `pkg/flashduty/incidents.go:379`.
  Glossary says 成员 → "member", but `person_ids` is the wire-level backend field name. The description text
  explains what the parameter accepts ("Comma-separated person IDs"), so renaming the description would misalign
  with the literal parameter name. Leaving as-is; a future full rename would need to touch wire payload too.
- All `Person*` Go identifiers, struct fields, and JSON tags (wire-level, not user-facing).
- `partial_outage` / `full_outage` enum values in status-page tools — these are wire-level component status
  enums, not references to Flashduty 故障.
