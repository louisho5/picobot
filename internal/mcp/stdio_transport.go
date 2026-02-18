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
	defaultShutdownTimeout = 5 * time.Second
)

// pendingCall represents a waiting RPC call.
type pendingCall struct {
	id   int
	done chan *response
}

// StdioTransport implements Transport using stdio (subprocess).
type StdioTransport struct {
	cmd        *exec.Cmd
	stdin      *bufio.Writer
	stdinPipe  *os.File
	stdout     *bufio.Reader
	stderr     *bufio.Reader
	mu         sync.Mutex
	closed     atomic.Bool
	pending    map[int]*pendingCall // guarded by mu
	readerDone chan struct{}        // closed when response reader exits
}

// NewStdioTransport creates a new stdio-based transport.
func NewStdioTransport(cfg config.MCPServerConfig) (*StdioTransport, error) {
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
		stdinFile = nil
	}

	transport := &StdioTransport{
		cmd:        cmd,
		stdin:      bufio.NewWriter(stdin),
		stdinPipe:  stdinFile,
		stdout:     bufio.NewReader(stdout),
		stderr:     bufio.NewReader(stderr),
		pending:    make(map[int]*pendingCall),
		readerDone: make(chan struct{}),
	}

	// Start background goroutines
	go transport.logStderr()
	go transport.readResponses()

	return transport, nil
}

// logStderr logs stderr output from the MCP server.
func (t *StdioTransport) logStderr() {
	scanner := bufio.NewScanner(t.stderr)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			log.Printf("[MCP Server] %s", line)
		}
	}
	if err := scanner.Err(); err != nil && !t.closed.Load() {
		log.Printf("[MCP] Stderr scanner error: %v", err)
	}
}

// readResponses is the SINGLE goroutine that reads from stdout.
// It demuxes responses to the appropriate pending calls.
func (t *StdioTransport) readResponses() {
	defer close(t.readerDone)

	for {
		if t.closed.Load() {
			return
		}

		line, err := t.stdout.ReadString('\n')
		if err != nil {
			if !t.closed.Load() {
				log.Printf("[MCP] Response reader error: %v", err)
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse enough to distinguish a response from a server notification.
		var msg struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      *int            `json:"id"` // pointer: nil means field was absent
			Method  string          `json:"method"`
			Result  json.RawMessage `json:"result,omitempty"`
			Error   *rpcError       `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			log.Printf("[MCP] Failed to parse message: %v", err)
			continue
		}

		// Server-initiated notifications have a method but no id — ignore them.
		if msg.ID == nil {
			if msg.Method != "" {
				log.Printf("[MCP] Received server notification: %s", msg.Method)
			}
			continue
		}

		resp := response{JSONRPC: msg.JSONRPC, ID: *msg.ID, Result: msg.Result, Error: msg.Error}

		// Dispatch to pending call
		t.mu.Lock()
		call, ok := t.pending[resp.ID]
		t.mu.Unlock()

		if ok {
			select {
			case call.done <- &resp:
			default:
				// Receiver already timed out
			}
		} else {
			log.Printf("[MCP] Unexpected response ID: %d (no pending call)", resp.ID)
		}
	}
}

// Call sends a JSON-RPC request and returns the result.
func (t *StdioTransport) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	if t.closed.Load() {
		return nil, fmt.Errorf("transport is closed")
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

	// Register pending call BEFORE sending request
	call := &pendingCall{
		id:   req.ID,
		done: make(chan *response, 1),
	}

	t.mu.Lock()
	if t.closed.Load() {
		t.mu.Unlock()
		return nil, fmt.Errorf("transport is closed")
	}
	t.pending[req.ID] = call
	t.mu.Unlock()

	// Send request (protected by mutex for concurrent safety on stdin)
	t.mu.Lock()
	_, err = t.stdin.Write(append(data, '\n'))
	if err != nil {
		t.mu.Unlock()
		// Clean up pending call
		t.mu.Lock()
		delete(t.pending, req.ID)
		t.mu.Unlock()
		return nil, fmt.Errorf("failed to write request: %w", err)
	}
	err = t.stdin.Flush()
	t.mu.Unlock()
	if err != nil {
		// Clean up pending call
		t.mu.Lock()
		delete(t.pending, req.ID)
		t.mu.Unlock()
		return nil, fmt.Errorf("failed to flush request: %w", err)
	}

	// Wait for response or timeout
	select {
	case resp := <-call.done:
		// Clean up pending call
		t.mu.Lock()
		delete(t.pending, req.ID)
		t.mu.Unlock()

		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		// Clean up pending call
		t.mu.Lock()
		delete(t.pending, req.ID)
		t.mu.Unlock()
		return nil, fmt.Errorf("MCP request timeout: %w", ctx.Err())
	}
}

// SendNotification sends a JSON-RPC notification.
func (t *StdioTransport) SendNotification(ctx context.Context, method string, params interface{}) error {
	if t.closed.Load() {
		return fmt.Errorf("transport is closed")
	}

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	notif := notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed.Load() {
		return fmt.Errorf("transport is closed")
	}

	_, err = t.stdin.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write notification: %w", err)
	}
	return t.stdin.Flush()
}

// Close gracefully shuts down the transport.
func (t *StdioTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}

	if t.stdinPipe != nil {
		_ = t.stdinPipe.Close()
	}

	if t.cmd != nil && t.cmd.Process != nil {
		// Create a channel to signal when Wait() completes
		done := make(chan struct{})
		go func() {
			_ = t.cmd.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(defaultShutdownTimeout):
			// Timeout - force kill
			if err := t.cmd.Process.Kill(); err != nil {
				log.Printf("[MCP] Failed to kill process: %v", err)
			}
			// Wait for the goroutine to complete after kill
			<-done
		}
	}

	// Wait for response reader to exit
	<-t.readerDone

	return nil
}
