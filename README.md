# Flashduty MCP Server

English | [中文](README_zh.md)

The Flashduty MCP Server is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction) server that provides seamless integration with Flashduty APIs, enabling advanced incident management and automation capabilities for developers and tools.

### Use Cases

- Automating Flashduty workflows and processes.
- Extracting and analyzing data from Flashduty.
- Building AI-powered tools and applications that interact with Flashduty.

---

## Remote Flashduty MCP Server

[![Install in Cursor](https://img.shields.io/badge/Cursor-Install_Server-24bfa5?style=flat-square&logo=visualstudiocode&logoColor=white)](#remote-cursor)

The remote Flashduty MCP Server provides the easiest method for getting up and running with Flashduty integration. If your MCP host does not support remote MCP servers, you can use the [local version of the Flashduty MCP Server](#local-flashduty-mcp-server) instead.

### Prerequisites

1. An MCP host that supports the latest MCP specification and remote servers, such as [Cursor](https://www.cursor.com/).
2. A Flashduty APP key from your Flashduty account.

### Installation

<span id="remote-cursor"></span>

#### For example, with Cursor:

For Cursors that support Remote MCP, use the following configuration:

```json
{
  "mcpServers": {
    "flashduty": {
      "url": "https://mcp.flashcat.cloud/mcp",
      "headers": {
        "Authorization": "Bearer <your_flashduty_app_key>"
      }
    }
  }
}
```

> **Note:** Refer to your MCP host's documentation for the correct syntax and location for remote MCP server setup.

---

## Local Flashduty MCP Server

[![Install with Docker in Cursor](https://img.shields.io/badge/Cursor-Install_Server-24bfa5?style=flat-square&logo=visualstudiocode&logoColor=white)](#local-cursor)

### Prerequisites

1. To run the server in a container, you will need to have [Docker](https://www.docker.com/) installed and running.
2. You will need a Flashduty APP key from your Flashduty account.

### Installation

<span id="local-cursor"></span>

#### For example, with Cursor:

#### Using Docker

Add the following JSON block to your Cursor MCP configuration.

```json
{
  "mcpServers": {
    "flashduty": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e",
        "FLASHDUTY_APP_KEY",
        "registry.flashcat.cloud/public/flashduty-mcp-server"
      ],
      "env": {
        "FLASHDUTY_APP_KEY": "your_flashduty_app_key"
      }
    }
  }
}
```

#### Using binary

Besides building from source, you can also download a pre-compiled version for your operating system directly from the project's [GitHub Releases](https://github.com/flashcatcloud/flashduty-mcp-server/releases), which is a faster and more convenient option.

If you prefer to build from source, you can use `go build` to build the binary in the `cmd/flashduty-mcp-server` directory. You can provide the APP key either via environment variable or command-line argument.

You should configure your MCP host to use the built executable as its `command`. For example:

**Via Environment Variable:**
```json
{
  "mcpServers": {
    "flashduty": {
      "command": "/path/to/flashduty-mcp-server",
      "args": ["stdio"],
      "env": {
        "FLASHDUTY_APP_KEY": "your_app_key_here"
      }
    }
  }
}
```

**Via Command-line Argument:**
```json
{
  "mcpServers": {
    "flashduty": {
      "command": "/path/to/flashduty-mcp-server",
      "args": ["stdio", "--app-key", "your_app_key_here"]
    }
  }
}
```

---

## Tool Configuration

The Flashduty MCP Server supports several configuration options for different use cases. The main options include:

- **Toolsets**: Allows you to enable or disable specific groups of functionalities. Enabling only the toolsets you need can help the LLM with tool choice and reduce the context size.
- **Read-Only Mode**: Restricts the server to read-only operations, preventing any modifications and enhancing security.
- **i18n**: Supports customizing tool descriptions to suit different languages or team preferences.

Configuration methods are divided into **Remote Service Configuration** and **Local Service Configuration**.

### Remote Server Configuration

When using the public remote service (`https://mcp.flashcat.cloud/mcp`), you can dynamically configure it by appending query parameters to the URL.

#### Configuration Example

Here is an example of configuring the remote service, specifying toolsets and read-only mode:

```json
{
  "mcpServers": {
    "flashduty": {
      "url": "https://mcp.flashcat.cloud/mcp?toolsets=incidents,users&read_only=true",
      "headers": {
        "Authorization": "Bearer <your_flashduty_app_key>"
      }
    }
  }
}
```

- `headers.Authorization`: Your Flashduty APP key for authentication, prefixed with `Bearer `.
- `toolsets=...`: Use a comma-separated list to specify the toolsets to enable (e.g., `incidents,users,channels`).
- `read_only=true`: Enables read-only mode.

### Local Server Configuration

When running the service locally via Docker or from source, you have full configuration control.

#### 1. Via Environment Variables

This is the most common method for local configuration, especially in a Docker environment.

| Variable | Description | Required | Default |
|---|---|---|---|
| `FLASHDUTY_APP_KEY` | Flashduty APP key | ✅ | - |
| `FLASHDUTY_TOOLSETS` | Toolsets to enable (comma-separated) | ❌ | All toolsets |
| `FLASHDUTY_READ_ONLY` | Restrict to read-only operations (`1` or `true`) | ❌ | `false` |
| `FLASHDUTY_OUTPUT_FORMAT` | Output format for tool results (`json` or `toon`) | ❌ | `json` |
| `FLASHDUTY_BASE_URL` | Flashduty API base URL | ❌ | `https://api.flashcat.cloud` |
| `FLASHDUTY_LOG_FILE` | Log file path | ❌ | stderr |
| `FLASHDUTY_ENABLE_COMMAND_LOGGING` | Enable command logging | ❌ | `false` |

**Docker Example:**

```bash
docker run -i --rm \
  -e FLASHDUTY_APP_KEY=<your-app-key> \
  -e FLASHDUTY_TOOLSETS="incidents,users,channels" \
  -e FLASHDUTY_READ_ONLY=1 \
  registry.flashcat.cloud/public/flashduty-mcp-server
```

#### 2. Via Command-Line Arguments

If you build and run the binary directly from the source, you can use command-line arguments.

```bash
./flashduty-mcp-server stdio \
  --app-key your_app_key_here \
  --toolsets incidents,users,channels \
  --read-only
```

Available command-line arguments:
- `--app-key`: Flashduty APP key (alternative to `FLASHDUTY_APP_KEY` environment variable)
- `--toolsets`: Comma-separated list of toolsets to enable
- `--read-only`: Enable read-only mode
- `--output-format`: Output format for tool results (`json` or `toon`)
- `--base-url`: Flashduty API base URL
- `--log-file`: Path to log file
- `--enable-command-logging`: Enable command logging
- `--export-translations`: Save translations to a JSON file

> Note: Command-line arguments take precedence over environment variables. For toolsets configuration, if both `FLASHDUTY_TOOLSETS` environment variable and `--toolsets` argument are set, the command-line argument takes priority.

#### 3. Output Format (TOON)

The server supports [TOON (Token-Oriented Object Notation)](https://github.com/toon-format/toon) format for tool results, which can significantly reduce token usage (30-50%) when working with LLMs.

**JSON output** (default):
```json
{"members":[{"person_id":1,"person_name":"Alice"},{"person_id":2,"person_name":"Bob"}],"total":2}
```

**TOON output** (compact):
```
members[2]{person_id,person_name}:
  1,Alice
  2,Bob
total: 2
```

To enable TOON format:

**Via Environment Variable:**
```bash
export FLASHDUTY_OUTPUT_FORMAT=toon
```

**Via Command-line:**
```bash
./flashduty-mcp-server stdio --output-format toon
```

> **Note:** TOON format is particularly effective for arrays of objects with uniform fields (e.g., member lists, incident lists). Most modern LLMs can parse TOON format naturally.

#### 4. i18n / Overriding Descriptions (Local-Only)

The feature to override tool descriptions is only available for local deployments. You can achieve this by creating a `flashduty-mcp-server-config.json` file or by setting environment variables.

**Via JSON File:**

Create `flashduty-mcp-server-config.json` in the same directory as the binary:
```json
{
  "TOOL_CREATE_INCIDENT_DESCRIPTION": "an alternative description",
  "TOOL_LIST_TEAMS_DESCRIPTION": "List all teams in Flashduty account"
}
```

**Via Environment Variables:**

```sh
export FLASHDUTY_MCP_TOOL_CREATE_INCIDENT_DESCRIPTION="an alternative description"
```

#### 5. Logging & Security

The server provides structured logging and built-in security features:

- **Data Masking**: Sensitive information such as `APP_KEY` and `Authorization` headers are automatically masked in logs to prevent accidental credential leakage.
- **Log Truncation**: Large request/response bodies are automatically truncated in logs (default 2KB) to maintain performance.
- **W3C Trace Context**: Supports W3C Trace Context (`traceparent`) for end-to-end observability. Trace IDs are automatically included in logs for easy request tracking.

---

## Available Toolsets

The following toolsets are available (all are on by default). You can also use `all` to enable all toolsets.

| Toolset        | Description                                      | Tools |
| -------------- | ------------------------------------------------ | ----- |
| `incidents`    | Incident lifecycle management                    | 6     |
| `changes`      | Change record query                              | 1     |
| `status_page`  | Status page management                           | 4     |
| `users`        | Member and team query                            | 2     |
| `channels`     | Collaboration space and escalation rules         | 2     |
| `fields`       | Custom field definitions                         | 1     |

**Total: 16 tools**

---

## Tools

### `incidents` - Incident Lifecycle Management (6 tools)
- `query_incidents` - Query incidents with enriched data (timeline, alerts, responders)
- `create_incident` - Create a new incident
- `update_incident` - Update incident (title, description, severity, custom_fields)
- `ack_incident` - Acknowledge incidents
- `close_incident` - Close (resolve) incidents
- `list_similar_incidents` - Find similar historical incidents

### `changes` - Change Record Query (1 tool)
- `query_changes` - Query change records with filters

### `status_page` - Status Page Management (4 tools)
- `query_status_pages` - Query status pages with full configuration
- `list_status_changes` - List change events on status page
- `create_status_incident` - Create incident on status page
- `create_change_timeline` - Add timeline update to status change

### `users` - Member and Team Query (2 tools)
- `query_members` - Query members with optional filters
- `query_teams` - Query teams with member details

### `channels` - Collaboration Space and Escalation Rules (2 tools)
- `query_channels` - Query collaboration spaces with enriched data (team and creator names)
- `query_escalation_rules` - Query escalation rules for a channel

### `fields` - Custom Field Definitions (1 tool)
- `query_fields` - Query custom field definitions

---

## Library Usage

The exported Go API of this module should currently be considered unstable, and subject to breaking changes. In the future, we may offer stability; please file an issue if there is a use case where this would be valuable.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.