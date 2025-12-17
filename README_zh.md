# Flashduty MCP 服务

中文 | [English](README.md)

Flashduty MCP 服务是一个 [模型上下文协议 (MCP)](https://modelcontextprotocol.io/introduction) 服务，它提供了与 Flashduty API 的无缝集成，为开发人员和工具提供了高级的故障管理和自动化功能。

### 使用场景

- 自动化 Flashduty 工作流和流程。
- 从 Flashduty 提取和分析数据。
- 构建与 Flashduty 交互的 AI 驱动的工具和应用程序。

---

## 远程 Flashduty MCP 服务

[![在 Cursor 中安装](https://img.shields.io/badge/Cursor-Install_Server-24bfa5?style=flat-square&logo=visualstudiocode&logoColor=white)](#remote-cursor)

远程 Flashduty MCP 服务提供了与 Flashduty 集成的最简单方法。如果您的 MCP 主机不支持远程 MCP 服务，您可以改用[本地版本的 Flashduty MCP 服务](#local-flashduty-mcp-server)。

## 先决条件

1. 支持最新 MCP 规范和远程服务的 MCP 主机，例如 [Cursor](https://www.cursor.com/)。
2. 来自您 Flashduty 账户的 Flashduty APP 密钥。

## 安装

<span id="remote-cursor"></span>

以 Cursor 为例：

```json
{
  "mcpServers": {
    "flashduty": {
      "url": "https://mcp.flashcat.cloud/flashduty?toolsets=flashduty_incidents,flashduty_teams&read_only=true",
      "authorization_token": "Bearer <your_flashduty_app_key>"
    }
  }
}
```

> **注意：** 有关远程 MCP 服务设置的正确语法和位置，请参阅 Cursor 的文档。

---

## 本地 Flashduty MCP 服务

[![在 Cursor 中使用 Docker 安装](https://img.shields.io/badge/Cursor-Install_Server-24bfa5?style=flat-square&logo=visualstudiocode&logoColor=white)](#local-cursor)

## 先决条件

1. 要在容器中运行该服务，您需要安装 [Docker](https://www.docker.com/)。
2. 安装 Docker 后，您还需要确保 Docker 正在运行。
3. 最后，您需要从您的 Flashduty 帐户获取一个 Flashduty APP 密钥。

## 安装

<span id="local-cursor"></span>

以 Cursor 为例：

### 使用 Docker

将以下 JSON 块添加到您的 Cursor MCP 配置中。

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

### 使用二进制

除了通过源码构建，您也可以直接从本项目的 [GitHub Releases](https://github.com/flashcatcloud/flashduty-mcp-server/releases) 页面下载适用于您操作系统的预编译版本，这是一个更快捷方便的选项。

如果您没有 Docker，您可以使用 `go build` 在 `cmd/flashtudy-mcp-server` 目录中构建二进制文件。您可以通过环境变量或命令行参数提供 APP 密钥。

您应该配置 Cursor 使用构建的可执行文件作为其 `command`。例如：

**通过环境变量:**
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

**通过命令行参数:**
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

> **注意：** 有关 MCP 服务设置的正确语法和位置，请参阅 Cursor 的文档。

---

## 工具配置

Flashduty MCP 服务支持多种配置选项，以满足不同的使用场景。主要配置项包括：

- **工具集 (Toolsets)**: 允许您启用或禁用特定的功能组。仅启用需要的工具集可以帮助 LLM 更准确地选择工具，并减小上下文大小。
- **只读模式 (Read-Only)**: 将服务限制在只读模式，禁止任何修改性操作，增强安全性。
- **国际化 (i18n)**: 支持自定义工具的描述，以适应不同的语言或团队偏好。

配置方式主要分为 **远程服务配置** 和 **本地服务配置** 两种。

### 远程工具配置

当您使用公共远程服务 (`https://mcp.flashcat.cloud/flashduty`) 时，可以通过在 URL 中附加查询参数来动态配置服务。

#### 配置示例

以下是在 Cursor 中配置远程服务并指定工具集和只读模式的示例：

```json
{
  "mcpServers": {
    "flashduty": {
      "url": "https://mcp.flashcat.cloud/flashduty?toolsets=flashduty_incidents,flashduty_teams&read_only=true",
      "authorization_token": "Bearer <your_flashduty_app_key>"
    }
  }
}
```

- `toolsets=...`: 使用逗号分隔的列表来指定要启用的工具集。
- `read_only=true`: 启用只读模式。

### 本地工具配置

当您通过 Docker 或源码在本地运行服务时，拥有完全的配置权限。

#### 1. 通过环境变量配置

这是最常见的本地配置方式，尤其是在 Docker 环境中。

| 变量 | 描述 | 必需 | 默认值 |
|---|---|---|---|
| `FLASHDUTY_APP_KEY` | Flashduty APP 密钥 | ✅ | - |
| `FLASHDUTY_TOOLSETS` | 要启用的工具集（逗号分隔） | ❌ | 所有工具集 |
| `FLASHDUTY_READ_ONLY` | 限制为只读操作 (`1` 或 `true`) | ❌ | `false` |
| `FLASHDUTY_BASE_URL` | Flashduty API 基础 URL | ❌ | `https://api.flashcat.cloud` |
| `FLASHDUTY_LOG_FILE` | 日志文件路径 | ❌ | stderr |
| `FLASHDUTY_ENABLE_COMMAND_LOGGING` | 启用命令日志记录 | ❌ | `false` |

**Docker 示例:**

```bash
docker run -i --rm \
  -e FLASHDUTY_APP_KEY=<your-app-key> \
  -e FLASHDUTY_TOOLSETS="flashduty_incidents,flashduty_teams" \
  -e FLASHDUTY_READ_ONLY=1 \
  registry.flashcat.cloud/public/flashduty-mcp-server
```

#### 2. 通过命令行参数配置

如果您直接从源码构建和运行二进制文件，可以使用命令行参数。

```bash
./flashduty-mcp-server stdio \
  --app-key your_app_key_here \
  --toolsets flashduty_incidents,flashduty_teams \
  --read-only
```

可用的命令行参数：
- `--app-key`: Flashduty APP 密钥（替代 `FLASHDUTY_APP_KEY` 环境变量）
- `--toolsets`: 要启用的工具集（逗号分隔）
- `--read-only`: 启用只读模式
- `--base-url`: Flashduty API 基础 URL
- `--log-file`: 日志文件路径
- `--enable-command-logging`: 启用命令日志记录
- `--export-translations`: 将翻译保存到 JSON 文件

> 注意：命令行参数的优先级高于环境变量。对于工具集配置，如果同时设置了 `FLASHDUTY_TOOLSETS` 环境变量和 `--toolsets` 参数，命令行参数优先生效。

#### 3. 国际化 / 覆盖描述 (仅限本地)

覆盖工具描述的功能仅在本地部署时可用。您可以通过创建 `flashduty-mcp-server-config.json` 文件或设置环境变量来实现。

**通过 JSON 文件:**

在二进制文件同目录下创建 `flashduty-mcp-server-config.json`：
```json
{
  "TOOL_CREATE_INCIDENT_DESCRIPTION": "an alternative description",
  "TOOL_LIST_TEAMS_DESCRIPTION": "List all teams in Flashduty account"
}
```

**通过环境变量:**

```sh
export FLASHDUTY_MCP_TOOL_CREATE_INCIDENT_DESCRIPTION="an alternative description"
```

---

## 可用的工具集

以下是所有可用的工具集，默认全部启用。您也可以使用 `all` 来代表所有工具集。

| 工具集 | 描述 |
| --- | --- |
| `flashduty_incidents` | Flashduty 故障管理工具 |
| `flashduty_members` | Flashduty 成员管理工具 |
| `flashduty_teams` | Flashduty 团队管理工具 |
| `flashduty_channels` | Flashduty 协作空间管理工具 |

---

## 工具

该服务基于 Flashduty API 提供以下工具集：

### `flashduty_members` - 成员管理工具
- `flashduty_member_infos` - 通过人员 ID 获取成员信息

### `flashduty_teams` - 团队管理工具
- `flashduty_teams_infos` - 通过团队 ID 获取团队信息

### `flashduty_channels` - 协作空间管理工具
- `flashduty_channels_infos` - 通过协作空间 ID 获取协作空间信息

### `flashduty_incidents` - 故障管理工具
- `flashduty_incidents_infos` - 通过故障 ID 获取故障信息
- `flashduty_list_incidents` - 使用综合过滤器列出故障
- `flashduty_list_past_incidents` - 列出类似的历史故障
- `flashduty_get_incident_timeline` - 获取故障时间线和动态
- `flashduty_get_incident_alerts` - 获取与故障相关的警报
- `flashduty_create_incident` - 创建一个新故障
- `flashduty_ack_incident` - 确认故障
- `flashduty_resolve_incident` - 解决故障
- `flashduty_assign_incident` - 将故障分配给人员或升级规则
- `flashduty_add_responder` - 为故障添加响应者
- `flashduty_snooze_incident` - 暂停故障一段时间
- `flashduty_merge_incident` - 将多个故障合并为一个
- `flashduty_comment_incident` - 为故障添加评论
- `flashduty_update_incident_title` - 更新故障标题
- `flashduty_update_incident_description` - 更新故障描述
- `flashduty_update_incident_impact` - 更新故障影响
- `flashduty_update_incident_root_cause` - 更新故障根本原因
- `flashduty_update_incident_resolution` - 更新故障解决方案
- `flashduty_update_incident_severity` - 更新故障严重性
- `flashduty_update_incident_fields` - 更新自定义字段

---

## 库的使用

该模块导出的 Go API 目前应被视为不稳定，并可能发生重大更改。将来，我们可能会提供稳定性；如果有用例认为这很有价值，请提交问题。

## 许可证

该项目根据 MIT 许可证授权 - 有关详细信息，请参阅 [LICENSE](LICENSE) 文件。 