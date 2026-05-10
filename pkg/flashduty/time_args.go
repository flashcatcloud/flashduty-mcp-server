package flashduty

import (
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// MaxTimeWindow is the backend's hard cap on (until-since) for incident/alert/change
// queries. Exceeding it produces a 400 from the API; we mirror the cap client-side
// so the LLM gets a guided error before the round-trip.
const MaxTimeWindow = 31 * 24 * time.Hour

// SinceDescription / UntilDescription are reused across query_incidents
// and query_changes. The wording is tuned for LLM callers that
// otherwise pick absolute dates from stale training data and silently query
// the wrong year — see the three failure modes documented at
// https://github.com/flashcatcloud/flashduty-mcp-server/pull/50.
const (
	SinceDescription = "Lower bound of the query window. " +
		"PREFER relative durations like \"24h\", \"7d\", \"30m\" — they are anchored " +
		"to server time and immune to your training-data cutoff. " +
		"Use absolute dates (\"2026-04-01\" or \"2026-04-01 10:00:00\") ONLY when " +
		"the user explicitly asked for a specific calendar date; double-check the " +
		"year, since picking the wrong year returns silently incorrect data. " +
		"Also accepts unix seconds (\"1712000000\") and \"now\". " +
		"Max window (until - since): 31 days. Data older than ~90 days may have been purged."

	UntilDescription = "Upper bound of the query window. Same formats as `since`, plus " +
		"future durations like \"+24h\", \"+7d\". Defaults to \"now\" when omitted. " +
		"Must be greater than `since` and within 31 days of it."
)

// WithSince / WithUntil are convenience wrappers that apply the canonical descriptions.
func WithSince(opts ...mcp.PropertyOption) mcp.ToolOption {
	return mcp.WithString("since", append([]mcp.PropertyOption{mcp.Description(SinceDescription)}, opts...)...)
}

func WithUntil(opts ...mcp.PropertyOption) mcp.ToolOption {
	return mcp.WithString("until", append([]mcp.PropertyOption{mcp.Description(UntilDescription)}, opts...)...)
}

// validateTimeWindow enforces the same constraints the backend would, but with
// LLM-actionable error messages. since and until are unix seconds; both must be
// non-zero. Returns nil when the window is valid.
func validateTimeWindow(since, until int64) error {
	if since <= 0 || until <= 0 {
		return fmt.Errorf("both since and until are required")
	}
	if since >= until {
		return fmt.Errorf("since (%d) must be earlier than until (%d); did you swap them?", since, until)
	}
	if window := time.Duration(until-since) * time.Second; window > MaxTimeWindow {
		return fmt.Errorf(
			"window of %s exceeds the 31-day max; split into smaller chunks (e.g. 7d at a time) "+
				"or pass *_ids for direct lookup",
			window.Round(time.Hour),
		)
	}
	return nil
}
