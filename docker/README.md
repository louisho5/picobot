# Docker Deployment

Run Picobot as a Docker container — one command to start.

## Quick Start

### Option 1: Docker Compose (Recommended)

```sh
# 1. Create .env with your API key and settings
nano docker/.env

# 2. Start
docker compose -f docker/docker-compose.yml up -d

# 3. Check logs
docker compose -f docker/docker-compose.yml logs -f
```

### Option 2: Docker Run

```sh
# Build the image
docker build -f docker/Dockerfile -t picobot .

# Run with environment variables
docker run -d \
  --name picobot \
  --restart unless-stopped \
  -e OPENAI_API_KEY="sk-or-v1-YOUR_KEY" \
  -e OPENAI_API_BASE="https://openrouter.ai/api/v1" \
  -e PICOBOT_MODEL="openrouter/free" \
  -e TELEGRAM_BOT_TOKEN="123456:ABC..." \
  -e TELEGRAM_ALLOW_FROM="8881234567" \
  -e DISCORD_BOT_TOKEN="MTIzNDU2..." \
  -e DISCORD_ALLOW_FROM="123456789012345678" \
  -v ./picobot-data:/home/picobot/.picobot \
  picobot
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_API_KEY` | Yes | — | OpenAI-compatible API key (OpenRouter, OpenAI, etc.) |
| `OPENAI_API_BASE` | No | `https://openrouter.ai/api/v1` | OpenAI-compatible API base URL |
| `PICOBOT_MODEL` | No | `google/gemini-2.5-flash` | LLM model to use |
| `TELEGRAM_BOT_TOKEN` | No | — | Telegram bot token from @BotFather |
| `TELEGRAM_ALLOW_FROM` | No | — | Comma-separated Telegram user IDs |
| `DISCORD_BOT_TOKEN` | No | — | Discord bot token from Developer Portal |
| `DISCORD_ALLOW_FROM` | No | — | Comma-separated Discord user IDs |

## Data Persistence

All data is stored in the `picobot-data` Docker volume:
- `config.json` — configuration
- `workspace/` — bootstrap files, memory, skills

Data persists across container restarts and rebuilds.
