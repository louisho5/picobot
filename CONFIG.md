# Configuration Reference

Picobot is configured via `~/.picobot/config.json`. Run `picobot onboard` to generate the default config.

## Full Default Config

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.picobot/workspace",
      "model": "stub-model",
      "maxTokens": 8192,
      "temperature": 0.7,
      "maxToolIterations": 100,
      "heartbeatIntervalS": 60,
      "requestTimeoutS": 60
    }
  },
  "channels": {
    "telegram": {
      "enabled": false,
      "token": "",
      "allowFrom": []
    },
    "discord": {
      "enabled": false,
      "token": "",
      "allowFrom": []
    }
  },
  "providers": {
    "openai": {
      "apiKey": "sk-or-v1-REPLACE_ME",
      "apiBase": "https://openrouter.ai/api/v1"
    }
  }
}
```

---

## agents.defaults

Agent behavior settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `workspace` | string | `~/.picobot/workspace` | Path to the agent's workspace directory. Contains bootstrap files, memory, and skills. |
| `model` | string | `stub-model` | Default LLM model to use. Set to a real model like `google/gemini-2.5-flash`. Can be overridden with the `-M` flag. |
| `maxTokens` | int | `8192` | Maximum tokens for LLM responses. |
| `temperature` | float | `0.7` | LLM temperature (0.0 = deterministic, 1.0 = creative). |
| `maxToolIterations` | int | `100` | Maximum number of tool-calling iterations per request. Prevents infinite loops. |
| `heartbeatIntervalS` | int | `60` | How often (in seconds) the heartbeat checks `HEARTBEAT.md` for periodic tasks. Only used in gateway mode. |
| `requestTimeoutS` | int | `60` | HTTP timeout in seconds for each LLM API request. Increase for slow models or poor network conditions. |

### Model Priority

The model is resolved in this order:
1. **CLI flag** (`-M` / `--model`)
2. **Config** (`agents.defaults.model`)
3. **Provider default** (fallback)

### Example

```json
{
  "agents": {
    "defaults": {
      "workspace": "/home/user/.picobot/workspace",
      "model": "google/gemini-2.5-flash",
      "maxTokens": 16384,
      "temperature": 0.5,
      "maxToolIterations": 200,
      "heartbeatIntervalS": 120,
      "requestTimeoutS": 120
    }
  }
}
```

---

## providers

LLM provider configuration. Picobot uses an OpenAI-compatible API provider.

### providers.openai

Connect to any OpenAI-compatible API service (OpenAI, OpenRouter, Ollama, etc.).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `apiKey` | string | *(required)* | Your API key. Get OpenRouter keys at https://openrouter.ai/keys |
| `apiBase` | string | `https://openrouter.ai/api/v1` | API base URL. Use `https://api.openai.com/v1` for OpenAI, `http://localhost:11434/v1` for local Ollama, or any compatible endpoint. |

```json
{
  "providers": {
    "openai": {
      "apiKey": "sk-or-v1-your-key-here",
      "apiBase": "https://openrouter.ai/api/v1"
    }
  }
}
```

**Examples:**

```json
// OpenAI
{
  "providers": {
    "openai": {
      "apiKey": "sk-proj-...",
      "apiBase": "https://api.openai.com/v1"
    }
  }
}

// Local Ollama (no API key needed)
{
  "providers": {
    "openai": {
      "apiKey": "not-needed",
      "apiBase": "http://localhost:11434/v1"
    }
  }
}
```

### Provider Fallback

If no valid provider is configured, Picobot uses a **Stub** provider (echoes back your message, for testing).

---

## channels

Chat channel integrations. Supports Telegram and Discord.

### channels.telegram

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Set to `true` to start the Telegram bot. |
| `token` | string | `""` | Your Telegram Bot token from [@BotFather](https://t.me/BotFather). |
| `allowFrom` | string[] | `[]` | List of allowed Telegram user IDs. Empty = allow all. |

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
      "allowFrom": ["8881234567"]
    }
  }
}
```

### channels.discord

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Set to `true` to start the Discord bot. |
| `token` | string | `""` | Your Discord Bot token from the [Developer Portal](https://discord.com/developers/applications). |
| `allowFrom` | string[] | `[]` | List of allowed Discord user IDs. Empty = allow all. |

```json
{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "MTIzNDU2Nzg5MDEyMzQ1Njc4OQ.XXXXXX.XXXXXXXXXXXXXXXXXXXXXXXX",
      "allowFrom": ["123456789012345678"]
    }
  }
}
```

The Discord bot uses the Gateway WebSocket API for receiving messages and the REST API for sending. In servers, the bot responds when **mentioned** (`@botname`) or when a message is a **reply** to the bot. In DMs, the bot responds to all messages.

**Required Bot Permissions:**
- Send Messages
- Read Message History

**Required Privileged Intents (enable in Developer Portal → Bot):**
- Message Content Intent

---

## Workspace Files

The workspace directory (default `~/.picobot/workspace`) contains files that shape agent behavior:

| File | Purpose | Who edits |
|------|---------|-----------|
| `SOUL.md` | Agent personality, values, communication style | You (once) |
| `AGENTS.md` | Agent instructions, rules, guidelines | You (once) |
| `USER.md` | Your profile — name, timezone, preferences | You (once) |
| `TOOLS.md` | Tool reference documentation | You (once) |
| `HEARTBEAT.md` | Periodic tasks checked every `heartbeatIntervalS` seconds | You / Agent |
| `memory/MEMORY.md` | Long-term memory | Agent (via write_memory tool) |
| `memory/YYYY-MM-DD.md` | Daily notes | Agent (via write_memory tool) |
| `skills/` | Skill packages | Agent (via skill tools) or you manually |

---

## Example: Minimal Production Config

```json
{
  "agents": {
    "defaults": {
      "workspace": "/home/user/.picobot/workspace",
      "model": "google/gemini-2.5-flash",
      "maxTokens": 8192,
      "temperature": 0.7,
      "maxToolIterations": 200,
      "heartbeatIntervalS": 60
    }
  },
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "YOUR_TELEGRAM_BOT_TOKEN",
      "allowFrom": ["YOUR_TELEGRAM_USER_ID"]
    },
    "discord": {
      "enabled": true,
      "token": "YOUR_DISCORD_BOT_TOKEN",
      "allowFrom": ["YOUR_DISCORD_USER_ID"]
    }
  },
  "providers": {
    "openai": {
      "apiKey": "sk-or-v1-YOUR_KEY",
      "apiBase": "https://openrouter.ai/api/v1"
    }
  }
}
```
