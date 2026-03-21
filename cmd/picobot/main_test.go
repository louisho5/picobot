package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/picobot/internal/agent/memory"
	"github.com/local/picobot/internal/config"
)

func TestMemoryCLI_ReadAppendWriteRecent(t *testing.T) {
	// set HOME to a temp dir so onboard writes to temp
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// create default config + workspace
	if _, _, err := config.Onboard(); err != nil {
		t.Fatalf("onboard failed: %v", err)
	}

	// run: picobot memory append today -c "hello"
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"memory", "append", "today", "-c", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("append today failed: %v", err)
	}

	// verify today's file exists
	cfg, _ := config.LoadConfig()
	ws := cfg.Agents.Defaults.Workspace
	if strings.HasPrefix(ws, "~") {
		home, _ := os.UserHomeDir()
		ws = filepath.Join(home, ws[2:])
	}
	memFile := filepath.Join(ws, "memory")
	files, _ := os.ReadDir(memFile)
	found := false
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".md") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected memory files, none found in %s", memFile)
	}

	// write long-term
	cmd = NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"memory", "write", "long", "-c", "LONGCONTENT"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("write long failed: %v", err)
	}

	// read long-term
	cmd = NewRootCmd()
	readBuf := &bytes.Buffer{}
	cmd.SetOut(readBuf)
	cmd.SetArgs([]string{"memory", "read", "long"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("read long failed: %v", err)
	}
	out := readBuf.String()
	if !strings.Contains(out, "LONGCONTENT") {
		t.Fatalf("expected LONGCONTENT in output, got %q", out)
	}

	// recent days
	cmd = NewRootCmd()
	recentBuf := &bytes.Buffer{}
	cmd.SetOut(recentBuf)
	cmd.SetArgs([]string{"memory", "recent", "--days", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("recent failed: %v", err)
	}
	if recentBuf.String() == "" {
		t.Fatalf("expected recent output, got empty")
	}
}

func TestMemoryCLI_Rank(t *testing.T) {
	// set HOME to a temp dir so onboard writes to temp
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// create default config + workspace
	if _, _, err := config.Onboard(); err != nil {
		t.Fatalf("onboard failed: %v", err)
	}

	// append some memories
	cfg, _ := config.LoadConfig()
	ws := cfg.Agents.Defaults.Workspace
	if strings.HasPrefix(ws, "~") {
		home, _ := os.UserHomeDir()
		ws = filepath.Join(home, ws[2:])
	}
	mem := memory.NewMemoryStoreWithWorkspace(ws, 100)
	_ = mem.AppendToday("buy milk and eggs")
	_ = mem.AppendToday("call mom tomorrow")
	_ = mem.AppendToday("milkshake recipe")

	// run rank command
	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"memory", "rank", "-q", "milk", "-k", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rank failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "buy milk") {
		t.Fatalf("expected 'buy milk' in output, got: %q", out)
	}
	if !strings.Contains(out, "milkshake") && !strings.Contains(out, "Important facts") {
		t.Fatalf("expected either 'milkshake' or 'Important facts' in output, got: %q", out)
	}
}

func TestAgentCLI_ModelFlag(t *testing.T) {
	// set HOME to a temp dir so onboard writes to temp
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if _, _, err := config.Onboard(); err != nil {
		t.Fatalf("onboard failed: %v", err)
	}
	// remove OpenAI from config so stub provider is used
	cfgPath, _, _ := config.ResolveDefaultPaths()
	cfg2, _ := config.LoadConfig()
	cfg2.Providers.OpenAI = nil
	_ = config.SaveConfig(cfg2, cfgPath)

	cmd := NewRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"agent", "--model", "stub-model", "-m", "hello"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "(stub) Echo") {
		t.Fatalf("expected stub echo output, got: %q", out)
	}
}
