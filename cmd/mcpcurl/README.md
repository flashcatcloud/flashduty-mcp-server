# mcpcurl

A CLI tool that dynamically builds commands based on schemas retrieved from MCP servers that can
be executed against the configured MCP server.

## Overview

`mcpcurl` is a command-line interface that:

1. Connects to an MCP server via stdio
2. Dynamically retrieves the available tools schema
3. Generates CLI commands corresponding to each tool
4. Handles parameter validation based on the schema
5. Executes commands and displays responses

## Installation

## Usage

```console
mcpcurl --stdio-server-cmd="<command to start MCP server>" <command> [flags]
```

The `--stdio-server-cmd` flag is required for all commands and specifies the command to run the MCP server.

### Available Commands

- `tools`: Contains all dynamically generated tool commands from the schema
- `schema`: Fetches and displays the raw schema from the MCP server
- `help`: Shows help for any command

### Examples

List available tools in FlashDuty's MCP server:

```console
% ./mcpcurl --stdio-server-cmd "docker run -i --rm -e FLASHDUTY_APP_KEY=<your_app_key> flashcat.tencentcloudcr.com/flashduty/flashduty-mcp-server" tools --help
Contains all dynamically generated tool commands from the schema

Usage:
  mcpcurl tools [command]

Available Commands:
  flashduty_ack_incident                Acknowledge incidents
  flashduty_add_responder               Add responders to incidents
  flashduty_assign_incident             Assign incidents to people or escalation rules
  flashduty_channels_infos              Get collaboration space information by channel IDs
  flashduty_comment_incident            Add comments to incidents
  flashduty_create_incident             Create a new incident
  flashduty_get_incident_alerts         Get alerts associated with incidents
  flashduty_get_incident_timeline       Get incident timeline and feed
  flashduty_incidents_infos             Get incident information by incident IDs
  flashduty_list_incidents              List incidents with comprehensive filters
  flashduty_list_past_incidents         List similar historical incidents
  flashduty_member_infos                Get member information by person IDs
  flashduty_merge_incident              Merge multiple incidents into one
  flashduty_resolve_incident            Resolve incidents
  flashduty_snooze_incident             Snooze incidents for a period
  flashduty_teams_infos                 Get team information by team IDs
  flashduty_update_incident_description Update incident description
  flashduty_update_incident_fields      Update custom fields
  flashduty_update_incident_impact      Update incident impact
  flashduty_update_incident_resolution  Update incident resolution
  flashduty_update_incident_root_cause  Update incident root cause
  flashduty_update_incident_severity    Update incident severity
  flashduty_update_incident_title       Update incident title

Flags:
  -h, --help   help for tools

Global Flags:
      --pretty                    Pretty print MCP response (only for JSON or JSONL responses) (default true)
      --stdio-server-cmd string   Shell command to invoke MCP server via stdio (required)

Use "mcpcurl tools [command] --help" for more information about a command.
```

Get help for a specific tool:

```console
 % ./mcpcurl --stdio-server-cmd "docker run -i --rm -e FLASHDUTY_APP_KEY=<your_app_key> flashcat.tencentcloudcr.com/flashduty/flashduty-mcp-server" tools flashduty_incidents_infos --help
Get incident information by incident IDs

Usage:
  mcpcurl tools flashduty_incidents_infos [flags]

Flags:
  -h, --help                  help for flashduty_incidents_infos
      --incident_ids string   Comma-separated list of incident IDs to get information for. Example: 'id1,id2,id3'

Global Flags:
      --pretty                    Pretty print MCP response (only for JSON or JSONL responses) (default true)
      --stdio-server-cmd string   Shell command to invoke MCP server via stdio (required)
```

Use one of the tools:

```console
 % ./mcpcurl --stdio-server-cmd "docker run -i --rm -e FLASHDUTY_APP_KEY=<your_app_key> flashcat.tencentcloudcr.com/flashduty/flashduty-mcp-server" tools flashduty_list_incidents --start_time=1701388800 --end_time=1704067199 --limit=2
{
  "has_next_page": true,
  "items": [
    {
      "incident_id": "68649b141afa719eeb65ec2c",
      "title": "Mysql连接数已超过80% / Default",
      "progress": "Closed",
      "incident_severity": "Warning",
      "created_at": 1751423764,
      "channel_name": "Screen_Monitor_Demo"
    },
    {
      "incident_id": "68649a41cf0919676e541493",
      "title": "Redis 集群->Cluster-01->master :  10.201.0.210:6379 / 连续飘红1次",
      "progress": "Closed",
      "incident_severity": "Warning",
      "created_at": 1751423553,
      "channel_name": "Screen_Monitor_Demo"
    }
  ],
  "total": 1000
}
```

## Dynamic Commands

All tools provided by the MCP server are automatically available as subcommands under the `tools` command. Each generated command has:

- Appropriate flags matching the tool's input schema
- Validation for required parameters
- Type validation
- Enum validation (for string parameters with allowable values)
- Help text generated from the tool's description

## How It Works

1. `mcpcurl` makes a JSON-RPC request to the server using the `tools/list` method
2. The server responds with a schema describing all available tools
3. `mcpcurl` dynamically builds a command structure based on this schema
4. When a command is executed, arguments are converted to a JSON-RPC request
5. The request is sent to the server via stdin, and the response is printed to stdout
