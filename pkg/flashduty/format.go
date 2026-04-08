package flashduty

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	sdk "github.com/flashcatcloud/flashduty-sdk"
)

// OutputFormat is a type alias for the SDK's OutputFormat.
type OutputFormat = sdk.OutputFormat

const (
	// OutputFormatJSON uses standard JSON serialization (default)
	OutputFormatJSON = sdk.OutputFormatJSON
	// OutputFormatTOON uses Token-Oriented Object Notation for reduced token usage
	OutputFormatTOON = sdk.OutputFormatTOON
)

// ParseOutputFormat converts a string to OutputFormat, defaulting to JSON.
var ParseOutputFormat = sdk.ParseOutputFormat

// outputFormat is the current output format setting (package-level for simplicity)
var outputFormat OutputFormat = OutputFormatJSON

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
	data, err := sdk.Marshal(v, format)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err))
	}
	return mcp.NewToolResultText(string(data))
}

// MarshalledTextResult is the original function that always uses JSON.
// Kept for backward compatibility. New code should use MarshalResult.
func MarshalledTextResult(v any) *mcp.CallToolResult {
	data, _ := sdk.Marshal(v, OutputFormatJSON)
	return mcp.NewToolResultText(string(data))
}
