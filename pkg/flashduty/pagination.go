package flashduty

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// LimitDescription is the canonical description for the `limit` parameter on
// list-shaped tools. Mentioning `truncated`/`total` up front primes the LLM to
// inspect them before assuming it has the full result.
const LimitDescription = "Maximum number of results per page. Default 20, max 100. " +
	"When more results exist than were returned, the response carries " +
	"`truncated:true` and a `hint` field with concrete next steps."

// PageDescription is the canonical description for the `page` parameter on
// list-shaped tools. Pairs with `limit`: page N returns results
// [(N-1)*limit, N*limit). Advertised alongside the `truncated`/`hint` fields so
// the LLM knows how to reach results beyond the first page.
const PageDescription = "1-based page number for paging through results beyond `limit`. " +
	"Default 1. When the response is `truncated`, request `page:2` (then 3, …) to fetch the rest."

// optionalPaging reads the shared `limit` and `page` parameters that every
// paginated list tool exposes, applying defLimit when limit is absent/invalid.
// Centralizing this keeps the six list handlers' paging contract identical.
func optionalPaging(r mcp.CallToolRequest, defLimit int) (limit, page int) {
	limit, _ = OptionalInt(r, "limit")
	if limit <= 0 {
		limit = defLimit
	}
	page, _ = OptionalInt(r, "page")
	return limit, page
}

// addTruncationHint stamps `truncated: true` and a human-readable `hint` onto
// list-shaped results when the returned slice is shorter than the backend's
// total. Use it for full-set / by-ID responses that return everything matching
// in one call (no page parameter). For paginated tools use addPageHint, which
// accounts for the page offset. Without these explicit fields the LLM has to
// remember to compare `len(items) < total`, and skipping that check is the most
// common cause of "the LLM only looked at the first 20 incidents and missed the
// obvious one" reports.
//
// No-op when nothing was truncated, so happy-path output stays clean.
// Returns the same map for one-line use.
func addTruncationHint(res map[string]any, count, total int) map[string]any {
	if total > count {
		res["truncated"] = true
		res["hint"] = fmt.Sprintf(
			"Returned %d of %d total. To see more, raise `limit` (max 100) or narrow filters (time window, severity, channel_ids, etc.).",
			count, total,
		)
	}
	return res
}

// addPageHint is the paginated counterpart to addTruncationHint. It flags
// `truncated` and names the next page only when the caller has NOT yet paged
// through everything: `count` is the current page's item count and page/limit
// describe the page just fetched (page is 1-based). This distinction matters —
// comparing `total > count` alone would spuriously flag the final partial page
// as truncated and send the agent chasing an empty next page.
//
// No-op on the last page (and on non-paginated callers where seen >= total), so
// happy-path output stays clean. Returns the same map for one-line use.
func addPageHint(res map[string]any, count, total, page, limit int) map[string]any {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = count
	}
	seen := (page-1)*limit + count
	if total > seen {
		res["truncated"] = true
		res["hint"] = fmt.Sprintf(
			"Showing %d of %d so far (through page %d). Request `page:%d` for the next page, or raise `limit` (max 100) for bigger pages. Narrowing filters also shrinks the result set.",
			seen, total, page, page+1,
		)
	}
	return res
}
