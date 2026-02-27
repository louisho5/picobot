# Development Guide

This document covers how to set up a local development environment, build, test, and publish Picobot.

## What You'll Need

- [Go](https://go.dev/dl/) 1.25+ installed
- [Docker](https://www.docker.com/) installed (for container builds)
- A [Docker Hub](https://hub.docker.com/) account (for publishing)

## Project Structure

```
cmd/picobot/          CLI entry point (main.go)
embeds/               Embedded assets (sample skills bundled into binary)
  skills/             Sample skills extracted on onboard
internal/
  agent/              Agent loop, context, tools, skills
  chat/               Chat message hub (Inbound / Outbound channels)
  channels/           Telegram and Discord integration
  config/             Config schema, loader, onboarding
  cron/               Cron scheduler
  heartbeat/          Periodic task checker
  memory/             Memory read/write/rank
  providers/          OpenAI-compatible provider (OpenAI, OpenRouter, Ollama, etc.)
  session/            Session manager
docker/               Dockerfile, compose, entrypoint
```

## Local Development

### Clone and install dependencies

```sh
git clone https://github.com/user/picobot.git
cd picobot
go mod download
```

### Build the binary

```sh
go build -o picobot ./cmd/picobot
```

The binary will be created in the current directory.

### Run locally

```sh
# First time? Run onboard to create ~/.picobot config and workspace
./picobot onboard

# Try a quick query
./picobot agent -m "Hello!"

# Login to channels (Telegram, Discord, WhatsApp)
./picobot channels login

# Start the full gateway (includes channels, heartbeat, etc.)
./picobot gateway
```

### Run tests

```sh
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/cron/
go test ./internal/agent/

# Run tests with verbose output
go test -v ./...
```

## Versioning

The version string is defined in `cmd/picobot/main.go`:

```go
const version = "x.x.x"
```

Update this value before building a new release.

## Building for Different Platforms

### Quick builds with Make

The project ships a `Makefile` that cross-compiles all supported platforms in one command:

```sh
# Build all targets — full and lite variants for Linux amd64/arm64 and macOS arm64
make build

# Build individual targets
make linux_amd64        # full build, Linux x86-64
make linux_arm64        # full build, Linux ARM64
make mac_arm64          # full build, macOS Apple Silicon
make linux_amd64_lite   # lite build, Linux x86-64
make linux_arm64_lite   # lite build, Linux ARM64
make mac_arm64_lite     # lite build, macOS Apple Silicon

# Remove all built binaries
make clean
```

Output files are named `picobot_<os>_<arch>[_lite]` and dropped in the project root.

### Manual cross-compilation

If you prefer to invoke `go build` directly:

```sh
# Linux AMD64 (most VPS / servers)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot_linux_amd64 ./cmd/picobot

# Linux ARM64 (Raspberry Pi, ARM servers)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot_linux_arm64 ./cmd/picobot

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot_mac_arm64 ./cmd/picobot

# Windows
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot.exe ./cmd/picobot
```

**What the flags do:**
- `CGO_ENABLED=0` → pure static binary, no libc dependency
- `-ldflags="-s -w"` → strip debug symbols (~22 MB → ~9 MB for the lite build)

### Full vs Lite builds

Picobot ships in two variants controlled by the `lite` Go build tag:

| Variant | Tag | Binary size | Future heavy packages |
|---------|-----|-------------|----------------------|
| **Full** (default) | *(none)* | ~22 MB | All features |
| **Lite** | `-tags lite` | ~9 MB | ❌ WhatsApp not included |

**Why "Lite" exists:**

Some optional features — starting with WhatsApp via [whatsmeow](https://github.com/tulir/whatsmeow) + [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) — pull in large dependencies that add ~13 MB to the binary. We know there are some users running Picobot on a standard server or desktop never need those features and shouldn't have to pay the size cost.

The lite build is aimed at resource-constrained environments: IoT devices, cheap VPS with limited storage, or any deployment where a ~9 MB static binary is strongly preferred over a ~22 MB one. It includes every core feature (agent loop, Telegram, Discord, memory, skills, cron, heartbeat) but omits packages gated behind the `!lite` build tag.

As new optional heavy integrations are added to Picobot in the future, they will follow the same pattern — included in the full build by default, excluded from the lite build.

```sh
# Full build — all features including WhatsApp (default)
go build ./cmd/picobot

# Lite build — no WhatsApp or other heavy optional packages
go build -tags lite ./cmd/picobot
```

For cross-compilation, simply add `-tags lite` alongside the existing `GOOS`/`GOARCH` flags, or use `make linux_amd64_lite` etc.

```sh
# Lite, Linux ARM64 (e.g. Raspberry Pi)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -tags lite -o picobot_linux_arm64_lite ./cmd/picobot
```

## Docker Workflow

### Build the image

We use a multi-stage Alpine-based build — keeps the final image around ~33MB:

```sh
docker build -f docker/Dockerfile -t louisho5/picobot:latest .
```

#### Multi-arch builds with BuildKit

Picobot's Dockerfile supports BuildKit/`buildx` so you can push both AMD64 and ARM64 images in a single run:

```sh
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --builder default \
  -t louisho5/picobot:latest .
```

Add `--push` to publish directly to a registry or `--load` to import one architecture into your local Docker engine.

> **Important:** Run this from the **project root**, not from inside `docker/`. The build context needs access to the whole codebase.

### Test it locally

Spin up a container to make sure it works:

```sh
docker run --rm -it \
  -e OPENAI_API_KEY="your-key" \
  -e OPENAI_API_BASE="https://openrouter.ai/api/v1" \
  -e PICOBOT_MODEL="google/gemini-2.5-flash" \
  -e TELEGRAM_BOT_TOKEN="your-token" \
  -v ./picobot-data:/home/picobot/.picobot \
  louisho5/picobot:latest
```

Check logs:

```sh
docker logs -f picobot
```

### Push to Docker Hub

**Build and push** in one shot:

```sh
go build ./... && \
docker build -f docker/Dockerfile -t louisho5/picobot:latest . && \
docker push louisho5/picobot:latest
```

Docker hub: [hub.docker.com/r/louisho5/picobot](https://hub.docker.com/r/louisho5/picobot).

## Environment Variables

These environment variables configure the Docker container:

| Variable | Description | Required |
|---|---|---|
| `OPENAI_API_KEY` | OpenAI-compatible API key (OpenRouter, OpenAI, etc.) | Yes |
| `OPENAI_API_BASE` | OpenAI-compatible API base URL | No |
| `PICOBOT_MODEL` | LLM model to use (e.g. `google/gemini-2.5-flash`) | No |
| `TELEGRAM_BOT_TOKEN` | Telegram Bot API token | No |
| `TELEGRAM_ALLOW_FROM` | Comma-separated Telegram user IDs to allow | No |
| `DISCORD_BOT_TOKEN` | Discord Bot token from Developer Portal | No |
| `DISCORD_ALLOW_FROM` | Comma-separated Discord user IDs to allow | No |

## Extending Picobot

### Adding a new tool

Let's say you want to add a `database` tool that queries PostgreSQL:

1. **Create the file:**
   ```sh
   touch internal/agent/tools/database.go
   ```

2. **Implement the `Tool` interface:**
   ```go
   package tools
   
   import "context"
   
   type DatabaseTool struct{}
   
   func NewDatabaseTool() *DatabaseTool { return &DatabaseTool{} }
   
   func (t *DatabaseTool) Name() string { return "database" }
   func (t *DatabaseTool) Description() string { 
       return "Query PostgreSQL database"
   }
   func (t *DatabaseTool) Parameters() map[string]interface{} {
       // return JSON Schema for arguments
   }
   func (t *DatabaseTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
       // your implementation here
   }
   ```

3. **Register it in `internal/agent/loop.go`:**
   ```go
   reg.Register(tools.NewDatabaseTool())
   ```

4. **Test it:**
   ```sh
   go test ./internal/agent/tools/
   ```

That's it. The agent loop will automatically expose it to the LLM and route tool calls to your implementation.

### Adding a new LLM provider

Want to add support for Anthropic, Cohere, or a custom provider?

1. **Create the provider file:**
   ```sh
   touch internal/providers/anthropic.go
   ```

2. **Implement the `LLMProvider` interface from `internal/providers/provider.go`:**
   ```go
   type LLMProvider interface {
       Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (ChatResponse, error)
       GetDefaultModel() string
   }
   ```

3. **Wire it up in the config schema:**
   - Add config fields in `internal/config/schema.go`
   - Update the factory logic in `internal/providers/factory.go`

4. **Test it:**
   ```sh
   go test ./internal/providers/
   ```

## Troubleshooting

### Build fails with weird errors

Try cleaning and re-downloading deps:

```sh
go clean -cache
go mod tidy
go build ./...
```
