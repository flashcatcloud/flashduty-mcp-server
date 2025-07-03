package flashduty

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createMCPRequest is a helper function to create a MCP request with the given arguments.
func createMCPRequest(args any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: struct {
			Name      string    `json:"name"`
			Arguments any       `json:"arguments,omitempty"`
			Meta      *mcp.Meta `json:"_meta,omitempty"`
		}{
			Arguments: args,
		},
	}
}

// getTextResult is a helper function that returns a text result from a tool call.
func getTextResult(t *testing.T, result *mcp.CallToolResult) mcp.TextContent {
	t.Helper()
	assert.NotNil(t, result)
	require.Len(t, result.Content, 1)
	require.IsType(t, mcp.TextContent{}, result.Content[0])
	textContent := result.Content[0].(mcp.TextContent)
	assert.Equal(t, "text", textContent.Type)
	return textContent
}

func TestMemberInfos(t *testing.T) {
	responses := map[string]interface{}{
		"/person/infos": map[string]interface{}{
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"person_id": 1, "person_name": "John Doe", "avatar": "https://example.com/avatar1.png", "as": "member"},
					map[string]interface{}{"person_id": 2, "person_name": "Jane Smith", "avatar": "https://example.com/avatar2.png", "as": "account"},
				},
			},
		},
	}
	getClient, translator := testSetup(t, responses)

	tool, handler := MemberInfos(getClient, translator)
	assert.Equal(t, "flashduty_member_infos", tool.Name)

	request := createMCPRequest(map[string]interface{}{
		"person_ids": "1,2",
	})
	ctx := context.Background()
	result, err := handler(ctx, request)

	assert.NoError(t, err)
	textResult := getTextResult(t, result)
	assert.Contains(t, textResult.Text, "John Doe")
	assert.Contains(t, textResult.Text, "Jane Smith")
}
