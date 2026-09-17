package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LoadConfig loads config from ~/.picobot/config.json if present, then applies any environment variable overrides on top.
func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	path := filepath.Join(home, ".picobot", "config.json")
	var cfg Config
	f, err := os.Open(path)
	if err == nil {
		defer func() { _ = f.Close() }()
		if err := json.NewDecoder(f).Decode(&cfg); err != nil {
			return Config{}, err
		}
	}
	// env vars always take precedence over the config file, enabling runtime overrides without editing config.json.
	applyEnvOverrides(&cfg)
	return cfg, nil
}

// applyEnvOverrides updates config fields from all environment variables
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("PICOBOT_MODEL"); v != "" {
		cfg.Agents.Defaults.Model = v
	}
	if v := os.Getenv("PICOBOT_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Agents.Defaults.MaxTokens = n
		}
	}
	if v := os.Getenv("PICOBOT_MAX_TOOL_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Agents.Defaults.MaxToolIterations = n
		}
	}
	if v := os.Getenv("PICOBOT_ENABLE_TOOL_ACTIVITY_INDICATOR"); v != "" {
		b := v != "false" && v != "0" && v != "False" && v != "FALSE"
		cfg.Agents.Defaults.EnableToolActivityIndicator = &b
	}
	if v := os.Getenv("PICOBOT_MEMORY_RANKER"); v != "" {
		cfg.Agents.Defaults.MemoryRanker = v
	}
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Channels.Telegram.Token = v
		cfg.Channels.Telegram.Enabled = true
	}
	if v := os.Getenv("TELEGRAM_ALLOW_FROM"); v != "" {
		var allowed []string
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				allowed = append(allowed, part)
			}
		}
		cfg.Channels.Telegram.AllowFrom = allowed
	}
}
