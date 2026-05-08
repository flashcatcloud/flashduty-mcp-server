package flashduty

import "fmt"

// LimitDescription is the canonical description for the `limit` parameter on
// list-shaped tools. Mentioning `truncated`/`total` up front primes the LLM to
// inspect them before assuming it has the full result.
const LimitDescription = "Maximum number of results to return. Default 20, max 100. " +
	"When more results exist than were returned, the response carries " +
	"`truncated:true` and a `hint` field with concrete next steps."

// addTruncationHint stamps `truncated: true` and a human-readable `hint` onto
// list-shaped results when the returned slice is shorter than the backend's
// total. Without these explicit fields the LLM has to remember to compare
// `len(items) < total`, and skipping that check is the most common cause of
// "the LLM only looked at the first 20 incidents and missed the obvious one"
// reports.
//
// No-op when nothing was truncated, so happy-path output stays clean.
// Returns the same map for one-line use.
func addTruncationHint(res map[string]any, count, total int) map[string]any {
	if total > count {
		res["truncated"] = true
		res["hint"] = fmt.Sprintf(
			"Returned %d of %d total. To see more: raise `limit` (max 100), narrow `since`/`until`, or add filters (severity, channel_ids, etc.).",
			count, total,
		)
	}
	return res
}
