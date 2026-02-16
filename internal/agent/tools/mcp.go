package tools

import (
	"context"
	"fmt"
	"log"

	"github.com/local/picobot/internal/mcp"
	"github.com/local/picobot/internal/providers"
)

// MCPToolAdapter wraps the MCP manager to provide tools compatible with Picobot's Tool interface.
type MCPToolAdapter struct {
	manager *mcp.Manager
}

// NewMCPToolAdapter creates a new MCP tool adapter.
func NewMCPToolAdapter(manager *mcp.Manager) *MCPToolAdapter {
	return &MCPToolAdapter{manager: manager}
}

// RegisterMCPTools registers all MCP tools with the given registry.
func (a *MCPToolAdapter) RegisterMCPTools(reg *Registry) {
	if a.manager == nil {
		return
	}

	tools := a.manager.GetAllTools()
	log.Printf("[MCP] Registering %d MCP tools", len(tools))

	for _, def := range tools {
		// Check for collision with existing tools
		if existing := reg.Get(def.Name); existing != nil {
			log.Printf("[MCP] Warning: Tool %s already registered, overwriting", def.Name)
		}
		tool := &mcpToolWrapper{
			manager:     a.manager,
			toolKey:     def.Name,
			description: def.Description,
			schema:      def.Parameters,
		}
		reg.Register(tool)
	}

	// Register the stats tool
	reg.Register(NewMCPStatsTool(a.manager))
}

// GetToolDefinitions returns all MCP tool definitions for the LLM.
func (a *MCPToolAdapter) GetToolDefinitions() []providers.ToolDefinition {
	if a.manager == nil {
		return nil
	}
	return a.manager.GetAllTools()
}

// mcpToolWrapper wraps an MCP tool to implement the Tool interface.
type mcpToolWrapper struct {
	manager     *mcp.Manager
	toolKey     string
	description string
	schema      map[string]interface{}
}

func (t *mcpToolWrapper) Name() string {
	return t.toolKey
}

func (t *mcpToolWrapper) Description() string {
	return t.description
}

func (t *mcpToolWrapper) Parameters() map[string]interface{} {
	return t.schema
}

func (t *mcpToolWrapper) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	result, err := t.manager.ExecuteTool(ctx, t.toolKey, args)
	if err != nil {
		return "", fmt.Errorf("MCP tool %s failed: %w", t.toolKey, err)
	}
	return result, nil
}

// MCPStatsTool provides statistics about MCP tool usage.
type MCPStatsTool struct {
	manager *mcp.Manager
}

// NewMCPStatsTool creates a new MCP stats tool.
func NewMCPStatsTool(manager *mcp.Manager) *MCPStatsTool {
	return &MCPStatsTool{manager: manager}
}

func (t *MCPStatsTool) Name() string {
	return "mcp_stats"
}

func (t *MCPStatsTool) Description() string {
	return "Get statistics about MCP tool usage, including which tools have been called and how many times"
}

func (t *MCPStatsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{},
	}
}

func (t *MCPStatsTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if t.manager == nil {
		return "No MCP manager configured", nil
	}

	usage := t.manager.GetUsage()
	if len(usage) == 0 {
		return "No MCP tools have been used yet in this session.", nil
	}

	result := "MCP Tool Usage Statistics:\n\n"
	for tool, count := range usage {
		result += fmt.Sprintf("  • %s: %d call(s)\n", tool, count)
	}
	return result, nil
}
