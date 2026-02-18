# Model Context Protocol (MCP) Guide

Picobot supports the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/), an open standard for connecting AI assistants to external data sources and tools.

## What is MCP?

MCP is a protocol developed by Anthropic that standardizes how AI agents connect to external tools. Think of it like a USB-C port for AI applications - one standard interface for many different services.

**Benefits:**
- Use official, maintained tools instead of building your own
- Secure, controlled access to external APIs
- Growing ecosystem of available integrations

> **Prerequisites:** MCP servers are separate processes that may require additional runtimes. Most official servers need **Node.js** (for `npx`), while community servers may need **Python**, **Docker**, or other runtimes. See [Requirements](#requirements) below.

## How It Works in Picobot

```
┌─────────────┐      stdio (local)      ┌─────────────┐      HTTP/HTTPS       ┌─────────────┐
│   Picobot   │  ─────────────────────>  │  MCP Server  │  ──────────────────> │  External   │
│    (Go)     │    JSON-RPC messages     │   (Node.js)  │    API calls         │    API      │
│             │ <──────────────────────  │              │ <─────────────────── │ (GitHub,    │
└─────────────┘                          └─────────────┘                      │  Slack, etc)│
                                                                               └─────────────┘
```

1. **Picobot spawns** an MCP server as a local subprocess (via stdio)
2. **Communication** happens via JSON-RPC over stdin/stdout
3. **The MCP server** translates requests to HTTP API calls (GitHub, Slack, etc.)
4. **Results** flow back through the same channel

## Requirements

- **Node.js** must be installed on the machine running Picobot
- MCP servers are installed on-demand via `npx`

## Configuration

Add MCP servers to `~/.picobot/config.json`:

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

### Configuration Fields

Picobot uses the **standard MCP configuration format**:

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `command` | **Yes** | - | Executable to run (e.g., `npx`, `python3`, `docker`) |
| `args` | No | `[]` | Arguments passed to command |
| `env` | No | `{}` | Environment variables for the server |

> **Note:** All servers defined in the config are automatically started. To disable a server, remove it from the config or comment it out.

### Tool Namespacing

MCP tools are registered with a prefix to avoid collisions:

```
<serverName>_<toolName>

Examples:
- github_search_repositories
- github_get_issue
- fetch_fetch_url
```

**Note:** If two servers export tools with identical names, the later server in the config will overwrite the earlier one. A warning will be logged: `[MCP] Warning: Tool X already registered, overwriting`

## Available MCP Servers

### Official Reference Servers

| Server | Package | Description | Required Env |
|--------|---------|-------------|--------------|
| **Filesystem** | `@modelcontextprotocol/server-filesystem` | Read/write files | `FILESYSTEM_PATHS` |
| **Fetch** | `@modelcontextprotocol/server-fetch` | Web content fetching | None |
| **Git** | `@modelcontextprotocol/server-git` | Git repository tools | None |
| **Memory** | `@modelcontextprotocol/server-memory` | Knowledge graph memory | None |
| **Time** | `@modelcontextprotocol/server-time` | Timezone conversions | None |

### Community Servers

Browse the [MCP Registry](https://registry.modelcontextprotocol.io/) for hundreds of community-built servers:

- PostgreSQL, SQLite, Redis
- Slack, Discord
- Google Drive, Dropbox
- Brave Search, DuckDuckGo
- And many more!

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

**Get a token:** GitHub → Settings → Developer settings → Personal access tokens

**Required scopes:** `repo` (for private repos), `read:user`

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

## HTTP-Only Servers (Proxy Pattern)

Some MCP servers only support HTTP transport. You can use them via a **stdio-to-HTTP proxy**:

```javascript
// mcp-http-proxy.js
const SERVER_URL = process.env.MCP_SERVER_URL;

process.stdin.on('data', async (data) => {
  const response = await fetch(SERVER_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: data
  });
  console.log(await response.text());
});
```

```json
{
  "mcp": {
    "servers": {
      "remote-api": {
        "command": "node",
        "args": ["/path/to/mcp-http-proxy.js"],
        "env": {
          "MCP_SERVER_URL": "https://api.example.com/mcp"
        }
      }
    }
  }
}
```

## Viewing Available Tools

List all loaded tools (including MCP):

```sh
./picobot tools
```

Show only MCP tools:

```sh
./picobot tools -m
# or
./picobot tools --mcp
```

Output:
```
Available tools (15):

  • filesystem
    Read, write, and list files in the workspace
    Parameters: yes

  • github_search_repositories
    [github] Search for repositories on GitHub
    Parameters: yes

  • fetch_fetch_url
    [fetch] Fetch content from a URL
    Parameters: yes
```

The `[serverName]` prefix identifies MCP tools.

## Tool Usage Statistics

The agent has access to an `mcp_stats` tool that shows how many times each MCP tool has been used in the current session:

**Ask the agent:** "Show me my MCP tool usage" or "What MCP tools have I used?"

**Example output:**
```
MCP Tool Usage Statistics:

  • github_search_repositories: 5 call(s)
  • github_get_issue: 2 call(s)
  • fetch_fetch_url: 1 call(s)
```

This is useful for:
- Monitoring API usage
- Debugging which tools are being called
- Understanding agent behavior

## Troubleshooting

### "failed to initialize MCP server: context deadline exceeded"

**Cause:** Timeout (30 seconds) while connecting to MCP server.

**Solutions:**
- Check the server is installed: `npx -y @package/name --help`
- Verify required environment variables are set
- Check server logs (stderr is logged to console)

### "failed to start MCP server: exec..."

**Cause:** Command not found.

**Solutions:**
- Ensure Node.js is installed: `node --version`
- Use full path if needed: `"command": "/usr/local/bin/npx"`
- For Python servers, ensure `python3` is in PATH

### "Registering 0 MCP tools"

**Cause:** MCP server started but no tools were enumerated.

**Solutions:**
- Verify server supports tool listing (some servers only provide resources)
- Check server logs for errors

### Tools not appearing in list

**Cause:** Server started after tool discovery.

**Solutions:**
- Restart Picobot: MCP tools are enumerated at startup
- Check logs: `grep "MCP" picobot.log`

## Architecture Details

### Transport

Picobot uses **stdio transport** - the most common and recommended approach:
- Spawns server as subprocess
- JSON-RPC over stdin/stdout
- Works with 95%+ of available MCP servers

### Security

- MCP servers run as **local subprocesses** (not remote)
- Environment variables (like API keys) are passed securely via process env
- Each server is sandboxed in its own process
- No network ports exposed

### Tool Execution Flow

```
1. User sends message to Picobot
2. Agent loop builds context with all tool definitions
3. LLM decides to use tool (e.g., "github_search_repositories")
4. Agent loop executes tool via registry
5. Registry routes to MCP manager
6. MCP manager forwards to correct MCP server via JSON-RPC
7. MCP server calls external API (GitHub, etc.)
8. Result flows back through same path
9. LLM receives result and formulates response
```

## Further Reading

- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [MCP Registry](https://registry.modelcontextprotocol.io/)
- [Official MCP Servers](https://github.com/modelcontextprotocol/servers)
- [MCP SDK Documentation](https://modelcontextprotocol.io/docs/concepts/sdk)

## Contributing

To add native support for a new MCP transport or feature:

1. See `internal/mcp/` for implementation
2. Add tests in `internal/mcp/*_test.go`
3. Update this guide with examples

---

**Note:** MCP is a rapidly evolving standard. Check the [official documentation](https://modelcontextprotocol.io/) for the latest updates.