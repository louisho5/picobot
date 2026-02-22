# AGENTS.md — Picobot Developer Guide

This document provides essential information for AI coding agents working on the Picobot project.

---

## Project Overview

**Picobot** is a lightweight AI agent written in Go that provides persistent memory, tool calling, skills, and Telegram integration in a single ~8MB binary. It runs happily on minimal hardware like a $5 VPS or Raspberry Pi.

Key characteristics:
- **Language**: Go 1.26+
- **Binary size**: ~8MB (with `-ldflags="-s -w"`)
- **Docker image**: ~28MB (Alpine-based)
- **RAM usage**: ~10MB idle
- **Dependencies**: Only one external dependency (`spf13/cobra` for CLI)

---

## Technology Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.26+ |
| CLI Framework | [spf13/cobra](https://github.com/spf13/cobra) |
| HTTP/JSON | Go standard library (`net/http`, `encoding/json`) |
| LLM Provider | OpenAI-compatible API (OpenAI, OpenRouter, Ollama, etc.) |
| Telegram | Raw Bot API (standard library, no third-party SDK) |
| Container | Alpine Linux 3.20 (multi-stage Docker build) |

---

## Project Structure

```
cmd/picobot/          # CLI entry point (main.go, main_test.go)
embeds/               # Embedded assets (sample skills bundled into binary)
  skills/             # Sample skills: cron/, example/, weather/
internal/
  agent/              # Core agent loop, context builder, tools, skills
    memory/           # Memory store and LLM-based ranking
    skills/           # Skill loader
    tools/            # All tool implementations
  channels/           # Telegram bot integration
  mcp/                # MCP (Model Context Protocol) client support
  chat/               # Message hub (Inbound/Outbound channels)
  config/             # Config schema, loader, onboarding
  cron/               # Cron scheduler for scheduled tasks
  heartbeat/          # Periodic task checker
  providers/          # OpenAI-compatible provider + stub provider
  session/            # Session manager for chat history
docker/               # Dockerfile, compose, entrypoint script
.github/workflows/    # CI/CD: docker-publish.yml
```

---

## Build Commands

### Local Development Build

```sh
# Standard build
go build -o picobot ./cmd/picobot

# Optimized build (smaller binary, recommended for distribution)
go build -ldflags="-s -w" -o picobot ./cmd/picobot

# Cross-compilation examples
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot_linux_amd64 ./cmd/picobot
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot_linux_arm64 ./cmd/picobot
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot_mac_arm64 ./cmd/picobot
```

### Docker Build

```sh
# Build Docker image (run from project root, not docker/)
docker build -f docker/Dockerfile -t louisho5/picobot:latest .

# Build and push (maintainer only)
go build ./... && \
docker build -f docker/Dockerfile -t louisho5/picobot:latest . && \
docker push louisho5/picobot:latest
```

---

## Test Commands

```sh
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/agent/
go test ./internal/cron/
go test ./internal/providers/

# Run tests with verbose output
go test -v ./...

# Run a specific test
go test -v ./cmd/picobot/ -run TestMemoryCLI
```

---

## Code Style Guidelines

### General Conventions

1. **Follow standard Go conventions**: Use `gofmt`, `go vet`, and `golint`
2. **Package names**: Short, lowercase, no underscores (e.g., `tools`, `config`, `memory`)
3. **Interface naming**: Use `-er` suffix for single-method interfaces (e.g., `Ranker`, `Provider`)
4. **Error handling**: Always check errors, wrap with context using `fmt.Errorf("...: %w", err)`
5. **Comments**: Document all exported types and functions with complete sentences

### Project-Specific Patterns

1. **Tool Interface**: All tools must implement the `Tool` interface:
   ```go
   type Tool interface {
       Name() string
       Description() string
       Parameters() map[string]interface{}
       Execute(ctx context.Context, args map[string]interface{}) (string, error)
   }
   ```

2. **Provider Interface**: LLM providers implement:
   ```go
   type LLMProvider interface {
       Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (LLMResponse, error)
       GetDefaultModel() string
   }
   ```

3. **File permissions**: Use octal notation with explicit zeros:
   - Config files: `0o640`
   - Directories: `0o755`
   - Regular files: `0o644`

4. **Context propagation**: Always pass `context.Context` through the call chain for cancellation support

5. **Log usage**: Use standard `log` package for operational logs (not structured logging)

---

## Testing Instructions

### Test Organization

- Tests are co-located with source files (e.g., `loop.go` → `loop_test.go`)
- Integration tests should be marked with build tags or `_integration_test.go` suffix
- Use `t.TempDir()` for temporary directories (auto-cleanup)

### Test Patterns

1. **CLI tests**: Use `NewRootCmd()`, capture output with `bytes.Buffer`:
   ```go
   cmd := NewRootCmd()
   buf := &bytes.Buffer{}
   cmd.SetOut(buf)
   cmd.SetArgs([]string{"memory", "read", "today"})
   err := cmd.Execute()
   ```

2. **Temp home directory**: Set `HOME` env var to isolate tests:
   ```go
   tmp := t.TempDir()
   os.Setenv("HOME", tmp)
   ```

3. **Stub provider**: Tests should use `providers.NewStubProvider()` to avoid external API calls

### Running Integration Tests

Integration tests that call real LLM APIs should be skipped by default. Check for environment variables:
```sh
# Run with real provider (optional)
OPENAI_API_KEY=test-key go test -v ./internal/agent/memory/ -run Integration
```

---

## Security Considerations

### Filesystem Sandbox

- The `filesystem` tool uses `os.OpenRoot()` (Go 1.26+) for kernel-enforced sandboxing
- All file operations are restricted to the workspace directory
- Never bypass the sandbox or use raw `os.Open()` for workspace files

### Command Execution

- The `exec` tool blocks dangerous commands (`rm -rf`, `format`, `dd`, `shutdown`, etc.)
- Commands run with a timeout (default 60s)
- Always validate command safety before execution

### API Keys

- API keys are stored in `~/.picobot/config.json` with permission `0o640`
- Keys should never be logged or exposed in responses
- Support for environment variable override in Docker containers

### Telegram Security

- Use `allowFrom` to restrict which Telegram users can interact with the bot
- Empty `allowFrom` allows anyone (not recommended for production)

---

## Development Workflow

### Adding a New Tool

1. Create file in `internal/agent/tools/` (e.g., `database.go`)
2. Implement the `Tool` interface
3. Register in `internal/agent/loop.go` via `reg.Register(tools.NewXxxTool())`
4. Add tests in `xxx_test.go`
5. Update `TOOLS.md` workspace template in `internal/config/onboard.go`

### Adding MCP (Model Context Protocol) Support

Picobot supports connecting to external MCP servers to extend tool capabilities.

**Configuration** (in `~/.picobot/config.json`):
```json
{
  "mcp": {
    "servers": {
      "github": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-github"],
        "env": {
          "GITHUB_PERSONAL_ACCESS_TOKEN": "your-token"
        },
        "enabled": true
      },
      "filesystem": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/allowed/dir"],
        "enabled": true
      }
    }
  }
}
```

**How it works:**
- MCP servers are started as subprocesses using stdio transport
- Tools from all connected MCP servers are registered with the prefix `<serverName>_<toolName>`
- The agent can call MCP tools just like native tools
- Connections are cleaned up when the agent shuts down

**Available MCP servers:** See [MCP Servers Registry](https://github.com/modelcontextprotocol/servers)

### Adding a New Provider

1. Create file in `internal/providers/` (e.g., `anthropic.go`)
2. Implement the `LLMProvider` interface
3. Add to factory in `internal/providers/factory.go`
4. Update config schema in `internal/config/schema.go`
5. Add tests

### Version Updates

Update the version constant in `cmd/picobot/main.go`:
```go
const version = "0.1.0"
```

### CI/CD Pipeline

The GitHub Actions workflow (`.github/workflows/docker-publish.yml`) runs on manual dispatch:
1. Runs `go test ./...`
2. Builds Go binary for validation
3. Builds multi-platform Docker image (linux/amd64, linux/arm64)
4. Pushes to Docker Hub

---

## Key Configuration

### Config File Location

- Default: `~/.picobot/config.json`
- Set via environment for Docker: `PICOBOT_HOME=/home/picobot/.picobot`

### Workspace Files

During `picobot onboard`, these files are created in `~/.picobot/workspace/`:

| File | Purpose |
|------|---------|
| `SOUL.md` | Agent personality and values |
| `AGENTS.md` | Agent instructions and guidelines |
| `USER.md` | User profile (customize this) |
| `TOOLS.md` | Tool reference documentation |
| `HEARTBEAT.md` | Periodic tasks checked every heartbeat |
| `memory/MEMORY.md` | Long-term memory |
| `memory/YYYY-MM-DD.md` | Daily notes |
| `skills/` | Skill packages (markdown files) |

---

## Architecture Notes

### Message Flow

```
Inbound (Telegram/CLI) → Chat Hub → Agent Loop → LLM Provider
                                              ↓
Outbound ← Tool Execution ← Tool Registry ← Response
                              ↑
                         MCP Servers (optional)
```

### Agent Loop

- Processes messages from `hub.In` channel
- Builds context from session history, memory, and bootstrap files
- Calls LLM with available tool definitions
- Executes tool calls iteratively (max iterations configurable)
- Sends final response via `hub.Out` channel
- Special handling for "remember..." commands (fast-path without LLM)

### Memory System

- **Daily notes**: Auto-organized by date (`memory/YYYY-MM-DD.md`)
- **Long-term memory**: Persistent across sessions (`memory/MEMORY.md`)
- **Ranking**: LLM-based semantic relevance ranking for retrieval

---

## Troubleshooting

### Build Issues

```sh
# Clean and rebuild
go clean -cache
go mod tidy
go build ./...
```

### Docker Issues

Ensure building from project root (not `docker/` directory):
```sh
docker build -f docker/Dockerfile -t picobot:latest .
```

---

## External Resources

- **OpenRouter** (recommended): https://openrouter.ai/keys
- **Telegram BotFather**: https://t.me/BotFather
- **Go Standard Library**: https://pkg.go.dev/std
