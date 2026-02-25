package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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

	// Telegram and Discord should also be present and disabled.
	if cfg.Channels.Telegram.Enabled {
		t.Error("Telegram should be disabled in the default config")
	}
	if cfg.Channels.Discord.Enabled {
		t.Error("Discord should be disabled in the default config")
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
