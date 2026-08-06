package flashduty

import (
	"context"
	"fmt"

	flashduty "github.com/flashcatcloud/go-flashduty"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/flashcatcloud/flashduty-mcp-server/internal/timeutil"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
)

// RUM (Real User Monitoring) tools expose the front-end error and analytics
// surface — application lookup, error issues, issue detail, ad-hoc SQL
// analytics, and sourcemap stack enrichment. They are aimed at no-shell hosts
// (Cursor / Claude Desktop / product-embedded assistants) where a coding agent
// can't just shell out to the fduty CLI; the toolset stays small and read-only
// on purpose, matching this server's curated philosophy.

// rumMillis converts a unix-seconds bound (as returned by timeutil.ParseAny)
// into the millisecond epoch the RUM APIs expect.
func rumMillis(sec int64) int64 { return sec * 1000 }

// resolveRUMWindow parses the shared since/until args into the millisecond
// window the RUM endpoints take, applying the same "until defaults to now" and
// 31-day-cap rules the incident tools use so error messages stay consistent.
func resolveRUMWindow(request mcp.CallToolRequest) (startMs, endMs int64, errResult *mcp.CallToolResult) {
	args := request.GetArguments()
	startSec, err := timeutil.ParseAny(args["since"])
	if err != nil {
		return 0, 0, mcp.NewToolResultError(fmt.Sprintf("invalid since: %v", err))
	}
	endSec, err := parseUntilArg(args["until"])
	if err != nil {
		return 0, 0, mcp.NewToolResultError(fmt.Sprintf("invalid until: %v", err))
	}
	if err := validateTimeWindow(startSec, endSec); err != nil {
		return 0, 0, mcp.NewToolResultError(err.Error())
	}
	return rumMillis(startSec), rumMillis(endSec), nil
}

const queryRUMApplicationsDescription = `Search RUM (Real User Monitoring) applications by name and return their application_id. Start here when you only know an app's name — the returned application_id feeds query_rum_issues.`

