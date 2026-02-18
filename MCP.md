# Model Context Protocol (MCP) Guide

Picobot supports the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/), an open standard for connecting AI assistants to external tools and data sources.

## What is MCP?

MCP is a protocol that standardises how AI agents connect to external tools. Think of it like a USB-C port for AI applications — one standard interface for many different services.

**Benefits:**
- Use official, maintained integrations instead of building your own
- Secure, controlled access to external APIs
- Growing ecosystem of available servers

## How It Works in Picobot

```
┌─────────────┐   stdio or HTTP/SSE   ┌──────────────┐   HTTP/HTTPS   ┌─────────────┐
│   Picobot   │ ─────────────────────>│  MCP Server  │ ─────────────> │  External   │
│    (Go)     │   JSON-RPC messages   │ (subprocess  │   API calls    │    API      │
│             │ <─────────────────────│  or remote)  │ <───────────── │ (GitHub,    │
└─────────────┘                       └──────────────┘                │  Slack, etc)│
                                                                       └─────────────┘
```

1. Picobot connects to each MCP server at startup
2. It sends an `initialize` handshake (protocol version `2024-11-05`)
3. It calls `tools/list` to enumerate available tools
4. Tools are registered in the tool registry with a `serverName_toolName` prefix
5. When the LLM calls an MCP tool, Picobot sends a `tools/call` request to the appropriate server
6. Results flow back through the same channel to the LLM

## Configuration

Add MCP servers to `~/.picobot/config.json`:

### HTTP Streaming Server

```json
{
  "mcp": {
    "servers": {
      "my-api": {
        "url": "https://api.example.com/mcp",
        "headers": {
          "Authorization": "Bearer your-token-here"
        }
      }
    }
  }
}
```

### Stdio Server

```json
{
  "mcp": {
    "servers": {
      "github": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-github"],
        "env": {
          "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_your_token_here"
        }
      },
      "fetch": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-fetch"]
      }
    }
  }
}
```

All servers in the config are started automatically at startup. To disable a server, remove it from the config.

### Configuration Fields

Picobot supports two transports. Use either `command` (stdio) **or** `url` (HTTP).

**Stdio transport (local subprocess):**

| Field | Required | Description |
|-------|----------|-------------|
| `command` | Yes | Executable to run (e.g. `npx`, `python3`) |
| `args` | No | Arguments passed to the command |
| `env` | No | Extra environment variables for the process |

**HTTP transport (remote server):**

| Field | Required | Description |
|-------|----------|-------------|
| `url` | Yes | HTTP endpoint (requests go to `<url>/mcp`) |

### Tool Namespacing

MCP tools are registered in the tool registry with a `mcp_[serverName-from-config]_[toolName]` prefix to avoid collisions:

```
mcp_github_search_repositories
mcp_github_get_issue
mcp_fetch_fetch
```

If two servers export a tool with the same resulting key, the later server overwrites the earlier one and a warning is logged.

## Transports

### Stdio Transport

Spawns the MCP server as a local subprocess and communicates over stdin/stdout using newline-delimited JSON-RPC. This is the most common transport and works with nearly all available MCP servers.

- Server stderr is forwarded to Picobot's log as `[MCP Server] <line>`
- Concurrent calls are fully supported (responses are dispatched by ID)
- Server-initiated notifications (no `id` field) are logged and ignored
- On shutdown: stdin is closed, Picobot waits up to 5 seconds for graceful exit, then sends SIGKILL

### HTTP Transport

Connects to a remote MCP server over HTTP. Requests go to `POST <url>/mcp`. Supports both direct JSON responses and Server-Sent Events (SSE) streaming responses.

- Session state is maintained via `Mcp-Session-Id` header
- SSE resumption is supported via `Last-Event-ID` header
- Server-initiated notifications can be received via `GET <url>/mcp` (SSE stream)
- On close: sends `DELETE <url>/mcp` to terminate the session

## Configuration Examples

### GitHub Integration

```json
{
  "mcp": {
    "servers": {
      "github": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-github"],
        "env": {
          "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_xxxxxxxxxxxx"
        }
      }
    }
  }
}
```

Get a token: GitHub → Settings → Developer settings → Personal access tokens
Required scopes: `repo` (for private repos), `read:user`

### Filesystem Access

```json
{
  "mcp": {
    "servers": {
      "fs": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/user/documents"]
      }
    }
  }
}
```

### Multiple Servers

```json
{
  "mcp": {
    "servers": {
      "github": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-github"],
        "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_xxx" }
      },
      "fetch": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-fetch"]
      },
      "postgres": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-postgres", "postgresql://localhost/mydb"]
      }
    }
  }
}
```

### Remote HTTP Server

```json
{
  "mcp": {
    "servers": {
      "cloud-api": {
        "url": "https://mcp.example.com/v1"
      }
    }
  }
}
```

