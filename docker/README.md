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
  -e PICOBOT_MODEL="google/gemini-2.5-flash" \
  -e TELEGRAM_BOT_TOKEN="123456:ABC..." \
  -e TELEGRAM_ALLOW_FROM="8881234567" \
  -e HOST_UID=$(id -u) \
  -e HOST_GID=$(id -g) \
  -v $HOME/picobot-data:/home/picobot/.picobot \
  picobot
```

To avoid touching host permissions, let Docker manage persistence:

```sh
docker volume create picobot-data
docker run -d --name picobot \
  -e OPENAI_API_KEY="sk-or-v1-YOUR_KEY" \
  -v picobot-data:/home/picobot/.picobot \
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
| `HOST_UID` | No | picobot user UID | Host UID to map onto container user |
| `HOST_GID` | No | picobot group GID | Host GID to map onto container user |

## Data Persistence

All data is stored in the `picobot-data` Docker volume:
- `config.json` — configuration
- `workspace/` — bootstrap files, memory, skills

Data persists across container restarts and rebuilds.
