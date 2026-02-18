package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/local/picobot/internal/config"
)

const defaultTimeout = 30 * time.Second

var requestIDCounter int64

func nextRequestID() int {
	return int(atomic.AddInt64(&requestIDCounter, 1))
}

// request represents a JSON-RPC request.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// notification represents a JSON-RPC notification (no id field).
type notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// response represents a JSON-RPC response.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError represents a JSON-RPC error.
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// Client is an MCP client that uses a Transport.
type Client struct {
	transport Transport
}

// NewClient creates a new MCP client with the appropriate transport.
// Uses HTTP transport if URL is provided, otherwise stdio transport.
func NewClient(cfg config.MCPServerConfig) (*Client, error) {
	var transport Transport
	var err error

	if cfg.URL != "" {
		// Use HTTP transport
		transport = NewHTTPTransport(cfg.URL)
		log.Printf("[MCP] Using HTTP transport for %s", cfg.URL)
	} else if cfg.Command != "" {
		// Use stdio transport
		transport, err = NewStdioTransport(cfg)
		if err != nil {
			return nil, err
		}
		log.Printf("[MCP] Using stdio transport for command: %s", cfg.Command)
	} else {
		return nil, fmt.Errorf("MCP server config must have either URL (for HTTP) or Command (for stdio)")
	}

	return &Client{transport: transport}, nil
}

// InitializeResult contains the result of the initialize handshake.
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
	Capabilities    map[string]interface{} `json:"capabilities"`
}

// ServerInfo contains information about the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Initialize performs the MCP initialization handshake.
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"clientInfo": map[string]string{
			"name":    "picobot",
			"version": "0.1.0",
		},
		"capabilities": map[string]interface{}{},
	}

	result, err := c.transport.Call(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}

	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		return nil, fmt.Errorf("failed to parse initialize result: %w", err)
	}

	// Send initialized notification
	if err := c.transport.SendNotification(ctx, "notifications/initialized", nil); err != nil {
		log.Printf("[MCP] Failed to send initialized notification: %v", err)
	}

	return &initResult, nil
}

// ToolDefinition represents an MCP tool definition.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ListToolsResult contains the result of tools/list.
type ListToolsResult struct {
	Tools []ToolDefinition `json:"tools"`
}

// ListTools retrieves all available tools from the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	result, err := c.transport.Call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var listResult ListToolsResult
	if err := json.Unmarshal(result, &listResult); err != nil {
		return nil, fmt.Errorf("failed to parse tools list: %w", err)
	}

	return listResult.Tools, nil
}

// CallToolResult contains the result of a tool call.
type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem represents a content item in a tool result.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// String returns the text content from all content items.
func (r *CallToolResult) String() string {
	var texts []string
	for _, item := range r.Content {
		if item.Type == "text" && item.Text != "" {
			texts = append(texts, item.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}

	result, err := c.transport.Call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}

	var callResult CallToolResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		return nil, fmt.Errorf("failed to parse tool call result: %w", err)
	}

	return &callResult, nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	if c.transport != nil {
		return c.transport.Close()
	}
	return nil
}