You can mix stdio and HTTP servers in the same config:

```json
{
  "mcp": {
    "servers": {
      "local-tools": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/user"]
      },
      "cloud-service": {
        "url": "https://api.example.com/mcp"
      }
    }
  }
}
```

### Custom Python Server

```json
{
  "mcp": {
    "servers": {
      "myserver": {
        "command": "python3",
        "args": ["/path/to/my_mcp_server.py"],
        "env": {
          "API_KEY": "secret123"
        }
      }
    }
  }
}
```

## Available MCP Servers

### Official Reference Servers

| Server | Package | Description |
|--------|---------|-------------|
| **Filesystem** | `@modelcontextprotocol/server-filesystem` | Read/write files |
| **Fetch** | `@modelcontextprotocol/server-fetch` | Web content fetching |
| **Git** | `@modelcontextprotocol/server-git` | Git repository tools |
| **Memory** | `@modelcontextprotocol/server-memory` | Knowledge graph memory |
| **Time** | `@modelcontextprotocol/server-time` | Timezone conversions |

### Test Servers

| Server | URL | Description |
|--------|-----|-------------|
| **Echo** | `https://mcp-echo.fybre.me` | Echoes back whatever is sent — useful for testing MCP connectivity |

Browse the [MCP Registry](https://registry.modelcontextprotocol.io/) for hundreds of community-built servers.

## Viewing Available Tools

List all loaded tools (including MCP):

```sh
./picobot tools
```

List only MCP tools (connects to configured servers):

```sh
./picobot tools --mcp
```

Output:
```
MCP tools (12):

  • github_search_repositories
    [github] Search for repositories on GitHub
    Parameters: yes

  • github_get_issue
    [github] Get details about a GitHub issue
    Parameters: yes
```

## Logging

Picobot logs MCP activity to stdout. Key log lines:

```
# Startup
[MCP] Using stdio transport for command: npx
[MCP] Connected to server github-mcp-server (version: 0.6.2)
[MCP] Server github provides 25 tools
[MCP] Registering 25 MCP tools

# Tool calls
[MCP] → github_search_repositories {"query":"picobot"}
[MCP] ✓ github_search_repositories completed in 342ms (4821 bytes)
[MCP] ✗ github_get_issue failed after 30000ms: MCP request timeout: context deadline exceeded

# Server output
[MCP Server] <line from server stderr>

# Server-initiated notifications (e.g. cancellation, progress)
[MCP] Received server notification: notifications/cancelled
```

## Tool Usage Statistics

The agent has access to an `mcp_stats` tool that reports how many times each MCP tool has been called in the current session. Ask the agent:

> "Show me my MCP tool usage"

Example output:
```
MCP Tool Usage Statistics:

  • github_search_repositories: 5 call(s)
  • github_get_issue: 2 call(s)
  • fetch_fetch: 1 call(s)
```

## Troubleshooting

### "failed to initialize MCP server: context deadline exceeded"

Timeout (30 seconds) while connecting.

- Check the server installs and starts: `npx -y @package/name --help`
- Verify required environment variables are set
- Check `[MCP Server]` log lines for errors from the server process

### "failed to start MCP server: exec..."

Command not found.

- Ensure Node.js is installed: `node --version`
- Use full path if needed: `"command": "/usr/local/bin/npx"`

### "Registering 0 MCP tools"

Server started but returned no tools.

- Verify the server supports tool listing (some servers only provide resources)
- Check `[MCP Server]` log lines for startup errors

### "Received server notification: ..."

Normal. Servers send notifications (e.g. `notifications/cancelled`) that have no `id` field. These are logged and safely ignored — they do not indicate an error.

### Tools not appearing

MCP tools are enumerated at startup only. Restart Picobot after changing the config.

## Architecture

### Tool Execution Flow

```
1. User sends message
2. Agent loop sends all tool definitions (native + MCP) to the LLM
3. LLM decides to call e.g. "github_search_repositories"
4. Registry routes to mcpToolWrapper → Manager → Client → Transport
5. Transport sends tools/call JSON-RPC to the MCP server
6. MCP server calls the external API (GitHub, etc.)
7. Result flows back: Transport → Client → Manager → Registry → Agent loop
8. LLM receives result and formulates a response
```

### Security

- Stdio servers run as local subprocesses — no network ports exposed
- Environment variables (API keys) are passed via process environment
- Each server is isolated in its own process

## Further Reading

- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [MCP Registry](https://registry.modelcontextprotocol.io/)
- [Official MCP Servers](https://github.com/modelcontextprotocol/servers)

---

**Note:** MCP is a rapidly evolving standard. Check the [official documentation](https://modelcontextprotocol.io/) for the latest updates.