// QueryRUMApplications creates a tool to look up RUM applications.
func QueryRUMApplications(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_rum_applications",
			mcp.WithDescription(t("TOOL_QUERY_RUM_APPLICATIONS_DESCRIPTION", queryRUMApplicationsDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_RUM_APPLICATIONS_USER_TITLE", "Query RUM applications"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("query", mcp.Description("Search by application name (fuzzy match). Omit to list all accessible applications.")),
			mcp.WithNumber("limit", mcp.Description(LimitDescription), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
			mcp.WithNumber("page", mcp.Description(PageDescription), mcp.DefaultNumber(1), mcp.Min(1)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			query, _ := OptionalParam[string](request, "query")
			limit, page := optionalPaging(request, defaultQueryLimit)

			req := &flashduty.RUMApplicationListRequest{Query: query}
			req.Limit = limit
			if page > 1 {
				req.Page = page
			}

			out, _, err := client.New.Applications.ReadList(ctx, req)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve RUM applications: %v", err)), nil
			}

			total := int(out.Total)
			return MarshalResult(addPageHint(map[string]any{
				"applications": out.Items,
				"total":        total,
			}, len(out.Items), total, page, limit)), nil
		}
}

const queryRUMIssuesDescription = `List RUM error issues for one or more applications within a time window, ordered by error count (noisiest first). An "issue" groups many occurrences of the same front-end error. Use get_rum_issue for the full stack of a specific issue. Requires application_ids (get them from query_rum_applications) and a since bound.`

// QueryRUMIssues creates a tool to list RUM error issues.
func QueryRUMIssues(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_rum_issues",
			mcp.WithDescription(t("TOOL_QUERY_RUM_ISSUES_DESCRIPTION", queryRUMIssuesDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_RUM_ISSUES_USER_TITLE", "Query RUM issues"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("application_ids", mcp.Description("Comma-separated RUM application IDs (from query_rum_applications). Required."), mcp.Required()),
			WithSince(mcp.Required()),
			WithUntil(),
			mcp.WithString("statuses", mcp.Description("Filter by issue status, comma-separated. Common value: for_review (open issues awaiting triage).")),
			mcp.WithNumber("limit", mcp.Description(LimitDescription), mcp.DefaultNumber(20), mcp.Min(1), mcp.Max(100)),
			mcp.WithNumber("page", mcp.Description(PageDescription), mcp.DefaultNumber(1), mcp.Min(1)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			appIdsStr, err := RequiredParam[string](request, "application_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			appIDs := parseCommaSeparatedStrings(appIdsStr)
			if len(appIDs) == 0 {
				return mcp.NewToolResultError("application_ids must contain at least one valid ID"), nil
			}

			startMs, endMs, errResult := resolveRUMWindow(request)
			if errResult != nil {
				return errResult, nil
			}

			limit, page := optionalPaging(request, defaultQueryLimit)

			req := &flashduty.RUMIssueListRequest{
				ApplicationIDs: appIDs,
				StartTime:      startMs,
				EndTime:        endMs,
				Orderby:        "error_count",
				ErrorRequired:  true,
			}
			req.Limit = limit
			if page > 1 {
				req.Page = page
			}
			if statuses, _ := OptionalParam[string](request, "statuses"); statuses != "" {
				req.Statuses = parseCommaSeparatedStrings(statuses)
			}

			out, _, err := client.New.Issues.ReadList(ctx, req)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve RUM issues: %v", err)), nil
			}

			total := int(out.Total)
			return MarshalResult(addPageHint(map[string]any{
				"issues": out.Items,
				"total":  total,
			}, len(out.Items), total, page, limit)), nil
		}
}

const getRUMIssueDescription = `Get the full detail of one RUM error issue by issue_id: error type, message, the page URL where it fires, the complete stack trace, affected session count, versions, first/last seen. This is the payload to feed an agent that will locate the offending code.`

// GetRUMIssue creates a tool to fetch a single RUM issue's detail.
func GetRUMIssue(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_rum_issue",
			mcp.WithDescription(t("TOOL_GET_RUM_ISSUE_DESCRIPTION", getRUMIssueDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_RUM_ISSUE_USER_TITLE", "Get RUM issue detail"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("issue_id", mcp.Description("The RUM issue ID (from query_rum_issues)."), mcp.Required()),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			issueID, err := RequiredParam[string](request, "issue_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			out, _, err := client.New.Issues.ReadInfo(ctx, &flashduty.RUMIssueIDRequest{IssueID: issueID})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to retrieve RUM issue: %v", err)), nil
			}
			return MarshalResult(out), nil
		}
}

const queryRUMDataDescription = `Run one ad-hoc RUM SQL query over raw events — the engine behind self-service analytics and custom dashboards. FROM one of: error, action, resource, view, session. format=table returns rows; format=time_series returns points bucketed by interval (seconds). Translate the user's natural-language ask into SQL, e.g. "SELECT browser_name, count(*) AS cnt FROM error GROUP BY browser_name ORDER BY cnt DESC LIMIT 10". Window (since/until) max 31 days.`

// QueryRUMData creates a tool to run an ad-hoc RUM SQL analytics query.
func QueryRUMData(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("query_rum_data",
			mcp.WithDescription(t("TOOL_QUERY_RUM_DATA_DESCRIPTION", queryRUMDataDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_QUERY_RUM_DATA_USER_TITLE", "Query RUM data (SQL)"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("sql", mcp.Description("Full RUM SELECT. FROM error/action/resource/view/session. The time column is event_time; the since/until window is injected automatically, so filter only on business conditions."), mcp.Required()),
			mcp.WithString("format", mcp.Description("Output format. table = rows; time_series = points bucketed by interval."), mcp.Enum("table", "time_series"), mcp.DefaultString("table")),
			mcp.WithNumber("interval", mcp.Description("Time bucket size in seconds. Only used when format=time_series (e.g. 60 for per-minute). The backend floors buckets at 30s.")),
			WithSince(mcp.Required()),
			WithUntil(),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			sql, err := RequiredParam[string](request, "sql")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			format, _ := OptionalParam[string](request, "format")
			if format == "" {
				format = "table"
			}
			interval, _ := OptionalInt(request, "interval")

			startMs, endMs, errResult := resolveRUMWindow(request)
			if errResult != nil {
				return errResult, nil
			}

			q := flashduty.RUMDataQueryDefinition{ID: "q1", Sql: sql, Format: format}
			if format == "time_series" && interval > 0 {
				// The API takes the bucket size in milliseconds, despite the
				// upstream schema comment saying seconds.
				q.Interval = rumMillis(int64(interval))
			}
			req := &flashduty.RUMDataQueryRequest{
				StartTime: startMs,
				EndTime:   endMs,
				Queries:   []flashduty.RUMDataQueryDefinition{q},
			}

			out, _, err := client.New.DataQuery.Query(ctx, req)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to run RUM data query: %v", err)), nil
			}
			return MarshalResult(out), nil
		}
}

const enrichRUMStackDescription = `Turn a minified/compressed front-end stack trace back into source file:line using the sourcemap uploaded for a given service + version. Feed it the raw stack from get_rum_issue when the frames are unreadable.`

// EnrichRUMStack creates a tool to symbolicate a stack via sourcemaps.
func EnrichRUMStack(getClient GetFlashdutyClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("enrich_rum_stack",
			mcp.WithDescription(t("TOOL_ENRICH_RUM_STACK_DESCRIPTION", enrichRUMStackDescription)),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_ENRICH_RUM_STACK_USER_TITLE", "Enrich RUM stack (sourcemap)"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("service", mcp.Description("Application/service name the sourcemap was uploaded under."), mcp.Required()),
			mcp.WithString("version", mcp.Description("Application version the sourcemap was uploaded under."), mcp.Required()),
			mcp.WithString("stack", mcp.Description("Raw (minified) stack trace to symbolicate."), mcp.Required()),
			mcp.WithString("type", mcp.Description("Source platform. Defaults to browser when omitted.")),
			mcp.WithNumber("near", mcp.Description("Number of nearby source lines to return around each converted frame."), mcp.DefaultNumber(3)),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get Flashduty client: %w", err)
			}

			service, err := RequiredParam[string](request, "service")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			version, err := RequiredParam[string](request, "version")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			stack, err := RequiredParam[string](request, "stack")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			typ, _ := OptionalParam[string](request, "type")
			near, _ := OptionalInt(request, "near")

			req := &flashduty.SourcemapStackEnrichRequest{
				Service: service,
				Version: version,
				Stack:   stack,
				Type:    typ,
				Near:    int64(near),
			}

			out, _, err := client.New.Sourcemaps.StackEnrich(ctx, req)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Unable to enrich stack: %v", err)), nil
			}
			return MarshalResult(out), nil
		}
}
