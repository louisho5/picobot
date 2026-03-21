package tools

import (
	"context"
	"fmt"

	"github.com/local/picobot/internal/mcp"
)

// MCPTool wraps a single MCP server tool to implement the picobot Tool interface.
type MCPTool struct {
	client     *mcp.Client
	serverName string
	tool       mcp.Tool
}

// NewMCPTool creates a Tool that delegates execution to an MCP server.
func NewMCPTool(client *mcp.Client, serverName string, tool mcp.Tool) *MCPTool {
	return &MCPTool{client: client, serverName: serverName, tool: tool}
}

func (t *MCPTool) Name() string {
	return fmt.Sprintf("mcp_%s_%s", t.serverName, t.tool.Name)
}

func (t *MCPTool) Description() string {
	desc := t.tool.Description
	if desc == "" {
		desc = fmt.Sprintf("MCP tool %s from server %s", t.tool.Name, t.serverName)
	}
	return fmt.Sprintf("[MCP: %s] %s", t.serverName, desc)
}

func (t *MCPTool) Parameters() map[string]interface{} {
	return t.tool.InputSchema
}

func (t *MCPTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	return t.client.CallTool(ctx, t.tool.Name, args)
}
