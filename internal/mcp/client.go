package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/local/picobot/internal/config"
)

const (
	defaultTimeout         = 30 * time.Second
	defaultShutdownTimeout = 5 * time.Second
)

// Client is a stdio-based JSON-RPC client for MCP servers.
type Client struct {
	cmd       *exec.Cmd
	stdin     *bufio.Writer
	stdinPipe *os.File // Store for closing
	stdout    *bufio.Reader
	stderr    *bufio.Reader
	mu        sync.Mutex
	closed    atomic.Bool
}

// NewClient starts an MCP server process and returns a Client.
func NewClient(cfg config.MCPServerConfig) (*Client, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)

	// Set environment variables
	if len(cfg.Env) > 0 {
		env := os.Environ()
		for k, v := range cfg.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	// Get the file from the pipe for closing later
	stdinFile, ok := stdin.(*os.File)
	if !ok {
		// Should not happen with StdinPipe, but handle gracefully
		stdinFile = nil
	}

	client := &Client{
		cmd:       cmd,
		stdin:     bufio.NewWriter(stdin),
		stdinPipe: stdinFile,
		stdout:    bufio.NewReader(stdout),
		stderr:    bufio.NewReader(stderr),
	}

	// Start stderr logger
	go client.logStderr()

	return client, nil
}

// logStderr logs stderr output from the MCP server.
func (c *Client) logStderr() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			log.Printf("[MCP Server] %s", line)
		}
	}
	if err := scanner.Err(); err != nil && !c.closed.Load() {
		log.Printf("[MCP] Stderr scanner error: %v", err)
	}
}

// request represents a JSON-RPC request.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
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
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

var requestIDCounter int64

func nextRequestID() int {
	return int(atomic.AddInt64(&requestIDCounter, 1))
}

// call sends a JSON-RPC request and returns the result.
// Uses per-call state to avoid goroutine leaks on timeout.
func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("client is closed")
	}

	// Marshal params
	var paramsRaw json.RawMessage
	var err error
	if params != nil {
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	req := request{
		JSONRPC: "2.0",
		ID:      nextRequestID(),
		Method:  method,
		Params:  paramsRaw,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create per-call state
	type callResult struct {
		resp *response
		err  error
	}
	callDone := make(chan callResult, 1)

	// Start response reader goroutine that will clean itself up
	go func(reqID int, done chan<- callResult) {
		resp, err := c.readResponse(reqID)
		select {
		case done <- callResult{resp: resp, err: err}:
		default:
			// Channel has buffer of 1, so this means nobody is waiting
			// (timeout occurred), just discard
		}
	}(req.ID, callDone)

	// Send request (protected by mutex for concurrent safety)
	c.mu.Lock()
	_, err = c.stdin.Write(append(data, '\n'))
	if err != nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to write request: %w", err)
	}
	err = c.stdin.Flush()
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("failed to flush request: %w", err)
	}

	// Wait for response or timeout
	select {
	case result := <-callDone:
		if result.err != nil {
			return nil, result.err
		}
		if result.resp.Error != nil {
			return nil, result.resp.Error
		}
		return result.resp.Result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("MCP request timeout: %w", ctx.Err())
	}
}

// readResponse reads and parses a JSON-RPC response with the given ID.
func (c *Client) readResponse(expectedID int) (*response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var resp response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			// Log and skip malformed lines
			log.Printf("[MCP] Failed to parse response: %v", err)
			continue
		}

		// Only handle responses (not notifications)
		if resp.ID == expectedID {
			return &resp, nil
		}

		// Unexpected ID, log and continue
		log.Printf("[MCP] Unexpected response ID: got %d, want %d", resp.ID, expectedID)
	}
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

	result, err := c.call(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}

	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		return nil, fmt.Errorf("failed to parse initialize result: %w", err)
	}

	// Send initialized notification
	if err := c.sendNotification(ctx, "notifications/initialized", nil); err != nil {
		log.Printf("[MCP] Failed to send initialized notification: %v", err)
	}

	return &initResult, nil
}

// sendNotification sends a JSON-RPC notification (no response expected).
func (c *Client) sendNotification(ctx context.Context, method string, params interface{}) error {
	if c.closed.Load() {
		return fmt.Errorf("client is closed")
	}

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	req := request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	_, err = c.stdin.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write notification: %w", err)
	}
	return c.stdin.Flush()
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
	result, err := c.call(ctx, "tools/list", nil)
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

	result, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}

	var callResult CallToolResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		return nil, fmt.Errorf("failed to parse tool call result: %w", err)
	}

	return &callResult, nil
}

// Close gracefully shuts down the MCP server process.
func (c *Client) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		// Already closed
		return nil
	}

	// Close stdin to signal EOF to the subprocess (triggers graceful exit)
	if c.stdinPipe != nil {
		_ = c.stdinPipe.Close()
	}

	// Try graceful shutdown
	if c.cmd != nil && c.cmd.Process != nil {
		// Wait for process to exit gracefully
		done := make(chan error, 1)
		go func() {
			done <- c.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(defaultShutdownTimeout):
			// Timeout, force kill
			if err := c.cmd.Process.Kill(); err != nil {
				log.Printf("[MCP] Failed to kill process: %v", err)
			}
			_ = c.cmd.Wait() // Reap the process
		}
	}

	return nil
}
