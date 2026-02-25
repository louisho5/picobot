# How to Start Using Picobot

## Prerequisites

- **Go 1.26+** installed ([download](https://go.dev/dl/))
- An **API key** for an OpenAI-compatible service:
  - [OpenRouter](https://openrouter.ai/keys) (recommended, supports many models)
  - [OpenAI](https://platform.openai.com/api-keys)
  - Or use a local [Ollama](https://ollama.ai) instance (no key needed)

## Step 1: Build

Picobot is a single static binary with no runtime dependencies.

### Choose your variant

Picobot ships in two variants:

| Variant | Build command | Binary size | WhatsApp |
|---------|--------------|-------------|----------|
| **Full** (default) | `go build ./cmd/picobot` | ~31 MB | ✅ included |
| **Lite** | `go build -tags lite ./cmd/picobot` | ~13 MB | ❌ excluded |

The **lite** build is designed for resource-constrained environments (IoT, cheap VPS, minimal servers) where every megabyte matters. It includes all core features — agent, Telegram, Discord, memory, skills, cron — but strips out large optional packages like WhatsApp. If you don't need WhatsApp (or other heavy integrations added in the future), lite is the right choice.

The **full** build is the default. If you're unsure, start here.

### Build from source

```sh
git clone <repo-url>
cd picobot

# Full build (includes WhatsApp)
go build -o picobot ./cmd/picobot

# Lite build (smaller, no WhatsApp)
go build -tags lite -o picobot ./cmd/picobot
```

### Build all platforms at once (Makefile)

Use `make` to cross-compile every platform in one shot:

```sh
make build
```

This produces six binaries:

| File | Platform | Variant |
|------|----------|---------|
| `picobot_linux_amd64` | Linux x86-64 | Full |
| `picobot_linux_arm64` | Linux ARM64 | Full |
| `picobot_mac_arm64` | macOS Apple Silicon | Full |
| `picobot_linux_amd64_lite` | Linux x86-64 | Lite |
| `picobot_linux_arm64_lite` | Linux ARM64 | Lite |
| `picobot_mac_arm64_lite` | macOS Apple Silicon | Lite |

You can also build individual targets:

```sh
make linux_amd64        # full, Linux x86-64
make linux_arm64_lite   # lite, Linux ARM64
make clean              # remove all built binaries
```

## Step 2: Onboard

Run the onboard command to create the config and workspace:

```sh
./picobot onboard
```

This creates:
- `~/.picobot/config.json` — your configuration file
- `~/.picobot/workspace/` — the agent's workspace with bootstrap files:
  - `SOUL.md` — agent personality and values
  - `AGENTS.md` — agent instructions and guidelines
  - `USER.md` — your profile (customize this!)
  - `TOOLS.md` — documentation of all available tools
  - `HEARTBEAT.md` — periodic tasks
  - `memory/MEMORY.md` — long-term memory
  - `skills/example/SKILL.md` — example skill

## Step 3: Configure API Key

Edit `~/.picobot/config.json` and replace the placeholder API key:

```sh
# Open in your editor
nano ~/.picobot/config.json
```

Change `"sk-or-v1-REPLACE_ME"` to your actual API key.

Also set your preferred model (e.g., `google/gemini-2.5-flash` for OpenRouter, `gpt-4o-mini` for OpenAI):

```json
{
  "agents": {
    "defaults": {
      "model": "google/gemini-2.5-flash"
    }
  },
  "providers": {
    "openai": {
      "apiKey": "sk-or-v1-YOUR_ACTUAL_KEY",
      "apiBase": "https://openrouter.ai/api/v1"
    }
  }
}
```

## Step 4: Customize Your Profile

Edit `~/.picobot/workspace/USER.md` to fill in your name, timezone, preferences, etc. This helps the agent personalize its responses.

## Step 5: Try It!

### Single-shot query

```sh
./picobot agent -m "Hello, what tools do you have?"
```

### Use a specific model

```sh
./picobot agent -M "google/gemini-2.5-flash" -m "What is 2+2?"
```

### Login to channels (Telegram, Discord, WhatsApp)

```sh
./picobot channels login
```

### Start the gateway (long-running mode)

```sh
./picobot gateway
```

This starts the agent loop, heartbeat, and any enabled channels (e.g., Telegram, Discord).

## CLI Commands

| Command | Description |
|---------|-------------|
| `picobot version` | Print version |
| `picobot onboard` | Create default config and workspace |
| `picobot channels login` | Interactively connect Telegram, Discord, or WhatsApp |
| `picobot agent -m "..."` | Run a single-shot agent query |
| `picobot agent -M model -m "..."` | Query with a specific model |
| `picobot gateway` | Start long-running gateway |
| `picobot memory read today` | Read today's memory notes |
| `picobot memory read long` | Read long-term memory |
| `picobot memory append today -c "..."` | Append to today's notes |
| `picobot memory append long -c "..."` | Append to long-term memory |
| `picobot memory write long -c "..."` | Overwrite long-term memory |
| `picobot memory recent -days 7` | Show recent 7 days' notes |
| `picobot memory rank -q "query"` | Rank memories by relevance |

## Available Tools

The agent has access to 11 tools:

| Tool | Purpose |
|------|---------|
| `message` | Send messages to channels |
| `filesystem` | Read, write, list files |
| `exec` | Run shell commands |
| `web` | Fetch web content from URLs |
| `spawn` | Spawn background subagent |
| `cron` | Schedule cron jobs |
| `write_memory` | Persist information to memory |
| `create_skill` | Create a new skill |
| `list_skills` | List available skills |
| `read_skill` | Read a skill's content |
| `delete_skill` | Delete a skill |

## Setting Up Telegram (BotFather Guide)

To chat with Picobot on Telegram, you need to create a bot via **@BotFather**.

### Quick setup (recommended)

Run the interactive channel login wizard:

```sh
./picobot channels login
```

Select **1) Telegram**, then follow the prompts — it will ask for your bot token and your user ID, enable the channel, and save the config automatically.

### Manual setup

If you prefer to edit the config directly, follow the steps below.

### 1. Open BotFather

Open Telegram and search for [@BotFather](https://t.me/BotFather), or click the link directly. This is Telegram's official bot for creating and managing bots.

### 2. Create a New Bot

Send the command:

```
/newbot
```

BotFather will ask you two questions:

1. **Bot name** — A display name (e.g., `My Picobot`)
2. **Bot username** — A unique username ending in `bot` (e.g., `my_picobot_bot`)

### 3. Copy the Token

After creation, BotFather will reply with a message like:

```
Done! Congratulations on your new bot. You will find it at t.me/my_picobot_bot.
Use this token to access the HTTP API:
123456789:ABCdefGHIjklMNOpqrsTUVwxyz
```

Copy the token — you'll need it in the next step.

### 4. Get Your Telegram User ID

To restrict who can talk to your bot, you need your numeric Telegram user ID.

Send a message to [@userinfobot](https://t.me/userinfobot) on Telegram — it will reply with your user ID (a number like `8881234567`).

### 5. Configure Picobot

#### Option 1

Run the interactive channel login wizard:

```sh
./picobot channels login
```

Select **1) Telegram**, then follow the prompts — it will ask for your bot token and your user ID, enable the channel, and save the config automatically.

#### Option 2

Edit `~/.picobot/config.json` and add your Telegram settings:

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
      "allowFrom": ["8881234567"]
    }
  }
}
```

| Field | Description |
|-------|-------------|
| `enabled` | Set to `true` to activate the Telegram channel |
| `token` | The bot token from BotFather |
| `allowFrom` | List of user IDs allowed to chat. Empty `[]` = anyone can use it |

### 6. Start the Gateway

```sh
./picobot gateway
```

Now open Telegram, find your bot by its username, and send it a message. Picobot will respond!

### Optional: Customize Your Bot in BotFather

You can also send these commands to @BotFather to polish your bot:

| Command | What it does |
|---------|-------------|
| `/setdescription` | Short description shown on the bot's profile |
| `/setabouttext` | "About" text in the bot info page |
| `/setuserpic` | Upload a profile photo for your bot |
| `/setcommands` | Set the bot's command menu (e.g., `/start`) |
| `/mybots` | Manage all your bots |

---

## Setting Up Discord

To connect Picobot to Discord, you need to create a bot application in the Discord Developer Portal.

### Quick setup (recommended)

Run the interactive channel login wizard:

```sh
./picobot channels login
```

Select **2) Discord**, then follow the prompts — it will ask for your bot token and your user ID, enable the channel, and save the config automatically.

### Manual setup

If you prefer to edit the config directly, follow the steps below.

### 1. Create a Discord Application

Go to the [Discord Developer Portal](https://discord.com/developers/applications) and click **"New Application"**. Give it a name (e.g., `Picobot`).

### 2. Create a Bot

In your application, go to the **Bot** tab and click **"Add Bot"**. This creates a bot user for your application.

### 3. Enable Message Content Intent

In the **Bot** tab, scroll down to **Privileged Gateway Intents** and enable:
- **Message Content Intent** — required for the bot to read message content

### 4. Copy the Bot Token

In the **Bot** tab, click **"Reset Token"** to generate a new token. Copy it — you'll need it in the next step.

> ⚠️ Keep your bot token secret! Anyone with the token can control your bot.

### 5. Get Your Discord User ID

Enable **Developer Mode** in Discord (Settings → Advanced → Developer Mode). Then right-click your username and select **"Copy User ID"**. This is a number like `123456789012345678`.

### 6. Invite the Bot to Your Server

Go to the **OAuth2** tab → **URL Generator**:
1. Select the `bot` scope
2. Select permissions: **Send Messages**, **Read Message History**
3. Copy the generated URL and open it in your browser
4. Select the server to add the bot to

### 7. Configure Picobot

#### Option 1

Run the interactive channel login wizard:

```sh
./picobot channels login
```

Select **2) Discord**, then follow the prompts — it will ask for your bot token and your user ID, enable the channel, and save the config automatically.

#### Option 2

Edit `~/.picobot/config.json` and add your Discord settings:

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

| Field | Description |
|-------|-------------|
| `enabled` | Set to `true` to activate the Discord channel |
| `token` | The bot token from the Developer Portal |
| `allowFrom` | List of Discord user IDs allowed to chat. Empty `[]` = anyone can use it |

### 8. Start the Gateway

```sh
./picobot gateway
```

Now mention your bot in a Discord server (`@Picobot hello!`) or send it a DM. Picobot will respond!

**How the bot responds:**
- **In servers** — only when @-mentioned (e.g. `@Picobot what's the weather?`)
- **In DMs** — responds to every message

---

## Setting Up WhatsApp

Picobot can receive and reply to WhatsApp messages. It uses [whatsmeow](https://github.com/tulir/whatsmeow) — a Go implementation of the WhatsApp Web protocol, so no phone stays open; the session is stored in a local SQLite database.

> **One-time pairing is required.** You need physical access to the phone that will be linked. After pairing, the bot runs headlessly.

> **Full build required.** WhatsApp is not included in the lite build. If you built with `-tags lite`, rebuild without it.

### 1. Run the Channel Login Wizard

```sh
./picobot channels login
```

Select **3) WhatsApp**. This will:
1. Display a QR code in the terminal
2. Wait for you to scan it with WhatsApp on your phone:
   - Open WhatsApp → **Settings** → **Linked Devices** → **Link a Device**
3. Sync with the phone (takes ~15 seconds)
4. **Automatically update** `~/.picobot/config.json` with `enabled: true` and the correct `dbPath`

You should see:

```
Which channel would you like to connect?

  1) Telegram
  2) Discord
  3) WhatsApp

Enter 1, 2 or 3: 3

=== WhatsApp Setup ===

Scan the QR code below with WhatsApp on your phone:
(Open WhatsApp > Settings > Linked Devices > Link a Device)

[QR code appears here]

Pairing successful, finishing setup...
Syncing with phone...
Successfully authenticated!
Logged in as: 85298765432

WhatsApp setup complete! Run 'picobot gateway' to start.
```

### 2. Find Your Sender ID (LID)

Modern WhatsApp accounts use an internal **LID** (Linked ID) number instead of the phone number for message routing. When you start the gateway the first time, it logs both:

```
whatsapp: connected as 85298765432 (LID: 169032883908635)
```

Use the **LID number** (e.g. `169032883908635`) in `allowFrom` — not the phone number.

> **Why?** WhatsApp internally addresses messages with the LID on newer accounts. If you use the phone number in `allowFrom`, messages will be silently dropped.

### 3. Configure allowFrom

Edit `~/.picobot/config.json` to set who can send messages:

```json
{
  "channels": {
    "whatsapp": {
      "enabled": true,
      "dbPath": "/Users/you/.picobot/whatsapp.db",
      "allowFrom": ["169032883908635"]
    }
  }
}
```

| Field | Description |
|-------|-------------|
| `enabled` | `true` to activate the WhatsApp channel |
| `dbPath` | Path to the SQLite session file (auto-set by `picobot channels login`) |
| `allowFrom` | List of LID numbers allowed to send messages. Empty `[]` = anyone can send |

**To allow yourself only**, add your own LID. **To allow all**, leave `allowFrom` as `[]`.

### 4. Texting Yourself (Notes to Self)

You can use WhatsApp's **"Notes to Self"** chat to interact with Picobot — just open your own name in WhatsApp contacts and send a message. Self-chat always bypasses the `allowFrom` list.

### 5. Start the Gateway

```sh
./picobot gateway
```

You should see:

```
whatsapp: connected as 85298765432 (LID: 169032883908635)
```

Send a message from your allowed number (or from Notes to Self) — Picobot will reply.

### Running in Docker

WhatsApp requires a **one-time interactive QR scan** before the bot can run headlessly. Use `docker compose run` with a TTY for the initial pairing:

```sh
# Step 1: Pair (interactive — scan the QR with your phone)
docker compose run --rm -it picobot channels login
# Select "3" for WhatsApp and scan the QR code.
# The SQLite session DB is saved into ./picobot-data/

# Step 2: Re-start container
docker compose down 
docker compose up -d
```

The session is stored in the `./picobot-data` volume — as long as that directory persists, you won't need to re-scan the QR code.

---

## Next Steps

- Edit `SOUL.md` to change the agent's personality
- Edit `AGENTS.md` to add custom instructions
- Ask the agent to create skills for tasks you do often
- Enable Telegram in `config.json` to chat with your bot on mobile
- Enable Discord in `config.json` to chat with your bot on Discord
- See [CONFIG.md](CONFIG.md) for all configuration options
