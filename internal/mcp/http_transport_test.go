package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPTransport(t *testing.T) {
	transport := NewHTTPTransport("https://example.com/mcp")
	if transport == nil {
		t.Fatal("NewHTTPTransport returned nil")
	}
	if transport.baseURL != "https://example.com/mcp" {
		t.Errorf("expected baseURL to be 'https://example.com/mcp', got %s", transport.baseURL)
	}
	if transport.client == nil {
		t.Error("expected client to be initialized")
	}
}

func TestNewHTTPTransport_TrailingSlash(t *testing.T) {
	transport := NewHTTPTransport("https://example.com/mcp/")
	if transport.baseURL != "https://example.com/mcp" {
		t.Errorf("expected trailing slash to be removed, got %s", transport.baseURL)
	}
}

func TestHTTPTransport_Call_JSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type header to be application/json")
		}
		if !strings.Contains(r.Header.Get("Accept"), "application/json") {
			t.Error("expected Accept header to contain application/json")
		}
		if r.Header.Get(protocolVersionHeader) == "" {
			t.Error("expected protocol version header")
		}

		// Verify request body
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Method != "test/method" {
			t.Errorf("expected method 'test/method', got %s", req.Method)
		}

		// Send JSON response
		w.Header().Set("Content-Type", "application/json")
		resp := response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"result": "success"}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := transport.Call(ctx, "test/method", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var resultMap map[string]string
	if err := json.Unmarshal(result, &resultMap); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if resultMap["result"] != "success" {
		t.Errorf("expected result 'success', got %s", resultMap["result"])
	}
}

func TestHTTPTransport_Call_SSEResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Accept header includes SSE
		if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			t.Error("expected Accept header to contain text/event-stream")
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Send SSE stream
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}

		// Send event with ID
		fmt.Fprintf(w, "id: event-1\n")
		flusher.Flush()

		// Send JSON-RPC response in SSE format
		resp := response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"data": "streamed"}`),
		}
		respJSON, _ := json.Marshal(resp)
		fmt.Fprintf(w, "data: %s\n\n", respJSON)
		flusher.Flush()
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := transport.Call(ctx, "stream/method", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var resultMap map[string]string
	if err := json.Unmarshal(result, &resultMap); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if resultMap["data"] != "streamed" {
		t.Errorf("expected data 'streamed', got %s", resultMap["data"])
	}

	// Verify last event ID was recorded
	if transport.lastEventID != "event-1" {
		t.Errorf("expected lastEventID to be 'event-1', got %s", transport.lastEventID)
	}
}

func TestHTTPTransport_Call_RPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		var req request
		json.NewDecoder(r.Body).Decode(&req)

		resp := response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32600,
				Message: "Invalid request",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := transport.Call(ctx, "test/method", nil)
	if err == nil {
		t.Fatal("expected error for RPC error response")
	}
	if !strings.Contains(err.Error(), "Invalid request") {
		t.Errorf("expected error message to contain 'Invalid request', got: %v", err)
	}
}

func TestHTTPTransport_Call_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := transport.Call(ctx, "test/method", nil)
	if err == nil {
		t.Fatal("expected error for HTTP error response")
	}
}

func TestHTTPTransport_Call_SessionManagement(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		// First request - no session ID, set one in response
		// Second request - should have session ID
		if requestCount == 1 {
			if r.Header.Get(sessionIDHeader) != "" {
				t.Error("first request should not have session ID")
			}
			w.Header().Set(sessionIDHeader, "test-session-123")
		} else {
			if r.Header.Get(sessionIDHeader) != "test-session-123" {
				t.Errorf("expected session ID 'test-session-123', got %s", r.Header.Get(sessionIDHeader))
			}
		}

		w.Header().Set("Content-Type", "application/json")
		var req request
		json.NewDecoder(r.Body).Decode(&req)
		resp := response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"count": ` + fmt.Sprintf("%d", requestCount) + `}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First call - should establish session
	_, err := transport.Call(ctx, "test/method", nil)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Verify session ID was stored
	if transport.sessionID != "test-session-123" {
		t.Errorf("expected session ID 'test-session-123', got %s", transport.sessionID)
	}

	// Second call - should use session ID
	_, err = transport.Call(ctx, "test/method", nil)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if requestCount != 2 {
		t.Errorf("expected 2 requests, got %d", requestCount)
	}
}

func TestHTTPTransport_SendNotification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Method != "notifications/test" {
			t.Errorf("expected method 'notifications/test', got %s", req.Method)
		}
		if req.ID != 0 {
			t.Error("notification should not have an ID")
		}

		// Notifications return 202 Accepted
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := transport.SendNotification(ctx, "notifications/test", map[string]string{"data": "value"})
	if err != nil {
		t.Fatalf("SendNotification failed: %v", err)
	}
}

func TestHTTPTransport_SendNotification_UnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := transport.SendNotification(ctx, "notifications/test", nil)
	if err == nil {
		t.Fatal("expected error for bad request")
	}
}

func TestHTTPTransport_Close(t *testing.T) {
	deleteReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			// Initialize - set session ID
			w.Header().Set(sessionIDHeader, "session-to-close")
			w.Header().Set("Content-Type", "application/json")
			var req request
			json.NewDecoder(r.Body).Decode(&req)
			resp := response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{}`),
			}
			json.NewEncoder(w).Encode(resp)
		case "DELETE":
			// Session termination
			deleteReceived = true
			if r.Header.Get(sessionIDHeader) != "session-to-close" {
				t.Error("DELETE request should have session ID")
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Initialize session
	_, err := transport.Call(ctx, "initialize", nil)
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}

	// Close should send DELETE
	err = transport.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !deleteReceived {
		t.Error("expected DELETE request for session termination")
	}
}

