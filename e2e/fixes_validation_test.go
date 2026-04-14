//go:build e2e

package e2e_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestQueryIncidentsChannelFilter exercises the channel_ids filter end-to-end.
// Regression test for the bug where MCP sent singular `channel_id` and the
// backend silently ignored it, returning incidents from every channel.
func TestQueryIncidentsChannelFilter(t *testing.T) {
	t.Parallel()
	mcpClient := setupMCPClient(t)

	// Find a channel that actually has incidents in the last 30 days.
	now := time.Now().Unix()
	startTime := now - 30*24*60*60

	allText := callTool(t, mcpClient, "query_incidents", map[string]any{
		"start_time":     startTime,
		"end_time":       now,
		"limit":          100,
		"include_alerts": false,
	})
	var allResp struct {
		Incidents []struct {
			IncidentID string `json:"incident_id"`
			ChannelID  int64  `json:"channel_id"`
		} `json:"incidents"`
	}
	unmarshalToolResponse(t, allText, &allResp)
	if len(allResp.Incidents) == 0 {
		t.Skip("no incidents in last 30 days; cannot exercise channel filter")
	}

	// Bucket by channel; pick the channel with the most incidents to give us a
	// non-trivial filter target. Also count how many incidents belong to *other*
	// channels so we can prove filtering actually narrowed the result.
	counts := map[int64]int{}
	for _, inc := range allResp.Incidents {
		counts[inc.ChannelID]++
	}
	var target int64
	var maxCount int
	for ch, c := range counts {
		if c > maxCount {
			target, maxCount = ch, c
		}
	}
	otherChannelCount := len(allResp.Incidents) - maxCount

	t.Logf("Filtering on channel_id=%d (expected %d match, %d non-match in unfiltered set)",
		target, maxCount, otherChannelCount)

	filteredText := callTool(t, mcpClient, "query_incidents", map[string]any{
		"start_time":     startTime,
		"end_time":       now,
		"limit":          100,
		"include_alerts": false,
		"channel_ids":    strconv.FormatInt(target, 10),
	})
	var filtered struct {
		Incidents []struct {
			IncidentID string `json:"incident_id"`
			ChannelID  int64  `json:"channel_id"`
		} `json:"incidents"`
	}
	unmarshalToolResponse(t, filteredText, &filtered)
	require.NotEmpty(t, filtered.Incidents, "expected at least one incident for target channel")

	for _, inc := range filtered.Incidents {
		require.Equal(t, target, inc.ChannelID,
			"channel_ids filter leaked: incident %s has channel %d, expected %d",
			inc.IncidentID, inc.ChannelID, target)
	}
}

// TestQueryMembersNameFilter exercises the name search via the `query` field.
// Regression test for the bug where MCP sent `member_name` (silently dropped
// by backend) instead of `query`.
func TestQueryMembersNameFilter(t *testing.T) {
	t.Parallel()
	mcpClient := setupMCPClient(t)

	allText := callTool(t, mcpClient, "query_members", nil)
	var allResp struct {
		Members []struct {
			MemberID   int64  `json:"member_id"`
			MemberName string `json:"member_name"`
		} `json:"members"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, allText, &allResp)

	if allResp.Total <= 1 || len(allResp.Members) == 0 {
		t.Skip("need >1 member to verify name filter narrows the result set")
	}

	// Pick a substring from the first member with a name long enough to filter.
	// Slice by runes (not bytes) so non-ASCII names like 中文 work correctly.
	var needle string
	var pickedFrom string
	for _, m := range allResp.Members {
		runes := []rune(m.MemberName)
		if len(runes) >= 2 {
			needle = string(runes[:2])
			pickedFrom = m.MemberName
			break
		}
	}
	if needle == "" {
		t.Skip("no member with a >=2 char name")
	}

	t.Logf("Filtering members by name substring %q (taken from %q, total members=%d)",
		needle, pickedFrom, allResp.Total)

	filteredText := callTool(t, mcpClient, "query_members", map[string]any{
		"name": needle,
	})
	var filtered struct {
		Members []struct {
			MemberID   int64  `json:"member_id"`
			MemberName string `json:"member_name"`
			Email      string `json:"email,omitempty"`
		} `json:"members"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, filteredText, &filtered)

	require.NotEmpty(t, filtered.Members,
		"expected at least one member to match substring %q", needle)
	require.Less(t, filtered.Total, allResp.Total,
		"name filter did not narrow the result (got %d, total is %d) — backend likely ignored the filter",
		filtered.Total, allResp.Total)

	// Backend `query` matches name, email, or phone — accept any of those.
	for _, m := range filtered.Members {
		hit := strings.Contains(strings.ToLower(m.MemberName), strings.ToLower(needle)) ||
			strings.Contains(strings.ToLower(m.Email), strings.ToLower(needle))
		require.True(t, hit,
			"member %d (%s, %s) does not match substring %q in name or email",
			m.MemberID, m.MemberName, m.Email, needle)
	}
}

// TestQueryTeamsNameFilter exercises the team name search via `query`.
// Regression test for the bug where MCP sent `team_name` instead of `query`.
func TestQueryTeamsNameFilter(t *testing.T) {
	t.Parallel()
	mcpClient := setupMCPClient(t)

	allText := callTool(t, mcpClient, "query_teams", nil)
	var allResp struct {
		Teams []struct {
			TeamID   int64  `json:"team_id"`
			TeamName string `json:"team_name"`
		} `json:"teams"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, allText, &allResp)

	if allResp.Total <= 1 || len(allResp.Teams) == 0 {
		t.Skip("need >1 team to verify name filter narrows the result set")
	}

	var needle string
	var pickedFrom string
	for _, tm := range allResp.Teams {
		runes := []rune(tm.TeamName)
		if len(runes) >= 2 {
			needle = string(runes[:2])
			pickedFrom = tm.TeamName
			break
		}
	}
	if needle == "" {
		t.Skip("no team with a >=2 char name")
	}

	t.Logf("Filtering teams by name substring %q (taken from %q, total teams=%d)",
		needle, pickedFrom, allResp.Total)

	filteredText := callTool(t, mcpClient, "query_teams", map[string]any{
		"name": needle,
	})
	var filtered struct {
		Teams []struct {
			TeamID   int64  `json:"team_id"`
			TeamName string `json:"team_name"`
		} `json:"teams"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, filteredText, &filtered)

	require.NotEmpty(t, filtered.Teams,
		"expected at least one team to match substring %q", needle)
	require.Less(t, filtered.Total, allResp.Total,
		"name filter did not narrow the result (got %d, total is %d) — backend likely ignored the filter",
		filtered.Total, allResp.Total)

	for _, tm := range filtered.Teams {
		require.Contains(t, strings.ToLower(tm.TeamName), strings.ToLower(needle),
			"team %d (%s) does not contain substring %q", tm.TeamID, tm.TeamName, needle)
	}
}

