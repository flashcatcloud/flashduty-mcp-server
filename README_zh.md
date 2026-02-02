# Flashduty MCP Server

[English](README.md) | 中文

Flashduty MCP Server 是一个基于 [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction) 的服务，提供与 Flashduty API 的无缝对接，帮助开发者和工具实现智能化的故障管理与自动化运维。

### 应用场景

- 自动化 Flashduty 工作流程
- 从 Flashduty 抽取和分析数据
- 构建与 Flashduty 交互的 AI 工具和应用

---

## 远程服务

[![在 Cursor 中安装](https://img.shields.io/badge/Cursor-安装服务-24bfa5?style=flat-square&logo=visualstudiocode&logoColor=white)](#remote-cursor)

远程服务是接入 Flashduty 最便捷的方式。如果你的 MCP 客户端不支持远程服务，可以使用[本地服务](#本地服务)。

### 前置条件

1. 支持 MCP 协议的客户端，如 [Cursor](https://www.cursor.com/)
2. Flashduty 账户的 APP Key

### 配置示例

<span id="remote-cursor"></span>

以 Cursor 为例：

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

> **提示：** 具体配置位置请参考你所使用的 MCP 客户端文档。

---

## 本地服务

[![在 Cursor 中安装](https://img.shields.io/badge/Cursor-安装服务-24bfa5?style=flat-square&logo=visualstudiocode&logoColor=white)](#local-cursor)

### 前置条件

1. 如需容器化运行，请先安装并启动 [Docker](https://www.docker.com/)
2. Flashduty 账户的 APP Key

### 配置示例

<span id="local-cursor"></span>

以 Cursor 为例：

#### Docker 方式

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

#### 二进制方式

你可以从 [GitHub Releases](https://github.com/flashcatcloud/flashduty-mcp-server/releases) 下载预编译的二进制文件，也可以通过 `go build` 在 `cmd/flashduty-mcp-server` 目录下自行构建。

**环境变量方式：**
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

**命令行参数方式：**
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

## 配置选项

Flashduty MCP Server 支持以下配置：

- **工具集 (Toolsets)**：按功能分组启用/禁用工具，减少上下文大小，帮助 LLM 更精准地选择工具
- **只读模式 (Read-Only)**：禁止写操作，适用于安全要求较高的场景
- **输出格式 (Output Format)**：支持 JSON 和 TOON 格式，TOON 格式可减少 30-50% 的 token 消耗
- **国际化 (i18n)**：支持自定义工具描述

### 远程服务配置

通过 URL 参数动态配置：

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

- `headers.Authorization`：用于认证的 Flashduty APP Key，需添加 `Bearer ` 前缀
- `toolsets=...`：启用指定的工具集，多个用逗号分隔
- `read_only=true`：启用只读模式

### 本地服务配置

#### 1. 环境变量

| 变量 | 说明 | 必填 | 默认值 |
|---|---|---|---|
| `FLASHDUTY_APP_KEY` | Flashduty APP Key | ✅ | - |
| `FLASHDUTY_TOOLSETS` | 启用的工具集（逗号分隔） | ❌ | 全部 |
| `FLASHDUTY_READ_ONLY` | 只读模式（`1` 或 `true`） | ❌ | `false` |
| `FLASHDUTY_OUTPUT_FORMAT` | 输出格式（`json` 或 `toon`） | ❌ | `json` |
| `FLASHDUTY_BASE_URL` | API 地址 | ❌ | `https://api.flashcat.cloud` |
| `FLASHDUTY_LOG_FILE` | 日志文件路径 | ❌ | stderr |
| `FLASHDUTY_ENABLE_COMMAND_LOGGING` | 记录请求日志 | ❌ | `false` |
| `TZ` | 日志时间戳时区（如 `Asia/Shanghai`、`America/New_York`） | ❌ | 系统默认（无时区数据的容器中回退到 `Asia/Shanghai`） |

**Docker 示例：**

```bash
docker run -i --rm \
  -e FLASHDUTY_APP_KEY=<your-app-key> \
  -e FLASHDUTY_TOOLSETS="incidents,users,channels" \
  -e FLASHDUTY_READ_ONLY=1 \
  -e TZ=Asia/Shanghai \
  registry.flashcat.cloud/public/flashduty-mcp-server
```

> **提示：** `TZ` 环境变量用于控制日志时间戳的时区。如果未设置，服务将使用系统默认时区，或在没有时区数据的容器（如 distroless 镜像）中回退到 `Asia/Shanghai`。常用时区值包括 `Asia/Shanghai`、`America/New_York`、`Europe/London`、`UTC` 等。

#### 2. 命令行参数

```bash
./flashduty-mcp-server stdio \
  --app-key your_app_key_here \
  --toolsets incidents,users,channels \
  --read-only
```

支持的参数：
- `--app-key`：Flashduty APP Key
- `--toolsets`：启用的工具集
- `--read-only`：只读模式
- `--output-format`：输出格式（`json` 或 `toon`）
- `--base-url`：API 地址
- `--log-file`：日志文件路径
- `--enable-command-logging`：记录请求日志
- `--export-translations`：导出翻译配置

> **注意：** 命令行参数优先级高于环境变量。

#### 3. TOON 输出格式

服务支持 [TOON (Token-Oriented Object Notation)](https://github.com/toon-format/toon) 格式，可显著降低 token 消耗（约 30-50%），特别适合 LLM 场景。

**JSON 格式（默认）：**
```json
{"members":[{"person_id":1,"person_name":"Alice"},{"person_id":2,"person_name":"Bob"}],"total":2}
```

**TOON 格式（紧凑）：**
```
members[2]{person_id,person_name}:
  1,Alice
  2,Bob
total: 2
```

启用方式：

```bash
# 环境变量
export FLASHDUTY_OUTPUT_FORMAT=toon

# 命令行参数
./flashduty-mcp-server stdio --output-format toon
```

> **提示：** TOON 格式对统一结构的对象数组（如成员列表、故障列表）效果最佳，主流 LLM 均可正确解析。

#### 4. 国际化 / 自定义描述（仅本地）

可通过配置文件或环境变量覆盖工具描述：

**配置文件方式：**

在二进制文件同目录下创建 `flashduty-mcp-server-config.json`：
```json
{
  "TOOL_CREATE_INCIDENT_DESCRIPTION": "自定义描述",
  "TOOL_LIST_TEAMS_DESCRIPTION": "列出账户中的所有团队"
}
```

**环境变量方式：**

```sh
export FLASHDUTY_MCP_TOOL_CREATE_INCIDENT_DESCRIPTION="自定义描述"
```

#### 5. 日志与安全

服务内置了结构化日志和增强的安全特性：

- **数据脱敏**：日志会自动对敏感信息（如 `APP_KEY` 和 `Authorization` 请求头）进行掩码处理，防止密钥泄露。
- **日志截断**：对于过大的请求/响应体，日志会自动进行截断（默认 2KB），确保服务性能。
- **链路追踪**：支持 W3C Trace Context 标准。日志中会自动关联 `trace_id`，方便跨服务追踪请求全链路趋势。

---

## 工具集

默认启用全部工具集，也可使用 `all` 代表全部。

| 工具集 | 说明 | 工具数 |
| --- | --- | --- |
| `incidents` | 故障生命周期管理 | 6 |
| `changes` | 变更记录查询 | 1 |
| `status_page` | 状态页管理 | 4 |
| `users` | 成员和团队查询 | 2 |
| `channels` | 协作空间和分派策略 | 2 |
| `fields` | 自定义字段定义 | 1 |

**共计 16 个工具**

---

## 工具列表

### `incidents` - 故障管理 (6)
- `query_incidents` - 查询故障（含时间线、告警、响应人等完整信息）
- `create_incident` - 创建故障
- `update_incident` - 更新故障（标题、描述、严重程度、自定义字段）
- `ack_incident` - 认领故障
- `close_incident` - 关闭故障
- `list_similar_incidents` - 查找相似历史故障

### `changes` - 变更管理 (1)
- `query_changes` - 查询变更记录

### `status_page` - 状态页 (4)
- `query_status_pages` - 查询状态页配置
- `list_status_changes` - 查询状态页变更事件
- `create_status_incident` - 创建状态页故障
- `create_change_timeline` - 添加变更时间线

### `users` - 成员管理 (2)
- `query_members` - 查询成员
- `query_teams` - 查询团队（含成员详情）

### `channels` - 协作空间 (2)
- `query_channels` - 查询协作空间（含团队、创建者名称等富化信息）
- `query_escalation_rules` - 查询分派规则

### `fields` - 字段管理 (1)
- `query_fields` - 查询自定义字段定义

---

## 作为库使用

本项目导出的 Go API 目前处于不稳定状态，可能会有 breaking changes。如有稳定 API 需求，欢迎提 Issue。

## 开源协议

本项目基于 MIT 协议开源，详见 [LICENSE](LICENSE) 文件。