func TestHTTPTransport_Close_NoSession(t *testing.T) {
	transport := NewHTTPTransport("https://example.com/mcp")
	err := transport.Close()
	if err != nil {
		t.Fatalf("Close should not fail without session: %v", err)
	}
}

func TestHTTPTransport_Closed(t *testing.T) {
	transport := NewHTTPTransport("https://example.com/mcp")
	transport.closed.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := transport.Call(ctx, "test/method", nil)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected 'closed' error, got: %v", err)
	}

	err = transport.SendNotification(ctx, "test/notification", nil)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected 'closed' error for notification, got: %v", err)
	}
}

func TestHTTPTransport_StartEventStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET method for SSE, got %s", r.Method)
		}
		if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			t.Error("expected Accept header to contain text/event-stream")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not support flushing")
		}

		// Send a server-initiated notification
		notification := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "notifications/message",
			"params":  map[string]string{"message": "hello"},
		}
		notifJSON, _ := json.Marshal(notification)
		fmt.Fprintf(w, "data: %s\n\n", notifJSON)
		flusher.Flush()
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	messageReceived := make(chan string, 1)
	handler := func(method string, params json.RawMessage) {
		if method == "notifications/message" {
			var paramsMap map[string]string
			json.Unmarshal(params, &paramsMap)
			messageReceived <- paramsMap["message"]
		}
	}

	err := transport.StartEventStream(ctx, handler)
	if err != nil {
		t.Fatalf("StartEventStream failed: %v", err)
	}

	// Wait for message
	select {
	case msg := <-messageReceived:
		if msg != "hello" {
			t.Errorf("expected message 'hello', got %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for SSE message")
	}
}

func TestHTTPTransport_StartEventStream_NotSupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := transport.StartEventStream(ctx, func(string, json.RawMessage) {})
	if err == nil {
		t.Fatal("expected error for 405 response")
	}
	if !strings.Contains(err.Error(), "does not support SSE") {
		t.Errorf("expected 'does not support SSE' error, got: %v", err)
	}
}

func TestHTTPTransport_GetSessionID(t *testing.T) {
	transport := NewHTTPTransport("https://example.com/mcp")

	// Initially empty
	if transport.GetSessionID() != "" {
		t.Error("expected empty session ID initially")
	}

	// Set session ID
	transport.sessionID = "test-session"
	if transport.GetSessionID() != "test-session" {
		t.Errorf("expected session ID 'test-session', got %s", transport.GetSessionID())
	}
}

func TestHTTPTransport_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		var req request
		json.NewDecoder(r.Body).Decode(&req)
		resp := response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	transport := NewHTTPTransport(server.URL)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := transport.Call(ctx, "test/method", nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Errorf("expected context error, got: %v", err)
	}
}
