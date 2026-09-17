package config

import (
	"encoding/json"
	"os"
	"testing"
)

func TestApplyEnvOverrides(t *testing.T) {
	// Set test environment variables
	os.Setenv("TELEGRAM_BOT_TOKEN", "env-telegram-token")
	os.Setenv("TELEGRAM_ALLOW_FROM", " 12345, 67890 ,  13579 ")
	defer func() {
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("TELEGRAM_ALLOW_FROM")
	}()

	var cfg Config
	applyEnvOverrides(&cfg)

	if !cfg.Channels.Telegram.Enabled {
		t.Error("expected Telegram channel to be enabled")
	}
	if cfg.Channels.Telegram.Token != "env-telegram-token" {
		t.Errorf("expected Token to be 'env-telegram-token', got %q", cfg.Channels.Telegram.Token)
	}

	expectedAllow := []string{"12345", "67890", "13579"}
	if len(cfg.Channels.Telegram.AllowFrom) != len(expectedAllow) {
		t.Fatalf("expected %d allowed users, got %d", len(expectedAllow), len(cfg.Channels.Telegram.AllowFrom))
	}
	for i, v := range expectedAllow {
		if cfg.Channels.Telegram.AllowFrom[i] != v {
			t.Errorf("expected AllowFrom[%d] to be %q, got %q", i, v, cfg.Channels.Telegram.AllowFrom[i])
		}
	}
}

func TestUnmarshalTelegramConfig(t *testing.T) {
	jsonData := `{
		"enabled": true,
		"token": "test-token",
		"allowFrom": [
			1269963921,
			"9876543210"
		]
	}`

	var tc TelegramConfig
	if err := json.Unmarshal([]byte(jsonData), &tc); err != nil {
		t.Fatalf("failed to unmarshal TelegramConfig: %v", err)
	}

	if !tc.Enabled {
		t.Error("expected Enabled to be true")
	}
	if tc.Token != "test-token" {
		t.Errorf("expected token 'test-token', got %q", tc.Token)
	}

	expected := []string{"1269963921", "9876543210"}
	if len(tc.AllowFrom) != len(expected) {
		t.Fatalf("expected %d elements, got %d", len(expected), len(tc.AllowFrom))
	}
	for i, v := range expected {
		if tc.AllowFrom[i] != v {
			t.Errorf("expected AllowFrom[%d] to be %q, got %q", i, v, tc.AllowFrom[i])
		}
	}
}
