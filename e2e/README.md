# End To End (e2e) Tests

The purpose of the E2E tests is to provide confidence in the black box behavior of the flashduty-mcp-server artifacts. It does this by:

* Building the `flashduty-mcp-server` docker image
* Running the image
* Interacting with the server via stdio
* Issuing requests that interact with the live Flashduty API

## Running the Tests

A service must be running that supports image building and container creation via the `docker` CLI.

Since these tests require an APP key to interact with real resources on the Flashduty API, it is gated behind the `e2e` build flag.

```bash
FLASHDUTY_E2E_APP_KEY=<YOUR_APP_KEY> go test -v --tags e2e ./e2e
```

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `FLASHDUTY_E2E_APP_KEY` | Yes | APP key for authenticating with Flashduty API |
| `FLASHDUTY_E2E_BASE_URL` | No | Base URL for Flashduty API (default: `https://api.flashcat.cloud`) |
| `FLASHDUTY_E2E_DEBUG` | No | Set to any value to run tests in debug mode (in-process) |

### Debug Mode

By default, the tests build and run a Docker container to test the server as a black box. However, for easier debugging, you can run the server in-process by setting `FLASHDUTY_E2E_DEBUG`:

```bash
FLASHDUTY_E2E_APP_KEY=<YOUR_APP_KEY> FLASHDUTY_E2E_DEBUG=true go test -v --tags e2e ./e2e
```

This allows you to:
- Set breakpoints in the MCP server code
- Get better visibility into failures
- Skip the Docker build step for faster iteration

Note: Debug mode has slightly reduced coverage as it doesn't test Docker integration or cobra/viper configuration parsing.

## Test Coverage

### Basic Tests

- **TestInitialize**: Verifies server startup and tool listing
- **TestToolsets**: Tests that toolset filtering works correctly
- **TestReadOnlyMode**: Tests that read-only mode only exposes read tools

### Read-Only Tools Tests

- **TestQueryIncidents**: Tests querying incidents
- **TestQueryChannels**: Tests querying collaboration spaces
- **TestQueryMembers**: Tests querying members

### Incident Lifecycle Tests

- **TestIncidentLifecycle**: Tests the full incident lifecycle (create → acknowledge → close)

## Limitations

1. **Resource Cleanup**: Unlike GitHub's API, Flashduty doesn't support deleting incidents or collaboration spaces. Tests that create resources will close them instead of deleting them.

2. **Test Account**: It's recommended to use a dedicated test APP key to avoid polluting production data.

3. **Rate Limiting**: Be aware of Flashduty API rate limits when running tests frequently.

4. **Global State**: Some operations may affect the global state of your Flashduty account. Use with caution on production accounts.

## CI Integration

The e2e tests can be run in GitHub Actions via the `e2e.yml` workflow.

### Setup

1. Add the following secrets to your repository:
   - `FLASHDUTY_E2E_APP_KEY`: Your Flashduty APP key for testing
   - `FLASHDUTY_E2E_BASE_URL` (optional): Custom API base URL

2. Optionally, set the repository variable `ENABLE_E2E_TESTS=true` to enable automatic runs.

3. Run the workflow manually from the Actions tab, or trigger it via the GitHub API.

### Manual Trigger

You can trigger the workflow manually with debug mode:

```bash
gh workflow run e2e.yml -f debug=true
```

## Adding New Tests

When adding new tests:

1. Use the `t.Parallel()` directive for tests that can run concurrently
2. Use `callTool()` helper for making tool calls
3. Use `unmarshalToolResponse()` for parsing responses
4. For tests that create resources, ensure proper cleanup in `t.Cleanup()`
5. Consider using the native API client (`getAPIClient()`) for verification
