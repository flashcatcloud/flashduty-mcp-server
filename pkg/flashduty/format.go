package flashduty

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	toon "github.com/toon-format/toon-go"
)

// OutputFormat defines the serialization format for tool results
type OutputFormat string

const (
	// OutputFormatJSON uses standard JSON serialization (default)
	OutputFormatJSON OutputFormat = "json"
	// OutputFormatTOON uses Token-Oriented Object Notation for reduced token usage
	OutputFormatTOON OutputFormat = "toon"
)

// ParseOutputFormat converts a string to OutputFormat, defaulting to JSON
func ParseOutputFormat(s string) OutputFormat {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "toon":
		return OutputFormatTOON
	default:
		return OutputFormatJSON
	}
}

// String returns the string representation of OutputFormat
func (f OutputFormat) String() string {
	return string(f)
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

// MarshalResult serializes the given value according to the current output format
// and returns it as a text result for MCP tool response.
func MarshalResult(v any) *mcp.CallToolResult {
	return MarshalResultWithFormat(v, outputFormat)
}

// MarshalResultWithFormat serializes the given value using the specified format
func MarshalResultWithFormat(v any, format OutputFormat) *mcp.CallToolResult {
	var data []byte
	var err error

	switch format {
	case OutputFormatTOON:
		data, err = toon.Marshal(v)
	default:
		data, err = json.Marshal(v)
	}

	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err))
	}

	return mcp.NewToolResultText(string(data))
}

// MarshalledTextResult is the original function that always uses JSON.
// Kept for backward compatibility. New code should use MarshalResult.
func MarshalledTextResult(v any) *mcp.CallToolResult {
	r, _ := json.Marshal(v)
	return mcp.NewToolResultText(string(r))
}
