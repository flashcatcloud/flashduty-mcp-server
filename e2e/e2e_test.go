//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/flashcatcloud/flashduty-mcp-server/internal/flashduty"
	pkgflashduty "github.com/flashcatcloud/flashduty-mcp-server/pkg/flashduty"
	"github.com/flashcatcloud/flashduty-mcp-server/pkg/translations"
	mcpClient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

var (
	// Shared variables and sync.Once instances to ensure one-time execution
	getAppKeyOnce sync.Once
	appKey        string

	getBaseURLOnce sync.Once
	baseURL        string

	buildOnce  sync.Once
	buildError error
)

// getE2EAppKey ensures the environment variable is checked only once and returns the app key
func getE2EAppKey(t *testing.T) string {
	getAppKeyOnce.Do(func() {
		appKey = os.Getenv("FLASHDUTY_E2E_APP_KEY")
		if appKey == "" {
			t.Fatalf("FLASHDUTY_E2E_APP_KEY environment variable is not set")
		}
	})
	return appKey
}

// getE2EBaseURL ensures the environment variable is checked only once and returns the base URL
func getE2EBaseURL() string {
	getBaseURLOnce.Do(func() {
		baseURL = os.Getenv("FLASHDUTY_E2E_BASE_URL")
		if baseURL == "" {
			baseURL = "https://api.flashcat.cloud"
		}
	})
	return baseURL
}

// getAPIClient creates a native Flashduty API client for verification purposes
func getAPIClient(t *testing.T) *pkgflashduty.Client {
	appKey := getE2EAppKey(t)
	baseURL := getE2EBaseURL()

	client, err := pkgflashduty.NewClient(appKey, baseURL, "e2e-test-client/1.0.0")
	require.NoError(t, err, "expected to create Flashduty API client")

	return client
}

// ensureDockerImageBuilt makes sure the Docker image is built only once across all tests
func ensureDockerImageBuilt(t *testing.T) {
	buildOnce.Do(func() {
		t.Log("Building Docker image for e2e tests...")
		cmd := exec.Command("docker", "build", "-t", "flashcat/e2e-flashduty-mcp-server", ".")
		cmd.Dir = ".." // Run this in the context of the root, where the Dockerfile is located.
		output, err := cmd.CombinedOutput()
		buildError = err
		if err != nil {
			t.Logf("Docker build output: %s", string(output))
		}
	})

	// Check if the build was successful
	require.NoError(t, buildError, "expected to build Docker image successfully")
}

// clientOpts holds configuration options for the MCP client setup
type clientOpts struct {
	// Toolsets to enable in the MCP server
	enabledToolsets []string
	// ReadOnly indicates if only read-only tools should be enabled
	readOnly bool
}

// clientOption defines a function type for configuring ClientOpts
type clientOption func(*clientOpts)

// withToolsets returns an option that sets the toolsets to enable
func withToolsets(toolsets []string) clientOption {
	return func(opts *clientOpts) {
		opts.enabledToolsets = toolsets
	}
}

// withReadOnly returns an option that sets the read-only mode
func withReadOnly(readOnly bool) clientOption {
	return func(opts *clientOpts) {
		opts.readOnly = readOnly
	}
}

