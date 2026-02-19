#!/bin/bash
set -e

PICOBOT_HOME="${PICOBOT_HOME:-/home/picobot/.picobot}"
CONFIG="${PICOBOT_HOME}/config.json"

# Auto-onboard if config doesn't exist yet
if [ ! -f "${CONFIG}" ]; then
  echo "First run detected — running onboard..."
  picobot onboard
  echo "✅ Onboard complete. Config at ${CONFIG}"
  echo ""
  echo "⚠️  You need to configure your API key and model."
  echo "   Mount a config file or set environment variables."
  echo ""
fi

# Helper: apply a jq filter to the config file in-place
apply_jq() {
  local filter="$1"
  TMP=$(mktemp)
  jq "$filter" "${CONFIG}" > "$TMP" && mv "$TMP" "${CONFIG}"
}

# Allow overriding config values via environment variables
if [ -n "${OPENAI_API_KEY}" ]; then
  echo "Applying OPENAI_API_KEY from environment..."
  apply_jq --arg key "${OPENAI_API_KEY}" '.providers.openai.apiKey = $key'
fi

if [ -n "${OPENAI_API_BASE}" ]; then
  echo "Applying OPENAI_API_BASE from environment..."
  apply_jq --arg base "${OPENAI_API_BASE}" '.providers.openai.apiBase = $base'
fi

if [ -n "${TELEGRAM_BOT_TOKEN}" ]; then
  echo "Applying TELEGRAM_BOT_TOKEN from environment..."
  apply_jq --arg token "${TELEGRAM_BOT_TOKEN}" '.channels.telegram.enabled = true | .channels.telegram.token = $token'
fi

if [ -n "${TELEGRAM_ALLOW_FROM}" ]; then
  echo "Applying TELEGRAM_ALLOW_FROM from environment..."
  # Convert comma-separated IDs to JSON array
  ALLOW_JSON=$(echo "${TELEGRAM_ALLOW_FROM}" | jq -R 'split(",")')
  apply_jq --argjson allow "${ALLOW_JSON}" '.channels.telegram.allowFrom = $allow'
fi

if [ -n "${DISCORD_BOT_TOKEN}" ]; then
  echo "Applying DISCORD_BOT_TOKEN from environment..."
  apply_jq --arg token "${DISCORD_BOT_TOKEN}" '.channels.discord.enabled = true | .channels.discord.token = $token'
fi

if [ -n "${DISCORD_ALLOW_FROM}" ]; then
  echo "Applying DISCORD_ALLOW_FROM from environment..."
  # Convert comma-separated IDs to JSON array
  ALLOW_JSON=$(echo "${DISCORD_ALLOW_FROM}" | jq -R 'split(",")')
  apply_jq --argjson allow "${ALLOW_JSON}" '.channels.discord.allowFrom = $allow'
fi

if [ -n "${PICOBOT_MODEL}" ]; then
  echo "Applying PICOBOT_MODEL from environment..."
  apply_jq --arg model "${PICOBOT_MODEL}" '.agents.defaults.model = $model'
fi

echo "Starting picobot $@..."
exec picobot "$@"
