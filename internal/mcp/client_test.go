package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientInitializeAndListTools(t *testing.T) {
	var calls []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		calls = append(calls, req.Method)

		switch req.Method {
		case "initialize":
			resp := rpcResponse{
				JSONRPC: "2.0", ID: req.ID,
				Result: json.RawMessage(`{"capabilities":{},"serverInfo":{"name":"test"}}`),
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			resp := rpcResponse{
				JSONRPC: "2.0", ID: req.ID,
				Result: json.RawMessage(`{"tools":[{"name":"echo","description":"echoes input"}]}`),
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.Error(w, "unknown method: "+req.Method, 400)
		}
	}))
	defer srv.Close()

	client, err := NewHTTPClient("test", srv.URL, nil)
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	if client.Name() != "test" {
		t.Fatalf("expected name 'test', got %q", client.Name())
	}
	tools := client.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Fatalf("expected tool 'echo', got %q", tools[0].Name)
	}
	if len(calls) < 3 {
		t.Fatalf("expected >= 3 RPC calls, got %d: %v", len(calls), calls)
	}
	if calls[0] != "initialize" {
		t.Fatalf("first call should be 'initialize', got %q", calls[0])
	}
}

func TestHTTPClientCallTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"capabilities":{}}`)})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"tools":[{"name":"greet"}]}`)})
		case "tools/call":
			b, _ := json.Marshal(req.Params)
			var p struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			_ = json.Unmarshal(b, &p)
			text := "hello " + p.Arguments["name"].(string)
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"content":[{"type":"text","text":"` + text + `"}]}`)})
		}
	}))
	defer srv.Close()

	client, err := NewHTTPClient("test", srv.URL, nil)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	result, err := client.CallTool(context.TODO(), "greet", map[string]interface{}{"name": "world"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result != "hello world" {
		t.Fatalf("expected 'hello world', got %q", result)
	}
}

func TestHTTPClientCallToolError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"capabilities":{}}`)})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"tools":[{"name":"fail"}]}`)})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"content":[{"type":"text","text":"bad thing"}],"isError":true}`)})
		}
	}))
	defer srv.Close()

	client, err := NewHTTPClient("test", srv.URL, nil)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.CallTool(context.TODO(), "fail", nil)
	if err == nil {
		t.Fatal("expected error from isError:true response")
	}
}

func TestHTTPClientSSEResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Content-Type", "text/event-stream")
			resp, _ := json.Marshal(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"capabilities":{}}`)})
			_, _ = w.Write([]byte("event: message\ndata: " + string(resp) + "\n\n"))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"tools":[]}`)})
		}
	}))
	defer srv.Close()

	client, err := NewHTTPClient("sse-test", srv.URL, nil)
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	if len(client.Tools()) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(client.Tools()))
	}
}
