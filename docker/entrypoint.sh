#!/bin/bash
set -e

PICOBOT_USER="${PICOBOT_USER:-picobot}"
PICOBOT_GROUP="${PICOBOT_GROUP:-picobot}"
PICOBOT_HOME="${PICOBOT_HOME:-/home/picobot/.picobot}"

lookup_gid() {
  awk -F: -v group_name="$1" '$1==group_name {print $3}' /etc/group
}

remap_user() {
  local target_uid="$1"
  local target_gid="$2"

  local current_uid
  current_uid=$(id -u "${PICOBOT_USER}" 2>/dev/null || true)
  local current_user_gid
  current_user_gid=$(id -g "${PICOBOT_USER}" 2>/dev/null || true)
  if [ -n "${current_uid}" ]; then
    if [ "${current_uid}" != "${target_uid}" ] || [ -n "${current_user_gid}" ] && [ "${current_user_gid}" != "${target_gid}" ]; then
      deluser "${PICOBOT_USER}" || true
      current_uid=""
    fi
  fi

  local current_gid
  current_gid=$(lookup_gid "${PICOBOT_GROUP}")
  if [ -n "${current_gid}" ] && [ "${current_gid}" != "${target_gid}" ]; then
    delgroup "${PICOBOT_GROUP}" || true
    current_gid=""
  fi
  if [ -z "${current_gid}" ]; then
    addgroup -g "${target_gid}" -S "${PICOBOT_GROUP}"
  fi

  if [ -z "${current_uid}" ]; then
    adduser -S -u "${target_uid}" -G "${PICOBOT_GROUP}" "${PICOBOT_USER}"
  fi

  chown -R "${PICOBOT_USER}:${PICOBOT_GROUP}" /home/picobot
}

if [ "$(id -u)" = "0" ]; then
  CURRENT_UID=$(id -u "${PICOBOT_USER}" 2>/dev/null || true)
  if [ -z "${CURRENT_UID}" ]; then
    CURRENT_UID=100
  fi
  TARGET_UID="${HOST_UID:-${CURRENT_UID}}"
  TARGET_GID="${HOST_GID:-$(lookup_gid "${PICOBOT_GROUP}")}"
  if [ -z "${TARGET_GID}" ]; then
    TARGET_GID=101
  fi

  remap_user "${TARGET_UID}" "${TARGET_GID}"

  exec su-exec "${PICOBOT_USER}" "$0" "$@"
fi


# Auto-onboard if config doesn't exist yet
if [ ! -f "${PICOBOT_HOME}/config.json" ]; then
  echo "First run detected — running onboard..."
  picobot onboard
  echo "✅ Onboard complete. Config at ${PICOBOT_HOME}/config.json"
  echo ""
  echo "⚠️  You need to configure your API key and model."
  echo "   Mount a config file or set environment variables."
  echo ""
fi

# Allow overriding config values via environment variables
if [ -n "${OPENAI_API_KEY}" ]; then
  echo "Applying OPENAI_API_KEY from environment..."
  TMP=$(mktemp)
  cat "${PICOBOT_HOME}/config.json" | \
    sed "s|sk-or-v1-REPLACE_ME|${OPENAI_API_KEY}|g" > "$TMP" && \
    mv "$TMP" "${PICOBOT_HOME}/config.json"
fi

if [ -n "${OPENAI_API_BASE}" ]; then
  echo "Applying OPENAI_API_BASE from environment..."
  TMP=$(mktemp)
  cat "${PICOBOT_HOME}/config.json" | \
    sed "s|https://openrouter.ai/api/v1|${OPENAI_API_BASE}|g" > "$TMP" && \
    mv "$TMP" "${PICOBOT_HOME}/config.json"
fi

if [ -n "${TELEGRAM_BOT_TOKEN}" ]; then
  echo "Applying TELEGRAM_BOT_TOKEN from environment..."
  TMP=$(mktemp)
  # Enable telegram and set token using sed
  cat "${PICOBOT_HOME}/config.json" | \
    sed 's|"enabled": false|"enabled": true|g' | \
    sed "s|\"token\": \"\"|\"token\": \"${TELEGRAM_BOT_TOKEN}\"|g" > "$TMP" && \
    mv "$TMP" "${PICOBOT_HOME}/config.json"
fi

if [ -n "${TELEGRAM_ALLOW_FROM}" ]; then
  echo "Applying TELEGRAM_ALLOW_FROM from environment..."
  TMP=$(mktemp)
  # Convert comma-separated IDs to JSON array: "id1,id2" -> ["id1","id2"]
  ALLOW_JSON=$(echo "${TELEGRAM_ALLOW_FROM}" | sed 's/,/","/g' | sed 's/^/["/' | sed 's/$/"]/') 
  cat "${PICOBOT_HOME}/config.json" | \
    sed "s/\"allowFrom\": null/\"allowFrom\": ${ALLOW_JSON}/g" | \
    sed "s/\"allowFrom\": \[\]/\"allowFrom\": ${ALLOW_JSON}/g" > "$TMP" && \
    mv "$TMP" "${PICOBOT_HOME}/config.json"
fi

if [ -n "${PICOBOT_MODEL}" ]; then
  echo "Applying PICOBOT_MODEL from environment..."
  TMP=$(mktemp)
  cat "${PICOBOT_HOME}/config.json" | \
    sed "s|\"model\": \"stub-model\"|\"model\": \"${PICOBOT_MODEL}\"|g" > "$TMP" && \
    mv "$TMP" "${PICOBOT_HOME}/config.json"
fi

echo "Starting picobot $@..."
exec picobot "$@"
