package flashduty

import (
	"strconv"
	"strings"
	"testing"
	"time"

	sdk "github.com/flashcatcloud/flashduty-sdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// timeFixture is a small struct exercising both SDK timestamp types the way an
// SDK response object would carry them.
type timeFixture struct {
	CreatedAt sdk.Timestamp      `json:"created_at"`
	UpdatedAt sdk.TimestampMilli `json:"updated_at"`
}

// resultText pulls the single text payload out of an MCP CallToolResult.
func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, res)
	require.False(t, res.IsError, "tool result reported an error: %+v", res.Content)
	require.Len(t, res.Content, 1)
	tc, ok := res.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", res.Content[0])
	return tc.Text
}

// TestMarshalResultRendersTimestampsAsRFC3339 proves the transparent win from
// the typed-timestamp SDK: mcp marshals SDK results straight through and gets
// RFC3339 strings out, never raw epoch integers — with no mcp-side code change.
func TestMarshalResultRendersTimestampsAsRFC3339(t *testing.T) {
	t.Parallel()

	// A concrete instant well clear of the epoch so the year is unambiguous.
	secs := int64(1748487600) // 2025-ish; exact day depends on TZ, year does not.
	wantYear := strconv.Itoa(time.Unix(secs, 0).Year())

	fixture := timeFixture{
		CreatedAt: sdk.Timestamp(secs),
		UpdatedAt: sdk.TimestampMilli(secs * 1000),
	}

	formats := []struct {
		name   string
		format OutputFormat
		// JSON wraps the RFC3339 value in quotes; TOON leaves it bare. The
		// year + 'T' separator are present in both.
		text string
	}{
		{name: "default-json", format: GetOutputFormat()},
		{name: "json", format: OutputFormatJSON},
		{name: "toon", format: OutputFormatTOON},
	}

	for _, f := range formats {
		f := f
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()

			out := resultText(t, MarshalResultWithFormat(fixture, f.format))

			// RFC3339 shape: contains the date/time 'T' separator and the year.
			assert.Contains(t, out, "T", "expected RFC3339 'T' separator in %q", out)
			assert.Contains(t, out, wantYear, "expected RFC3339 year in %q", out)

			// Negative assertion: the raw epoch integers must NOT appear.
			assert.NotContains(t, out, strconv.FormatInt(secs, 10),
				"raw epoch-seconds leaked into output %q", out)
			assert.NotContains(t, out, strconv.FormatInt(secs*1000, 10),
				"raw epoch-millis leaked into output %q", out)
		})
	}
}

// TestMarshalResultUsesGlobalFormat covers the no-arg MarshalResult path so the
// default-format wrapper is exercised too.
func TestMarshalResultUsesGlobalFormat(t *testing.T) {
	secs := int64(1748487600)
	wantYear := strconv.Itoa(time.Unix(secs, 0).Year())

	fixture := timeFixture{CreatedAt: sdk.Timestamp(secs)}

	out := resultText(t, MarshalResult(fixture))
	assert.True(t, strings.Contains(out, wantYear) && strings.Contains(out, "T"),
		"expected RFC3339 timestamp in default-format output %q", out)
	assert.NotContains(t, out, strconv.FormatInt(secs, 10),
		"raw epoch-seconds leaked into default-format output %q", out)
}
