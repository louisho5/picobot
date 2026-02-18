package mcp

import (
	"testing"

	"github.com/local/picobot/internal/config"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.clients == nil {
		t.Error("clients map is nil")
	}
	if m.tools == nil {
		t.Error("tools map is nil")
	}
}

func TestManager_InitializeServers_NoServers(t *testing.T) {
	m := NewManager()
	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServerConfig{},
	}

	err := m.InitializeServers(cfg)
	if err != nil {
		t.Errorf("InitializeServers() error = %v", err)
	}
}

func TestManager_InitializeServers_DisabledServer(t *testing.T) {
	m := NewManager()
	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServerConfig{
			"test": {
				Command: "echo",
				Args:    []string{"hello"},
			},
		},
	}

	err := m.InitializeServers(cfg)
	if err != nil {
		t.Errorf("InitializeServers() error = %v", err)
	}
}

func TestManager_GetTool_NotFound(t *testing.T) {
	m := NewManager()
	tool := m.GetTool("nonexistent_tool")
	if tool != nil {
		t.Error("GetTool() should return nil for non-existent tool")
	}
}

func TestManager_GetAllTools_Empty(t *testing.T) {
	m := NewManager()
	tools := m.GetAllTools()
	if len(tools) != 0 {
		t.Errorf("GetAllTools() returned %d tools, want 0", len(tools))
	}
}

func TestManager_GetToolsForServer(t *testing.T) {
	m := NewManager()
	// Manually add a tool for testing
	m.tools["github_search"] = &MCPTool{
		serverName: "github",
		toolName:   "search",
	}
	m.tools["github_read"] = &MCPTool{
		serverName: "github",
		toolName:   "read",
	}
	m.tools["other_tool"] = &MCPTool{
		serverName: "other",
		toolName:   "tool",
	}

	keys := m.GetToolsForServer("github")
	if len(keys) != 2 {
		t.Errorf("GetToolsForServer() returned %d tools, want 2", len(keys))
	}
}
