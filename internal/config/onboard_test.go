package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitializeWorkspaceCreatesFiles(t *testing.T) {
	d := t.TempDir()
	if err := InitializeWorkspace(d); err != nil {
		t.Fatalf("InitializeWorkspace failed: %v", err)
	}
	// Check a few files
	want := []string{"AGENTS.md", "SOUL.md", "USER.md", "TOOLS.md", "HEARTBEAT.md", filepath.Join("memory", "MEMORY.md")}
	for _, w := range want {
		p := filepath.Join(d, w)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected file %s to exist, err=%v", p, err)
		}
		// read to ensure not empty
		b, _ := os.ReadFile(p)
		if len(b) == 0 {
			t.Fatalf("expected %s to be non-empty", p)
		}
	}

	// Verify embedded skills were extracted
	embeddedSkills := []string{"example", "weather", "cron"}
	for _, skill := range embeddedSkills {
		skillPath := filepath.Join(d, "skills", skill, "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			t.Fatalf("expected embedded skill %s to exist, err=%v", skill, err)
		}
		b, _ := os.ReadFile(skillPath)
		if len(b) == 0 {
			t.Fatalf("expected skill %s SKILL.md to be non-empty", skill)
		}
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	d := t.TempDir()
	cfg := DefaultConfig()
	cfg.Agents.Defaults.Workspace = d
	path := filepath.Join(d, "config.json")
	if err := SaveConfig(cfg, path); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	// load via simple file read
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved config failed: %v", err)
	}
	var parsed Config
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if parsed.Agents.Defaults.Workspace != d {
		t.Fatalf("workspace mismatch: got %s want %s", parsed.Agents.Defaults.Workspace, d)
	}
	// verify provider defaults: OpenAI present with placeholder
	if parsed.Providers.OpenAI == nil || parsed.Providers.OpenAI.APIKey != "sk-or-v1-REPLACE_ME" {
		t.Fatalf("expected default OpenAI API key placeholder, got %v", parsed.Providers.OpenAI)
	}
	if parsed.Providers.OpenAI.APIBase != "https://openrouter.ai/api/v1" {
		t.Fatalf("expected default OpenAI API base, got %q", parsed.Providers.OpenAI.APIBase)
	}
}

func TestDefaultConfig_IncludesWhatsApp(t *testing.T) {
	cfg := DefaultConfig()

	// WhatsApp must be present and disabled by default.
	if cfg.Channels.WhatsApp.Enabled {
		t.Error("WhatsApp should be disabled in the default config")
	}

	// Telegram, Discord, and Slack should also be present and disabled.
	if cfg.Channels.Telegram.Enabled {
		t.Error("Telegram should be disabled in the default config")
	}
	if cfg.Channels.Discord.Enabled {
		t.Error("Discord should be disabled in the default config")
	}
	if cfg.Channels.Slack.Enabled {
		t.Error("Slack should be disabled in the default config")
	}
}

func TestDefaultConfig_WhatsAppRoundTrips(t *testing.T) {
	d := t.TempDir()
	cfg := DefaultConfig()
	cfg.Channels.WhatsApp = WhatsAppConfig{
		Enabled:   true,
		DBPath:    "~/.picobot/whatsapp.db",
		AllowFrom: []string{"15551234567"},
	}

	path := filepath.Join(d, "config.json")
	if err := SaveConfig(cfg, path); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved config failed: %v", err)
	}
	var parsed Config
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	wa := parsed.Channels.WhatsApp
	if !wa.Enabled {
		t.Error("WhatsApp should be enabled after round-trip")
	}
	if wa.DBPath != "~/.picobot/whatsapp.db" {
		t.Errorf("DBPath = %q, want ~/.picobot/whatsapp.db", wa.DBPath)
	}
	if len(wa.AllowFrom) != 1 || wa.AllowFrom[0] != "15551234567" {
		t.Errorf("AllowFrom = %v, want [15551234567]", wa.AllowFrom)
	}
}

// TestLoadConfigFromFile verifies that LoadConfigFromFile reads on-disk values
// without applying environment variable overrides.
func TestLoadConfigFromFile(t *testing.T) {
	// Write a minimal config to a temp file
	cfg := Config{}
	cfg.Agents.Defaults.Model = "on-disk-model"
	cfg.Agents.Defaults.MaxTokens = 1234
	cfg.Providers.Codex = &CodexProviderConfig{
		AccessToken:  "tok",
		RefreshToken: "ref",
		ExpiresAt:    time.Now().Add(10 * time.Minute),
		AccountID:    "acct-999",
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := SaveConfig(cfg, path); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Set env overrides that LoadConfig would apply
	t.Setenv("PICOBOT_MODEL", "env-model")
	t.Setenv("PICOBOT_MAX_TOKENS", "9999")

	// LoadConfigFromFile must ignore those env vars
	got, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFromFile: %v", err)
	}
	if got.Agents.Defaults.Model != "on-disk-model" {
		t.Errorf("model: got %q, want %q", got.Agents.Defaults.Model, "on-disk-model")
	}
	if got.Agents.Defaults.MaxTokens != 1234 {
		t.Errorf("maxTokens: got %d, want 1234", got.Agents.Defaults.MaxTokens)
	}
	if got.Providers.Codex == nil || got.Providers.Codex.AccountID != "acct-999" {
		t.Errorf("Codex token not preserved: %+v", got.Providers.Codex)
	}
}

// TestLoadConfigFromFileMissingReturnsZero verifies that a missing file returns
// an empty config rather than an error.
func TestLoadConfigFromFileMissingReturnsZero(t *testing.T) {
	got, err := LoadConfigFromFile("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if got.Agents.Defaults.Model != "" {
		t.Errorf("expected zero config, got model=%q", got.Agents.Defaults.Model)
	}
}