func setupMCPClient(t *testing.T, options ...clientOption) *mcpClient.Client {
	// Get app key
	appKey := getE2EAppKey(t)
	baseURL := getE2EBaseURL()

	// Create and configure options
	opts := &clientOpts{}

	// Apply all options to configure the opts struct
	for _, option := range options {
		option(opts)
	}

	// By default, we run the tests including the Docker image, but with DEBUG
	// enabled, we run the server in-process, allowing for easier debugging.
	var client *mcpClient.Client
	if os.Getenv("FLASHDUTY_E2E_DEBUG") == "" {
		ensureDockerImageBuilt(t)

		// Prepare Docker arguments
		args := []string{
			"docker",
			"run",
			"-i",
			"--rm",
			"-e",
			"FLASHDUTY_APP_KEY",
			"-e",
			"FLASHDUTY_BASE_URL",
		}

		// Add toolsets environment variable to the Docker arguments
		if len(opts.enabledToolsets) > 0 {
			args = append(args, "-e", "FLASHDUTY_TOOLSETS")
		}

		// Add read-only environment variable
		if opts.readOnly {
			args = append(args, "-e", "FLASHDUTY_READ_ONLY")
		}

		// Add the image name
		args = append(args, "flashcat/e2e-flashduty-mcp-server")

		// Construct the env vars for the MCP Client to execute docker with
		dockerEnvVars := []string{
			fmt.Sprintf("FLASHDUTY_APP_KEY=%s", appKey),
			fmt.Sprintf("FLASHDUTY_BASE_URL=%s", baseURL),
		}

		if len(opts.enabledToolsets) > 0 {
			toolsetsStr := ""
			for i, ts := range opts.enabledToolsets {
				if i > 0 {
					toolsetsStr += ","
				}
				toolsetsStr += ts
			}
			dockerEnvVars = append(dockerEnvVars, fmt.Sprintf("FLASHDUTY_TOOLSETS=%s", toolsetsStr))
		}

		if opts.readOnly {
			dockerEnvVars = append(dockerEnvVars, "FLASHDUTY_READ_ONLY=true")
		}

		// Create the client
		t.Log("Starting Stdio MCP client...")
		var err error
		client, err = mcpClient.NewStdioMCPClient(args[0], dockerEnvVars, args[1:]...)
		require.NoError(t, err, "expected to create client successfully")
	} else {
		// Debug mode: run server in-process
		enabledToolsets := opts.enabledToolsets
		if len(enabledToolsets) == 0 {
			enabledToolsets = pkgflashduty.DefaultTools
		}

		flashdutyServer, err := flashduty.NewMCPServer(flashduty.FlashdutyConfig{
			Version:         "e2e-test",
			BaseURL:         baseURL,
			APPKey:          appKey,
			EnabledToolsets: enabledToolsets,
			ReadOnly:        opts.readOnly,
			Translator:      translations.NullTranslationHelper,
		})
		require.NoError(t, err, "expected to construct MCP server successfully")

		t.Log("Starting In Process MCP client...")
		client, err = mcpClient.NewInProcessClient(flashdutyServer)
		require.NoError(t, err, "expected to create in-process client successfully")
	}

	t.Cleanup(func() {
		require.NoError(t, client.Close(), "expected to close client successfully")
	})

	// Initialize the client
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	request := mcp.InitializeRequest{}
	request.Params.ProtocolVersion = "2025-03-26"
	request.Params.ClientInfo = mcp.Implementation{
		Name:    "e2e-test-client",
		Version: "0.0.1",
	}

	result, err := client.Initialize(ctx, request)
	require.NoError(t, err, "failed to initialize client")
	require.Equal(t, "flashduty-mcp-server", result.ServerInfo.Name, "unexpected server name")

	return client
}

// callTool is a helper function that calls a tool and returns the text content
func callTool(t *testing.T, client *mcpClient.Client, toolName string, args map[string]any) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	request := mcp.CallToolRequest{}
	request.Params.Name = toolName
	request.Params.Arguments = args

	response, err := client.CallTool(ctx, request)
	require.NoError(t, err, "expected to call '%s' tool successfully", toolName)

	if response.IsError {
		// Extract error message from response
		if len(response.Content) > 0 {
			if textContent, ok := response.Content[0].(mcp.TextContent); ok {
				t.Fatalf("tool '%s' returned error: %s", toolName, textContent.Text)
			}
		}
		t.Fatalf("tool '%s' returned error", toolName)
	}

	require.Len(t, response.Content, 1, "expected content to have one item")

	textContent, ok := response.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected content to be of type TextContent")

	return textContent.Text
}

// unmarshalToolResponse unmarshals the tool response text into the given interface
func unmarshalToolResponse(t *testing.T, text string, v interface{}) {
	err := json.Unmarshal([]byte(text), v)
	require.NoError(t, err, "expected to unmarshal tool response successfully")
}

// TestInitialize tests that the MCP server can be initialized successfully
func TestInitialize(t *testing.T) {
	t.Parallel()

	mcpClient := setupMCPClient(t)

	// The client is already initialized in setupMCPClient, so we just need to verify it works
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// List tools to verify the server is working
	request := mcp.ListToolsRequest{}
	response, err := mcpClient.ListTools(ctx, request)
	require.NoError(t, err, "expected to list tools successfully")
	require.NotEmpty(t, response.Tools, "expected to find at least one tool")

	t.Logf("Found %d tools", len(response.Tools))
}

