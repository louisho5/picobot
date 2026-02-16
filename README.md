<p align="center">
  <img src="logo.png" alt="Picobot" width="250" height="150">
  <h1 align="center">Picobot</h1>
  <p align="center"><strong>The AI agent that runs anywhere — even on a $5 VPS.</strong></p>
  <p align="center">
    <img src="https://img.shields.io/badge/binary-~8MB-brightgreen" alt="Binary Size">
    <img src="https://img.shields.io/badge/docker-~28MB-blue" alt="Docker Size">
    <img src="https://img.shields.io/badge/built_with-Go-00ADD8?logo=go" alt="Go">
    <img src="https://img.shields.io/badge/RAM-~10MB-orange" alt="Memory Usage">
    <img src="https://img.shields.io/badge/license-MIT-yellow" alt="License">
    <img src="https://github.com/louisho5/picobot/actions/workflows/docker-publish.yml/badge.svg" alt="Workflow">
  </p>
</p>

---

Love the idea of open-source AI agents like [OpenClaw](https://github.com/openclaw/openclaw) but tired of the bloat? **Picobot** gives you the same power — persistent memory, tool calling, skills, Telegram integration — in a single ~8MB binary that boots in milliseconds.

No Python. No Node. No 500MB container. Just one Go binary and a config file.

## Why Picobot?

| | Picobot | Typical Agent Frameworks |
|---|---|---|
| **Binary size** | ~8MB | 200MB+ (Python + deps) |
| **Docker image** | ~28MB (Alpine) | 500MB–1GB+ |
| **Cold start** | Instant | 5–30 seconds |
| **RAM usage** | ~10MB idle | 200MB–1GB |
| **Dependencies** | Zero (single binary) | Python, pip, venv, Node… |

Picobot runs happily on a **$5/mo VPS**, a Raspberry Pi, or even an old Android phone via Termux.

## Quick Start — 30 seconds

### Docker Run

```sh
docker run -d --name picobot \
  -e OPENAI_API_KEY="your-key" \
  -e OPENAI_API_BASE="https://openrouter.ai/api/v1" \
  -e PICOBOT_MODEL="google/gemini-2.5-flash" \
  -e TELEGRAM_BOT_TOKEN="your-telegram-token" \
  -e HOST_UID=$(id -u) \
  -e HOST_GID=$(id -g) \
  -v $HOME/picobot-data:/home/picobot/.picobot \
  --restart unless-stopped \
  louisho5/picobot:latest
```

All config, memory, and skills are persisted in `$HOME/picobot-data` on your host.  
Prefer a Docker-managed volume instead? `docker volume create picobot-data` and mount it with `-v picobot-data:/home/picobot/.picobot` — no extra permissions needed.

### Docker Compose

Create a `docker-compose.yml`:

```yaml
services:
  picobot:
    image: louisho5/picobot:latest
    container_name: picobot
    restart: unless-stopped
    environment:
      - OPENAI_API_KEY=your-key
      - OPENAI_API_BASE=https://openrouter.ai/api/v1
      - PICOBOT_MODEL=google/gemini-2.5-flash
      - TELEGRAM_BOT_TOKEN=your-telegram-token
      - TELEGRAM_ALLOW_FROM=your-user-id
    volumes:
      - ./picobot-data:/home/picobot/.picobot
```

Then run:

```sh
docker compose up -d
```

### From Source

```sh
go build -o picobot ./cmd/picobot
./picobot onboard                     # creates ~/.picobot config + workspace
./picobot agent -m "Hello!"           # single-shot query
./picobot gateway                     # long-running mode with Telegram
```

## Architecture

Actually the logic is simple and straightforward. Messages flow through a **Chat Hub** (inbound/outbound channels) into the **Agent Loop**, which builds context from memory/sessions/skills, calls the LLM via OpenAI-compatible API, and executes tools (filesystem, exec, web, etc.) before sending replies back through the hub.

<p>
  <img src="how-it-works.png" alt="How Picobot Works" width="600">
</p>

Notes: Channel refers to communication channels (e.g., Telegram, WhatsApp, etc.).

## Features

### 11 Built-in Tools

The agent can take real actions — not just chat:

| Tool | What it does |
|------|-------------|
| `filesystem` | Read, write, list files |
| `exec` | Run shell commands |
| `web` | Fetch web pages and APIs |
| `message` | Send messages to channels |
| `spawn` | Launch background subagents |
| `cron` | Schedule recurring tasks |
| `write_memory` | Persist information across sessions |
| `create_skill` | Create reusable skill packages |
| `list_skills` | List available skills |
| `read_skill` | Read a skill's content |
| `delete_skill` | Remove a skill |

### Persistent Memory

Picobot remembers things between conversations:

- **Daily notes** — auto-organized by date
- **Long-term memory** — survives restarts
- **Ranked recall** — retrieves the most relevant memories for each query

```sh
picobot memory recent --days 7     # what happened this week?
picobot memory rank -q "meeting"   # find relevant memories
```

### Skills System

Teach your agent new tricks. Skills are modular knowledge packages that extend the agent:

```sh
You: "Create a skill for checking weather using curl wttr.in"
Agent: Created skill "weather" — I'll use it from now on.
```

Skills are just markdown files in `~/.picobot/workspace/skills/`. Create them via the agent or manually.

### Telegram Integration

Chat with your agent from your phone. Set up in 2 minutes:

1. Message [@BotFather](https://t.me/BotFather) — `/newbot` — copy the token
2. Add the token to config or pass as `TELEGRAM_BOT_TOKEN` env var
3. Start the communication gateway

See [HOW_TO_START.md](HOW_TO_START.md) for a detailed BotFather walkthrough.

### Heartbeat

A configurable periodic check (default: 60s) that reads `HEARTBEAT.md` for scheduled tasks — like a personal cron with natural language.

## Configuration

Picobot uses a single JSON config at `~/.picobot/config.json`:

```json
{
  "agents": {
    "defaults": {
      "model": "google/gemini-2.5-flash",
      "maxTokens": 8192,
      "temperature": 0.7,
      "maxToolIterations": 200
    }
  },
  "providers": {
    "openai": {
      "apiKey": "sk-or-v1-YOUR_KEY",
      "apiBase": "https://openrouter.ai/api/v1"
    }
  },
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "YOUR_BOT_TOKEN",
      "allowFrom": ["YOUR_USER_ID"]
    }
  }
}
```

Supports any **OpenAI-compatible API** (OpenAI, OpenRouter, Ollama, etc.). See [CONFIG.md](CONFIG.md) for more details.

## CLI Reference

```
picobot version                        # print version
picobot onboard                        # create config + workspace
picobot agent -m "..."                 # one-shot query
picobot agent -M model -m "..."        # query with specific model
picobot gateway                        # start long-running agent
picobot memory read today|long         # read memory
picobot memory append today|long -c "" # append to memory
picobot memory write long -c ""        # overwrite long-term memory
picobot memory recent --days N         # recent N days
picobot memory rank -q "query"         # semantic memory search
```

## Run on Minimal Hardware

Picobot was designed for constrained environments:

```sh
# Raspberry Pi / ARM device
GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot ./cmd/picobot

# Old x86 VPS
GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o picobot ./cmd/picobot
```

Works on any Linux with 256MB RAM. No runtime dependencies. Just copy the binary and run.

## Tech Stack

| Layer | Technology |
|-------|------------|
| Language | [Go](https://go.dev/) 1.25+ |
| CLI framework | [Cobra](https://github.com/spf13/cobra) |
| LLM providers | OpenAI-compatible API (OpenAI, OpenRouter, Ollama, etc.) |
| Telegram | Raw Bot API (no third-party SDK, standard library `net/http`) |
| HTTP / JSON | Go standard library only (`net/http`, `encoding/json`) |
| Container | Alpine Linux 3.20 (multi-stage Docker build) |

Picobot has **one** external dependency (`spf13/cobra` for CLI parsing). Everything else — HTTP clients, JSON handling, Telegram polling, provider integrations — uses the Go standard library.

## Project Structure

```
cmd/picobot/          CLI entry point
embeds/               Embedded assets (sample skills)
internal/
  agent/              Agent loop, context, tools, skills
  chat/               Chat message hub
  channels/           Telegram (more coming)
  config/             Config schema, loader, onboarding
  cron/               Cron scheduler
  heartbeat/          Periodic task checker
  memory/             Memory read/write/rank
  providers/          OpenAI-compatible provider
  session/            Session manager
docker/               Dockerfile, compose, entrypoint
```

## Roadmap

- [x] Add Telegram support
- [ ] Add WhatsApp support
- [ ] Add Discord support
- [x] AI agent with skill creation capability
- [ ] Integrate additional useful default skills
- [ ] Add more tools (email, file processing, etc.)

Want to contribute? Open an issue or PR with your ideas!

## Docs

- [HOW_TO_START.md](HOW_TO_START.md) — step-by-step getting started guide
- [CONFIG.md](CONFIG.md) — full configuration reference
- [DEVELOPMENT.md](DEVELOPMENT.md) — development, testing, and Docker publishing
- [docker/README.md](docker/README.md) — Docker deployment guide

## License

MIT — use it however you want.
