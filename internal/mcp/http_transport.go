package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	protocolVersion    = "2024-11-05"
	protocolVersionHeader = "Mcp-Protocol-Version"
	sessionIDHeader    = "Mcp-Session-Id"
	lastEventIDHeader  = "Last-Event-ID"
)

// HTTPTransport implements Transport over HTTP with SSE support.
type HTTPTransport struct {
	baseURL    string
	client     *http.Client
	sessionID  string
	lastEventID string
	mu         sync.RWMutex
	closed     atomic.Bool
}

// NewHTTPTransport creates a new HTTP-based transport.
func NewHTTPTransport(url string) *HTTPTransport {
	return &HTTPTransport{
		baseURL: strings.TrimSuffix(url, "/"),
		client: &http.Client{
			Timeout: 0, // No timeout - we handle it per-request
		},
	}
}

// Call sends a JSON-RPC request via HTTP POST and returns the result.
// Supports both direct JSON response and SSE streaming.
func (t *HTTPTransport) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	if t.closed.Load() {
		return nil, fmt.Errorf("transport is closed")
	}

	// Build request body
	reqBody := request{
		JSONRPC: "2.0",
		ID:      nextRequestID(),
		Method:  method,
	}

	if params != nil {
		paramsRaw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		reqBody.Params = paramsRaw
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := t.baseURL + "/mcp" // MCP endpoint
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	httpReq.Header.Set(protocolVersionHeader, protocolVersion)

	// Add session ID if we have one
	t.mu.RLock()
	sessionID := t.sessionID
	t.mu.RUnlock()

	if sessionID != "" {
		httpReq.Header.Set(sessionIDHeader, sessionID)
	}

	// Send request
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle session ID from response
	if respSessionID := resp.Header.Get(sessionIDHeader); respSessionID != "" {
		t.mu.Lock()
		t.sessionID = respSessionID
		t.mu.Unlock()
	}

	// Handle response based on content type
	contentType := resp.Header.Get("Content-Type")

	switch {
	case strings.Contains(contentType, "text/event-stream"):
		return t.handleSSEResponse(ctx, resp.Body, reqBody.ID)
	case strings.Contains(contentType, "application/json"):
		return t.handleJSONResponse(resp.Body, reqBody.ID)
	default:
		// Try to read as JSON anyway
		return t.handleJSONResponse(resp.Body, reqBody.ID)
	}
}

// handleJSONResponse handles a direct JSON response.
func (t *HTTPTransport) handleJSONResponse(body io.Reader, expectedID int) (json.RawMessage, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var resp response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Validate response ID matches request ID
	if resp.ID != expectedID {
		return nil, fmt.Errorf("unexpected response ID: got %d, want %d", resp.ID, expectedID)
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// handleSSEResponse handles a Server-Sent Events stream response.
func (t *HTTPTransport) handleSSEResponse(ctx context.Context, body io.Reader, expectedID int) (json.RawMessage, error) {
	reader := bufio.NewReader(body)

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("SSE response timeout: %w", ctx.Err())
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			if ctx.Err() != nil {
				return nil, fmt.Errorf("SSE response timeout: %w", ctx.Err())
			}
			return nil, fmt.Errorf("SSE read error: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse SSE event
		if strings.HasPrefix(line, "id:") {
			eventID := strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			t.mu.Lock()
			t.lastEventID = eventID
			t.mu.Unlock()
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}

			// Try to parse as JSON-RPC response
			var resp response
			if err := json.Unmarshal([]byte(data), &resp); err != nil {
				// Not a valid JSON-RPC message, log and continue
				log.Printf("[MCP HTTP] Skipping non-response SSE event: %s", data)
				continue
			}

			// Check if this is the response we're waiting for
			if resp.ID == expectedID {
				if resp.Error != nil {
					return nil, resp.Error
				}
				return resp.Result, nil
			}
		}
	}

	return nil, fmt.Errorf("no response received in SSE stream")
}

// SendNotification sends a JSON-RPC notification via HTTP POST.
func (t *HTTPTransport) SendNotification(ctx context.Context, method string, params interface{}) error {
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

	notifBody := notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	}

	data, err := json.Marshal(notifBody)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	url := t.baseURL + "/mcp"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(protocolVersionHeader, protocolVersion)

	t.mu.RLock()
	sessionID := t.sessionID
	t.mu.RUnlock()

	if sessionID != "" {
		httpReq.Header.Set(sessionIDHeader, sessionID)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Notifications should return 202 Accepted with no body
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// StartEventStream starts an SSE stream to receive server-initiated messages.
// This runs in a goroutine and calls the handler for each message.
// The goroutine will exit when the context is cancelled or the connection is closed.
func (t *HTTPTransport) StartEventStream(ctx context.Context, handler func(method string, params json.RawMessage)) error {
	if t.closed.Load() {
		return fmt.Errorf("transport is closed")
	}

	url := t.baseURL + "/mcp"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create SSE request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")

	t.mu.RLock()
	sessionID := t.sessionID
	lastEventID := t.lastEventID
	t.mu.RUnlock()

	if sessionID != "" {
		req.Header.Set(sessionIDHeader, sessionID)
	}
	if lastEventID != "" {
		req.Header.Set(lastEventIDHeader, lastEventID)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("SSE request failed: %w", err)
	}

	if resp.StatusCode == http.StatusMethodNotAllowed {
		resp.Body.Close()
		return fmt.Errorf("server does not support SSE stream")
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("SSE request failed with status: %d", resp.StatusCode)
	}

	// Start goroutine to read SSE events
	// The goroutine will exit when:
	// 1. The context is cancelled
	// 2. The connection is closed by server
	// 3. An error occurs during reading
	go t.readSSEEvents(ctx, resp.Body, handler)

	return nil
}

// readSSEEvents reads SSE events from the stream.
func (t *HTTPTransport) readSSEEvents(ctx context.Context, body io.ReadCloser, handler func(method string, params json.RawMessage)) {
	reader := bufio.NewReader(body)
	defer body.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("[MCP HTTP] SSE read error: %v", err)
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse SSE fields
		if strings.HasPrefix(line, "id:") {
			eventID := strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			t.mu.Lock()
			t.lastEventID = eventID
			t.mu.Unlock()
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}

			// Try to parse as JSON-RPC request/notification
			var msg struct {
				JSONRPC string          `json:"jsonrpc"`
				Method  string          `json:"method"`
				Params  json.RawMessage `json:"params,omitempty"`
				ID      int             `json:"id,omitempty"`
			}

			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				log.Printf("[MCP HTTP] Failed to parse SSE message: %v", err)
				continue
			}

			// Only handle notifications/requests (not responses)
			if msg.ID == 0 && msg.Method != "" {
				handler(msg.Method, msg.Params)
			}
		}
	}
}

// Close terminates the session if one exists.
func (t *HTTPTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}

	t.mu.RLock()
	sessionID := t.sessionID
	t.mu.RUnlock()

	if sessionID != "" {
		// Try to terminate the session gracefully
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		url := t.baseURL + "/mcp"
		req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
		if err != nil {
			return err
		}

		req.Header.Set(sessionIDHeader, sessionID)

		resp, err := t.client.Do(req)
		if err != nil {
			log.Printf("[MCP HTTP] Failed to terminate session: %v", err)
			return nil // Don't fail on session termination error
		}
		resp.Body.Close()
	}

	return nil
}

// GetSessionID returns the current session ID (if any).
func (t *HTTPTransport) GetSessionID() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.sessionID
}
