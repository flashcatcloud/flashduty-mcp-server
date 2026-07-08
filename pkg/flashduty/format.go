package flashduty

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	toon "github.com/toon-format/toon-go"
)

// OutputFormat defines the serialization format for tool results.
type OutputFormat string

const (
	// OutputFormatJSON uses standard JSON serialization (default)
	OutputFormatJSON OutputFormat = "json"
	// OutputFormatTOON uses Token-Oriented Object Notation for reduced token usage
	OutputFormatTOON OutputFormat = "toon"
)

// ParseOutputFormat converts a string to OutputFormat, defaulting to JSON.
func ParseOutputFormat(s string) OutputFormat {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "toon":
		return OutputFormatTOON
	default:
		return OutputFormatJSON
	}
}

// String returns the string representation of OutputFormat.
func (f OutputFormat) String() string { return string(f) }

// marshal serializes v using the given format.
func marshal(v any, format OutputFormat) ([]byte, error) {
	switch format {
	case OutputFormatTOON:
		return toon.Marshal(v)
	default:
		return json.Marshal(v)
	}
}

// outputFormat is the current output format setting (package-level for simplicity)
var outputFormat = OutputFormatJSON

// SetOutputFormat sets the global output format
func SetOutputFormat(format OutputFormat) {
	outputFormat = format
}

// GetOutputFormat returns the current global output format
func GetOutputFormat() OutputFormat {
	return outputFormat
}

// MarshalResult serializes the given value according to the current output
// format and returns it as a text result for an MCP tool response.
//
// Values come from go-flashduty, whose Timestamp/TimestampMilli types already
// render absolute instants as RFC3339, so no post-processing is needed.
func MarshalResult(v any) *mcp.CallToolResult {
	return marshalResultWithFormat(v, outputFormat)
}

func marshalResultWithFormat(v any, format OutputFormat) *mcp.CallToolResult {
	data, err := marshal(v, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err))
	}
	return mcp.NewToolResultText(string(data))
}