// TestToolsets tests that toolset filtering works correctly
func TestToolsets(t *testing.T) {
	t.Parallel()

	// Test with only incidents toolset enabled
	mcpClient := setupMCPClient(t, withToolsets([]string{"incidents"}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request := mcp.ListToolsRequest{}
	response, err := mcpClient.ListTools(ctx, request)
	require.NoError(t, err, "expected to list tools successfully")

	// Check that incident tools exist
	toolNames := make(map[string]bool)
	for _, tool := range response.Tools {
		toolNames[tool.Name] = true
	}

	require.True(t, toolNames["query_incidents"], "expected to find 'query_incidents' tool")
	require.False(t, toolNames["query_channels"], "expected not to find 'query_channels' tool when only incidents toolset is enabled")
}

// TestReadOnlyMode tests that read-only mode only exposes read tools
func TestReadOnlyMode(t *testing.T) {
	t.Parallel()

	mcpClient := setupMCPClient(t, withReadOnly(true))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request := mcp.ListToolsRequest{}
	response, err := mcpClient.ListTools(ctx, request)
	require.NoError(t, err, "expected to list tools successfully")

	// Check that read-only tools exist but write tools don't
	toolNames := make(map[string]bool)
	for _, tool := range response.Tools {
		toolNames[tool.Name] = true
	}

	// Read tools should exist
	require.True(t, toolNames["query_incidents"], "expected to find 'query_incidents' tool")
	require.True(t, toolNames["query_channels"], "expected to find 'query_channels' tool")

	// Write tools should not exist in read-only mode
	require.False(t, toolNames["create_incident"], "expected not to find 'create_incident' tool in read-only mode")
	require.False(t, toolNames["ack_incident"], "expected not to find 'ack_incident' tool in read-only mode")
	require.False(t, toolNames["close_incident"], "expected not to find 'close_incident' tool in read-only mode")
}

// ============================================================================
// Read-Only Tool Tests
// ============================================================================

// TestQueryChannels tests the query_channels tool
func TestQueryChannels(t *testing.T) {
	t.Parallel()

	mcpClient := setupMCPClient(t)

	t.Log("Querying channels...")
	responseText := callTool(t, mcpClient, "query_channels", nil)

	var result struct {
		Channels []struct {
			ChannelID   int64  `json:"channel_id"`
			ChannelName string `json:"channel_name"`
			TeamID      int64  `json:"team_id,omitempty"`
			TeamName    string `json:"team_name,omitempty"`
		} `json:"channels"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, responseText, &result)

	t.Logf("Found %d channels", result.Total)

	// Verify response structure
	require.NotNil(t, result.Channels, "expected channels array to exist")
	require.GreaterOrEqual(t, result.Total, 0, "expected total to be non-negative")

	// If there are channels, verify each has required fields
	for _, ch := range result.Channels {
		require.NotZero(t, ch.ChannelID, "expected channel to have an ID")
		require.NotEmpty(t, ch.ChannelName, "expected channel to have a name")
	}
}

// TestQueryMembers tests the query_members tool
func TestQueryMembers(t *testing.T) {
	t.Parallel()

	mcpClient := setupMCPClient(t)

	t.Log("Querying members...")
	responseText := callTool(t, mcpClient, "query_members", nil)

	var result struct {
		Members []struct {
			MemberID   int    `json:"member_id"`
			MemberName string `json:"member_name"`
			Email      string `json:"email,omitempty"`
			Status     string `json:"status"`
		} `json:"members"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, responseText, &result)

	t.Logf("Found %d members", result.Total)

	// Verify response structure
	require.NotNil(t, result.Members, "expected members array to exist")
	require.GreaterOrEqual(t, result.Total, 0, "expected total to be non-negative")

	// If there are members, verify each has required fields
	for _, m := range result.Members {
		require.NotZero(t, m.MemberID, "expected member to have an ID")
		require.NotEmpty(t, m.MemberName, "expected member to have a name")
	}
}

// TestQueryIncidents tests the query_incidents tool with time range
func TestQueryIncidents(t *testing.T) {
	t.Parallel()

	mcpClient := setupMCPClient(t)

	// Query incidents from the last 7 days
	now := time.Now().Unix()
	startTime := now - 7*24*60*60 // 7 days ago

	t.Log("Querying incidents from the last 7 days...")
	responseText := callTool(t, mcpClient, "query_incidents", map[string]any{
		"start_time":     startTime,
		"end_time":       now,
		"limit":          10,
		"include_alerts": false,
	})

	var result struct {
		Incidents []struct {
			IncidentID  string `json:"incident_id"`
			Title       string `json:"title"`
			Severity    string `json:"severity"`
			Progress    string `json:"progress"`
			ChannelID   int64  `json:"channel_id"`
			ChannelName string `json:"channel_name,omitempty"`
			CreatedAt   int64  `json:"created_at"`
			AlertsTotal int    `json:"alerts_total,omitempty"`
		} `json:"incidents"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, responseText, &result)

	t.Logf("Found %d incidents", result.Total)

	// Verify response structure
	require.NotNil(t, result.Incidents, "expected incidents array to exist")
	require.GreaterOrEqual(t, result.Total, 0, "expected total to be non-negative")

	// If there are incidents, verify each has required fields
	for _, inc := range result.Incidents {
		require.NotEmpty(t, inc.IncidentID, "expected incident to have an ID")
		require.NotEmpty(t, inc.Title, "expected incident to have a title")
		require.NotEmpty(t, inc.Progress, "expected incident to have progress status")
	}
}

// TestQueryTeams tests the query_teams tool
func TestQueryTeams(t *testing.T) {
	t.Parallel()

	mcpClient := setupMCPClient(t)

	t.Log("Querying teams...")
	responseText := callTool(t, mcpClient, "query_teams", nil)

	var result struct {
		Teams []struct {
			TeamID   int64  `json:"team_id"`
			TeamName string `json:"team_name"`
			Members  []struct {
				PersonID   int64  `json:"person_id"`
				PersonName string `json:"person_name"`
				Email      string `json:"email,omitempty"`
			} `json:"members,omitempty"`
		} `json:"teams"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, responseText, &result)

	t.Logf("Found %d teams", result.Total)

	// Verify response structure
	require.NotNil(t, result.Teams, "expected teams array to exist")
	require.GreaterOrEqual(t, result.Total, 0, "expected total to be non-negative")

	// If there are teams, verify each has required fields
	for _, team := range result.Teams {
		require.NotZero(t, team.TeamID, "expected team to have an ID")
		require.NotEmpty(t, team.TeamName, "expected team to have a name")
	}
}

// TestQueryFields tests the query_fields tool
func TestQueryFields(t *testing.T) {
	t.Parallel()

	mcpClient := setupMCPClient(t)

	t.Log("Querying custom fields...")
	responseText := callTool(t, mcpClient, "query_fields", nil)

	var result struct {
		Fields []struct {
			FieldID      string `json:"field_id"`
			FieldName    string `json:"field_name"`
			DisplayName  string `json:"display_name"`
			FieldType    string `json:"field_type"`
			ValueType    string `json:"value_type"`
			DefaultValue any    `json:"default_value,omitempty"`
		} `json:"fields"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, responseText, &result)

	t.Logf("Found %d custom fields", result.Total)

	// Verify response structure
	require.NotNil(t, result.Fields, "expected fields array to exist")
	require.GreaterOrEqual(t, result.Total, 0, "expected total to be non-negative")
}

// TestQueryChanges tests the query_changes tool
func TestQueryChanges(t *testing.T) {
	t.Parallel()

	mcpClient := setupMCPClient(t)

	// Query changes from the last 7 days (API requires time range)
	now := time.Now().Unix()
	startTime := now - 7*24*60*60 // 7 days ago

	t.Log("Querying changes from the last 7 days...")
	responseText := callTool(t, mcpClient, "query_changes", map[string]any{
		"start_time": startTime,
		"end_time":   now,
		"limit":      10,
	})

	var result struct {
		Changes []struct {
			ChangeID    string `json:"change_id"`
			Title       string `json:"title"`
			Type        string `json:"type,omitempty"`
			ChannelID   int64  `json:"channel_id,omitempty"`
			ChannelName string `json:"channel_name,omitempty"`
			CreatedAt   int64  `json:"created_at"`
		} `json:"changes"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, responseText, &result)

	t.Logf("Found %d changes", result.Total)

	// Verify response structure
	require.NotNil(t, result.Changes, "expected changes array to exist")
	require.GreaterOrEqual(t, result.Total, 0, "expected total to be non-negative")
}

// ============================================================================
// Incident Lifecycle Tests
// ============================================================================

// TestIncidentLifecycle tests the full incident lifecycle: create -> acknowledge -> close
func TestIncidentLifecycle(t *testing.T) {
	// Don't run in parallel since this test creates and modifies resources
	mcpClient := setupMCPClient(t)

	testTitle := fmt.Sprintf("E2E Test Incident - %d", time.Now().UnixMilli())

	// Step 1: Create an incident
	t.Log("Creating a new incident...")
	createResponseText := callTool(t, mcpClient, "create_incident", map[string]any{
		"title":       testTitle,
		"severity":    "Info",
		"description": "This is an automated e2e test incident. Please ignore.",
	})

	var createResult struct {
		IncidentID string `json:"incident_id"`
	}
	unmarshalToolResponse(t, createResponseText, &createResult)

	incidentID := createResult.IncidentID
	require.NotEmpty(t, incidentID, "expected incident ID to be returned")
	t.Logf("Created incident: %s", incidentID)

	// Setup cleanup to ensure incident is closed even if test fails
	t.Cleanup(func() {
		t.Logf("Cleanup: ensuring incident %s is closed...", incidentID)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		request := mcp.CallToolRequest{}
		request.Params.Name = "close_incident"
		request.Params.Arguments = map[string]any{
			"incident_ids": incidentID,
		}

		// Ignore errors during cleanup - the incident might already be closed
		_, _ = mcpClient.CallTool(ctx, request)
	})

	// Step 2: Query the incident to verify it was created
	t.Log("Querying the created incident...")
	queryResponseText := callTool(t, mcpClient, "query_incidents", map[string]any{
		"incident_ids":   incidentID,
		"include_alerts": false,
	})

	var queryResult struct {
		Incidents []struct {
			IncidentID string `json:"incident_id"`
			Title      string `json:"title"`
			Progress   string `json:"progress"`
			Severity   string `json:"severity"`
		} `json:"incidents"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, queryResponseText, &queryResult)

	require.Equal(t, 1, queryResult.Total, "expected to find exactly one incident")
	require.Equal(t, incidentID, queryResult.Incidents[0].IncidentID, "expected incident ID to match")
	require.Equal(t, testTitle, queryResult.Incidents[0].Title, "expected title to match")
	require.Equal(t, "Triggered", queryResult.Incidents[0].Progress, "expected progress to be Triggered")

	// Step 3: Acknowledge the incident
	t.Log("Acknowledging the incident...")
	ackResponseText := callTool(t, mcpClient, "ack_incident", map[string]any{
		"incident_ids": incidentID,
	})

	var ackResult struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	unmarshalToolResponse(t, ackResponseText, &ackResult)

	require.Equal(t, "success", ackResult.Status, "expected ack to succeed")
	t.Logf("Acknowledged incident: %s", ackResult.Message)

	// Step 4: Verify the incident is now in Processing state
	t.Log("Verifying incident is in Processing state...")
	queryResponseText = callTool(t, mcpClient, "query_incidents", map[string]any{
		"incident_ids":   incidentID,
		"include_alerts": false,
	})
	unmarshalToolResponse(t, queryResponseText, &queryResult)

	require.Equal(t, 1, queryResult.Total, "expected to find exactly one incident")
	require.Equal(t, "Processing", queryResult.Incidents[0].Progress, "expected progress to be Processing after ack")

	// Step 5: Close the incident
	t.Log("Closing the incident...")
	closeResponseText := callTool(t, mcpClient, "close_incident", map[string]any{
		"incident_ids": incidentID,
	})

	var closeResult struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	unmarshalToolResponse(t, closeResponseText, &closeResult)

	require.Equal(t, "success", closeResult.Status, "expected close to succeed")
	t.Logf("Closed incident: %s", closeResult.Message)

	// Step 6: Verify the incident is now Closed
	t.Log("Verifying incident is Closed...")
	queryResponseText = callTool(t, mcpClient, "query_incidents", map[string]any{
		"incident_ids":   incidentID,
		"include_alerts": false,
	})
	unmarshalToolResponse(t, queryResponseText, &queryResult)

	require.Equal(t, 1, queryResult.Total, "expected to find exactly one incident")
	require.Equal(t, "Closed", queryResult.Incidents[0].Progress, "expected progress to be Closed")

	t.Logf("Incident lifecycle test completed successfully: %s", incidentID)
}

// TestIncidentQueryByTimeline tests querying incident timeline
func TestIncidentQueryByTimeline(t *testing.T) {
	// This test requires an existing incident, so we create one first
	mcpClient := setupMCPClient(t)

	testTitle := fmt.Sprintf("E2E Timeline Test - %d", time.Now().UnixMilli())

	// Create an incident
	t.Log("Creating a test incident for timeline query...")
	createResponseText := callTool(t, mcpClient, "create_incident", map[string]any{
		"title":       testTitle,
		"severity":    "Info",
		"description": "Timeline test incident",
	})

	var createResult struct {
		IncidentID string `json:"incident_id"`
	}
	unmarshalToolResponse(t, createResponseText, &createResult)

	incidentID := createResult.IncidentID
	require.NotEmpty(t, incidentID, "expected incident ID to be returned")

	// Cleanup
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		request := mcp.CallToolRequest{}
		request.Params.Name = "close_incident"
		request.Params.Arguments = map[string]any{
			"incident_ids": incidentID,
		}
		_, _ = mcpClient.CallTool(ctx, request)
	})

	// Query timeline
	t.Log("Querying incident timeline...")
	timelineResponseText := callTool(t, mcpClient, "query_incident_timeline", map[string]any{
		"incident_ids": incidentID,
	})

	var timelineResult struct {
		Results []struct {
			IncidentID string `json:"incident_id"`
			Timeline   []struct {
				Type      string `json:"type"`
				Timestamp int64  `json:"timestamp"`
				Detail    any    `json:"detail,omitempty"`
			} `json:"timeline"`
			Total int `json:"total"`
		} `json:"results"`
	}
	unmarshalToolResponse(t, timelineResponseText, &timelineResult)

	// The incident should have at least one timeline event (the creation event)
	require.NotEmpty(t, timelineResult.Results, "expected results array to exist")

	var timeline []struct {
		Type      string `json:"type"`
		Timestamp int64  `json:"timestamp"`
		Detail    any    `json:"detail,omitempty"`
	}
	for _, r := range timelineResult.Results {
		if r.IncidentID == incidentID {
			timeline = r.Timeline
			break
		}
	}
	require.NotEmpty(t, timeline, "expected at least one timeline event")

	t.Logf("Found %d timeline events", len(timeline))

	// Verify the first event is the creation event
	hasCreationEvent := false
	for _, event := range timeline {
		if event.Type == "i_new" {
			hasCreationEvent = true
			break
		}
	}
	require.True(t, hasCreationEvent, "expected to find incident creation event (i_new)")

	// Close the incident
	_ = callTool(t, mcpClient, "close_incident", map[string]any{
		"incident_ids": incidentID,
	})
}

// TestUpdateIncident tests updating an incident
func TestUpdateIncident(t *testing.T) {
	mcpClient := setupMCPClient(t)

	testTitle := fmt.Sprintf("E2E Update Test - %d", time.Now().UnixMilli())

	// Create an incident
	t.Log("Creating a test incident for update test...")
	createResponseText := callTool(t, mcpClient, "create_incident", map[string]any{
		"title":       testTitle,
		"severity":    "Info",
		"description": "Original description",
	})

	var createResult struct {
		IncidentID string `json:"incident_id"`
	}
	unmarshalToolResponse(t, createResponseText, &createResult)

	incidentID := createResult.IncidentID
	require.NotEmpty(t, incidentID, "expected incident ID to be returned")

	// Cleanup
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		request := mcp.CallToolRequest{}
		request.Params.Name = "close_incident"
		request.Params.Arguments = map[string]any{
			"incident_ids": incidentID,
		}
		_, _ = mcpClient.CallTool(ctx, request)
	})

	// Update the incident title
	newTitle := testTitle + " - Updated"
	t.Log("Updating incident title...")
	updateResponseText := callTool(t, mcpClient, "update_incident", map[string]any{
		"incident_id": incidentID,
		"title":       newTitle,
	})

	var updateResult struct {
		Status        string   `json:"status"`
		Message       string   `json:"message"`
		UpdatedFields []string `json:"updated_fields"`
	}
	unmarshalToolResponse(t, updateResponseText, &updateResult)

	require.Equal(t, "success", updateResult.Status, "expected update to succeed")
	require.Contains(t, updateResult.UpdatedFields, "title", "expected title to be in updated fields")

	// Verify the update
	t.Log("Verifying the update...")
	queryResponseText := callTool(t, mcpClient, "query_incidents", map[string]any{
		"incident_ids":   incidentID,
		"include_alerts": false,
	})

	var queryResult struct {
		Incidents []struct {
			IncidentID string `json:"incident_id"`
			Title      string `json:"title"`
		} `json:"incidents"`
		Total int `json:"total"`
	}
	unmarshalToolResponse(t, queryResponseText, &queryResult)

	require.Equal(t, 1, queryResult.Total, "expected to find exactly one incident")
	require.Equal(t, newTitle, queryResult.Incidents[0].Title, "expected title to be updated")

	// Close the incident
	_ = callTool(t, mcpClient, "close_incident", map[string]any{
		"incident_ids": incidentID,
	})

	t.Log("Update incident test completed successfully")
}
