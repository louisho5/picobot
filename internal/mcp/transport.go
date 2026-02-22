package mcp

import (
	"context"
	"encoding/json"
)

// Transport is the interface for MCP transports (stdio or HTTP).
type Transport interface {
	// Call sends a JSON-RPC request and returns the result.
	Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error)

	// SendNotification sends a JSON-RPC notification (no response expected).
	SendNotification(ctx context.Context, method string, params interface{}) error

	// Close closes the transport connection.
	Close() error
}
