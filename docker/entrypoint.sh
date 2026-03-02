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

# Allow overriding config values via environment variables
if [ -n "${OPENAI_API_KEY}" ]; then
  echo "Applying OPENAI_API_KEY from environment..."
  TMP=$(mktemp)
  jq --arg key "${OPENAI_API_KEY}" '.providers.openai.apiKey = $key' "${CONFIG}" > "$TMP" && mv "$TMP" "${CONFIG}"
fi

if [ -n "${OPENAI_API_BASE}" ]; then
  echo "Applying OPENAI_API_BASE from environment..."
  TMP=$(mktemp)
  jq --arg base "${OPENAI_API_BASE}" '.providers.openai.apiBase = $base' "${CONFIG}" > "$TMP" && mv "$TMP" "${CONFIG}"
fi

if [ -n "${TELEGRAM_BOT_TOKEN}" ]; then
  echo "Applying TELEGRAM_BOT_TOKEN from environment..."
  TMP=$(mktemp)
  jq --arg token "${TELEGRAM_BOT_TOKEN}" '.channels.telegram.enabled = true | .channels.telegram.token = $token' "${CONFIG}" > "$TMP" && mv "$TMP" "${CONFIG}"
fi

if [ -n "${TELEGRAM_ALLOW_FROM}" ]; then
  echo "Applying TELEGRAM_ALLOW_FROM from environment..."
  ALLOW_JSON=$(echo "${TELEGRAM_ALLOW_FROM}" | jq -R 'split(",")')
  TMP=$(mktemp)
  jq --argjson allow "${ALLOW_JSON}" '.channels.telegram.allowFrom = $allow' "${CONFIG}" > "$TMP" && mv "$TMP" "${CONFIG}"
fi

if [ -n "${DISCORD_BOT_TOKEN}" ]; then
  echo "Applying DISCORD_BOT_TOKEN from environment..."
  TMP=$(mktemp)
  jq --arg token "${DISCORD_BOT_TOKEN}" '.channels.discord.enabled = true | .channels.discord.token = $token' "${CONFIG}" > "$TMP" && mv "$TMP" "${CONFIG}"
fi

if [ -n "${DISCORD_ALLOW_FROM}" ]; then
  echo "Applying DISCORD_ALLOW_FROM from environment..."
  ALLOW_JSON=$(echo "${DISCORD_ALLOW_FROM}" | jq -R 'split(",")')
  TMP=$(mktemp)
  jq --argjson allow "${ALLOW_JSON}" '.channels.discord.allowFrom = $allow' "${CONFIG}" > "$TMP" && mv "$TMP" "${CONFIG}"
fi

if [ -n "${PICOBOT_MODEL}" ]; then
  echo "Applying PICOBOT_MODEL from environment..."
  TMP=$(mktemp)
  jq --arg model "${PICOBOT_MODEL}" '.agents.defaults.model = $model' "${CONFIG}" > "$TMP" && mv "$TMP" "${CONFIG}"
fi

if [ -n "${PICOBOT_MAX_TOKENS}" ]; then
  echo "Applying PICOBOT_MAX_TOKENS from environment..."
  TMP=$(mktemp)
  jq --argjson tokens "${PICOBOT_MAX_TOKENS}" '.agents.defaults.maxTokens = $tokens' "${CONFIG}" > "$TMP" && mv "$TMP" "${CONFIG}"
fi

if [ -n "${PICOBOT_MAX_TOOL_ITERATIONS}" ]; then
  echo "Applying PICOBOT_MAX_TOOL_ITERATIONS from environment..."
  TMP=$(mktemp)
  jq --argjson iter "${PICOBOT_MAX_TOOL_ITERATIONS}" '.agents.defaults.maxToolIterations = $iter' "${CONFIG}" > "$TMP" && mv "$TMP" "${CONFIG}"
fi

echo "Starting picobot $@..."
exec picobot "$@"
