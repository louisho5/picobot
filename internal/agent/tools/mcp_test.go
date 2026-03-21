package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/local/picobot/internal/mcp"
)

func newTestMCPServer(t *testing.T) (*httptest.Server, *mcp.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      *int64          `json:"id,omitempty"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		type rpcResp struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      *int64          `json:"id,omitempty"`
			Result  json.RawMessage `json:"result,omitempty"`
		}

		switch req.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"capabilities":{}}`)})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(rpcResp{
				JSONRPC: "2.0", ID: req.ID,
				Result: json.RawMessage(`{"tools":[{"name":"upper","description":"uppercases text","inputSchema":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}}]}`),
			})
		case "tools/call":
			var params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &params)
			text := strings.ToUpper(params.Arguments["text"].(string))
			_ = json.NewEncoder(w).Encode(rpcResp{
				JSONRPC: "2.0", ID: req.ID,
				Result: json.RawMessage(`{"content":[{"type":"text","text":"` + text + `"}]}`),
			})
		}
	}))

	client, err := mcp.NewHTTPClient("testsvr", srv.URL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("NewHTTPClient: %v", err)
	}
	return srv, client
}

func TestMCPToolNameAndDescription(t *testing.T) {
	srv, client := newTestMCPServer(t)
	defer srv.Close()
	defer func() { _ = client.Close() }()

	tools := client.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	mcpTool := NewMCPTool(client, "testsvr", tools[0])

	if got := mcpTool.Name(); got != "mcp_testsvr_upper" {
		t.Fatalf("expected name 'mcp_testsvr_upper', got %q", got)
	}
	if !strings.Contains(mcpTool.Description(), "[MCP: testsvr]") {
		t.Fatalf("description should contain server prefix, got %q", mcpTool.Description())
	}
	params := mcpTool.Parameters()
	if params == nil {
		t.Fatal("expected non-nil parameters")
	}
}

func TestMCPToolExecute(t *testing.T) {
	srv, client := newTestMCPServer(t)
	defer srv.Close()
	defer func() { _ = client.Close() }()

	tools := client.Tools()
	mcpTool := NewMCPTool(client, "testsvr", tools[0])

	result, err := mcpTool.Execute(context.Background(), map[string]interface{}{"text": "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "HELLO" {
		t.Fatalf("expected 'HELLO', got %q", result)
	}
}

func TestMCPToolRegistration(t *testing.T) {
	srv, client := newTestMCPServer(t)
	defer srv.Close()
	defer func() { _ = client.Close() }()

	reg := NewRegistry()
	for _, tool := range client.Tools() {
		reg.Register(NewMCPTool(client, "testsvr", tool))
	}

	// Verify the tool is findable via the registry.
	tool := reg.Get("mcp_testsvr_upper")
	if tool == nil {
		t.Fatal("expected to find mcp_testsvr_upper in registry")
	}

	// Verify it shows up in definitions.
	defs := reg.Definitions()
	found := false
	for _, d := range defs {
		if d.Name == "mcp_testsvr_upper" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("mcp_testsvr_upper not found in definitions")
	}
}
