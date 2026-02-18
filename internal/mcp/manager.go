// Package mcp provides Model Context Protocol client functionality.
package mcp

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/local/picobot/internal/config"
	"github.com/local/picobot/internal/providers"
)

// Manager manages multiple MCP server connections.
type Manager struct {
	clients map[string]*Client
	tools   map[string]*MCPTool // key: "serverName/toolName"
	mu      sync.RWMutex
	usage   map[string]int      // tool usage counter
}

// MCPTool wraps an MCP tool for use with Picobot's tool registry.
type MCPTool struct {
	serverName  string
	toolName    string
	client      *Client
	description string
	schema      map[string]interface{}
}

// NewManager creates a new MCP manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
		tools:   make(map[string]*MCPTool),
		usage:   make(map[string]int),
	}
}

// InitializeServers connects to all MCP servers from config.
// All servers defined in the config are started automatically.
func (m *Manager) InitializeServers(cfg config.MCPConfig) error {
	for name, serverCfg := range cfg.Servers {
		if err := m.ConnectServer(name, serverCfg); err != nil {
			log.Printf("[MCP] Failed to connect to server %s: %v", name, err)
			continue
		}
	}
	return nil
}

// ConnectServer connects to a single MCP server.
func (m *Manager) ConnectServer(name string, cfg config.MCPServerConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create MCP client for %s: %w", name, err)
	}

	// Initialize the server
	initResult, err := client.Initialize(ctx)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to initialize MCP server %s: %w", name, err)
	}

	log.Printf("[MCP] Connected to server %s (version: %s)", 
		initResult.ServerInfo.Name, initResult.ServerInfo.Version)

	// Fetch tools
	tools, err := client.ListTools(ctx)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to list tools from %s: %w", name, err)
	}

	log.Printf("[MCP] Server %s provides %d tools", name, len(tools))

	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients[name] = client

	// Register each tool with a namespaced key
	for _, tool := range tools {
		toolKey := fmt.Sprintf("%s_%s", name, tool.Name)
		if existing, ok := m.tools[toolKey]; ok {
			log.Printf("[MCP] Warning: Tool %s already exists (from %s), overwriting with %s", 
				toolKey, existing.serverName, name)
		}
		m.tools[toolKey] = &MCPTool{
			serverName:  name,
			toolName:    tool.Name,
			client:      client,
			description: fmt.Sprintf("[%s] %s", name, tool.Description),
			schema:      tool.Parameters,
		}
	}

	return nil
}

// GetTool returns an MCP tool by its namespaced key.
func (m *Manager) GetTool(key string) *MCPTool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tools[key]
}

// GetAllTools returns all MCP tools as Picobot tool definitions.
func (m *Manager) GetAllTools() []providers.ToolDefinition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	defs := make([]providers.ToolDefinition, 0, len(m.tools))
	for key, tool := range m.tools {
		defs = append(defs, providers.ToolDefinition{
			Name:        key,
			Description: tool.description,
			Parameters:  tool.schema,
		})
	}
	return defs
}

// ExecuteTool executes an MCP tool by its namespaced key.
func (m *Manager) ExecuteTool(ctx context.Context, toolKey string, args map[string]interface{}) (string, error) {
	tool := m.GetTool(toolKey)
	if tool == nil {
		return "", fmt.Errorf("MCP tool not found: %s", toolKey)
	}

	result, err := tool.client.CallTool(ctx, tool.toolName, args)
	if err != nil {
		return "", fmt.Errorf("MCP tool execution failed: %w", err)
	}

	if result.IsError {
		return "", fmt.Errorf("MCP tool returned error")
	}

	// Track usage
	m.mu.Lock()
	m.usage[toolKey]++
	m.mu.Unlock()

	// Concatenate all content (text and other types)
	var output strings.Builder
	for _, content := range result.Content {
		switch content.Type {
		case "text":
			output.WriteString(content.Text)
		case "image":
			output.WriteString("[image content]")
		default:
			output.WriteString(fmt.Sprintf("[%s content]", content.Type))
		}
	}

	return output.String(), nil
}

// GetUsage returns usage statistics for MCP tools.
func (m *Manager) GetUsage() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	usage := make(map[string]int, len(m.usage))
	for k, v := range m.usage {
		usage[k] = v
	}
	return usage
}

// GetToolsForServer returns all tool keys for a specific server.
func (m *Manager) GetToolsForServer(serverName string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []string
	prefix := serverName + "_"
	for key := range m.tools {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	return keys
}

// Close shuts down all MCP connections.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		log.Printf("[MCP] Closing connection to %s", name)
		if err := client.Close(); err != nil {
			log.Printf("[MCP] Error closing client %s: %v", name, err)
		}
	}
}
