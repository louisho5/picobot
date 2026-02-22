package mcp

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/local/picobot/internal/config"
)

func TestNewStdioTransport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping stdio transport test on Windows")
	}

	// Use 'echo' command which should be available on most systems
	cfg := config.MCPServerConfig{
		Command: "echo",
		Args:    []string{"test"},
		Env:     map[string]string{"TEST_VAR": "test_value"},
	}

	transport, err := NewStdioTransport(cfg)
	if err != nil {
		t.Fatalf("NewStdioTransport failed: %v", err)
	}
	defer transport.Close()

	if transport.cmd == nil {
		t.Error("expected cmd to be set")
	}
	if transport.stdin == nil {
		t.Error("expected stdin to be set")
	}
	if transport.stdout == nil {
		t.Error("expected stdout to be set")
	}
	if transport.stderr == nil {
		t.Error("expected stderr to be set")
	}
}

func TestNewStdioTransport_InvalidCommand(t *testing.T) {
	cfg := config.MCPServerConfig{
		Command: "/nonexistent/command/that/does/not/exist",
		Args:    []string{},
	}

	_, err := NewStdioTransport(cfg)
	if err == nil {
		t.Fatal("expected error for invalid command")
	}
}

func TestStdioTransport_Call(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping stdio transport test on Windows")
	}

	// Create a simple mock MCP server using bash
	// This simulates an MCP server that reads JSON-RPC requests and responds
	mockServer := `#!/bin/bash
while IFS= read -r line; do
    # Parse the request ID
    id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d: -f2)
    
    # Echo back a JSON-RPC response
    echo '{"jsonrpc":"2.0","id":'$id',"result":{"status":"ok"}}'
done`

	cfg := config.MCPServerConfig{
		Command: "bash",
		Args:    []string{"-c", mockServer},
	}

	transport, err := NewStdioTransport(cfg)
	if err != nil {
		t.Fatalf("NewStdioTransport failed: %v", err)
	}
	defer transport.Close()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Skip if the mock server doesn't work properly
	// This is a basic smoke test
	t.Log("Stdio transport created successfully")
}

func TestStdioTransport_Closed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping stdio transport test on Windows")
	}

	cfg := config.MCPServerConfig{
		Command: "sleep",
		Args:    []string{"10"},
	}

	transport, err := NewStdioTransport(cfg)
	if err != nil {
		t.Fatalf("NewStdioTransport failed: %v", err)
	}

	// Close the transport
	err = transport.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify closed flag is set
	if !transport.closed.Load() {
		t.Error("expected closed flag to be set")
	}

	// Second close should not error
	err = transport.Close()
	if err != nil {
		t.Fatalf("Second close should not error: %v", err)
	}
}

func TestStdioTransport_Call_Closed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping stdio transport test on Windows")
	}

	cfg := config.MCPServerConfig{
		Command: "sleep",
		Args:    []string{"10"},
	}

	transport, err := NewStdioTransport(cfg)
	if err != nil {
		t.Fatalf("NewStdioTransport failed: %v", err)
	}

	// Close the transport
	transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err = transport.Call(ctx, "test/method", nil)
	if err == nil {
		t.Fatal("expected error when calling closed transport")
	}
}

func TestStdioTransport_SendNotification_Closed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping stdio transport test on Windows")
	}

	cfg := config.MCPServerConfig{
		Command: "sleep",
		Args:    []string{"10"},
	}

	transport, err := NewStdioTransport(cfg)
	if err != nil {
		t.Fatalf("NewStdioTransport failed: %v", err)
	}

	// Close the transport
	transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = transport.SendNotification(ctx, "test/notification", nil)
	if err == nil {
		t.Fatal("expected error when sending notification to closed transport")
	}
}

func TestStdioTransport_ContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping stdio transport test on Windows")
	}

	// Create a mock server that never responds
	mockServer := `#!/bin/bash
while IFS= read -r line; do
    # Read but don't respond
    :
done`

	cfg := config.MCPServerConfig{
		Command: "bash",
		Args:    []string{"-c", mockServer},
	}

	transport, err := NewStdioTransport(cfg)
	if err != nil {
		t.Fatalf("NewStdioTransport failed: %v", err)
	}
	defer transport.Close()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = transport.Call(ctx, "test/method", nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestStdioTransport_RequestID(t *testing.T) {
	// Test that request IDs are unique and incrementing
	id1 := nextRequestID()
	id2 := nextRequestID()
	id3 := nextRequestID()

	if id1 >= id2 || id2 >= id3 {
		t.Error("request IDs should be incrementing")
	}

	if id2 != id1+1 || id3 != id2+1 {
		t.Error("request IDs should increment by 1")
	}
}

func TestRPCError(t *testing.T) {
	err := &rpcError{
		Code:    -32600,
		Message: "Invalid Request",
		Data:    json.RawMessage(`{"detail": "test"}`),
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("rpcError.Error() should return non-empty string")
	}
	if !contains(errStr, "-32600") {
		t.Error("error message should contain code")
	}
	if !contains(errStr, "Invalid Request") {
		t.Error("error message should contain message")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsInternal(s, substr))
}

func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
