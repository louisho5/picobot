package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/config"
	"github.com/local/picobot/internal/providers"
)

// mcpCallingProvider requests discovered MCP tools and consumes their results.
type mcpCallingProvider struct{ calls int }

func (p *mcpCallingProvider) GetDefaultModel() string { return "mcp-test" }
func (p *mcpCallingProvider) Chat(_ context.Context, messages []providers.Message, defs []providers.ToolDefinition, _ string) (providers.LLMResponse, error) {
	available := map[string]bool{}
	for _, def := range defs {
		available[def.Name] = true
	}
	for _, name := range []string{"web_search", "mcp_parallel_web_search", "mcp_parallel_web_fetch"} {
		if !available[name] {
			return providers.LLMResponse{}, fmt.Errorf("missing tool %s", name)
		}
	}
	p.calls++
	switch p.calls {
	case 1:
		return providers.LLMResponse{HasToolCalls: true, ToolCalls: []providers.ToolCall{{ID: "search", Name: "mcp_parallel_web_search", Arguments: map[string]interface{}{
			"objective": "Find the Go HTTP client documentation", "search_queries": []string{"Go net/http client timeout"},
		}}}}, nil
	case 2:
		last := messages[len(messages)-1]
		if last.Role != "tool" || last.ToolCallID != "search" || !strings.Contains(last.Content, "https://pkg.go.dev/net/http") {
			return providers.LLMResponse{}, fmt.Errorf("search result did not reach model: %s", last.Content)
		}
		return providers.LLMResponse{HasToolCalls: true, ToolCalls: []providers.ToolCall{{ID: "fetch", Name: "mcp_parallel_web_fetch", Arguments: map[string]interface{}{
			"urls": []string{"https://pkg.go.dev/net/http"},
		}}}}, nil
	default:
		last := messages[len(messages)-1]
		if last.Role != "tool" || last.ToolCallID != "fetch" || !strings.Contains(last.Content, "Client.Timeout") {
			return providers.LLMResponse{}, fmt.Errorf("fetch result did not reach model: %s", last.Content)
		}
		return providers.LLMResponse{Content: last.Content}, nil
	}
}

func TestParallelMCPExampleAgentLoop(t *testing.T) {
	data, err := os.ReadFile("../../docs/examples/parallel-search.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	entry := cfg.MCPServers["parallel"]
	if len(cfg.MCPServers) != 1 || entry.URL != "https://search.parallel.ai/mcp" || entry.Command != "" || len(entry.Headers) != 1 || entry.Headers["User-Agent"] != "picobot (https://github.com/louisho5/picobot)" || cfg.Providers.OpenAI != nil {
		t.Fatalf("example must be an anonymous, opt-in HTTP configuration: %+v", cfg)
	}
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/mcp" || r.Header.Get("User-Agent") != entry.Headers["User-Agent"] || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected MCP request: %s %s headers=%v", r.Method, r.URL, r.Header)
		}
		var req struct {
			ID     interface{} `json:"id"`
			Method string      `json:"method"`
			Params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		methods = append(methods, req.Method)
		var result interface{}
		switch req.Method {
		case "initialize":
			result = map[string]interface{}{"protocolVersion": "2025-03-26", "capabilities": map[string]interface{}{}, "serverInfo": map[string]interface{}{"name": "fixture", "version": "1"}}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			result = map[string]interface{}{"tools": []interface{}{
				map[string]interface{}{"name": "web_search", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"objective": map[string]string{"type": "string"}, "search_queries": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}}}, "required": []string{"objective", "search_queries"}}},
				map[string]interface{}{"name": "web_fetch", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"urls": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}}}, "required": []string{"urls"}}},
			}}
		case "tools/call":
			text := ""
			switch req.Params.Name {
			case "web_search":
				if req.Params.Arguments["objective"] != "Find the Go HTTP client documentation" || fmt.Sprint(req.Params.Arguments["search_queries"]) != "[Go net/http client timeout]" {
					t.Errorf("incorrect search arguments: %v", req.Params.Arguments)
				}
				text = "Go HTTP client documentation: https://pkg.go.dev/net/http"
			case "web_fetch":
				if fmt.Sprint(req.Params.Arguments["urls"]) != "[https://pkg.go.dev/net/http]" {
					t.Errorf("incorrect fetch arguments: %v", req.Params.Arguments)
				}
				text = "Client.Timeout specifies a time limit for requests."
			default:
				t.Errorf("unexpected tool %s", req.Params.Name)
			}
			result = map[string]interface{}{"content": []interface{}{map[string]string{"type": "text", "text": text}}}
		default:
			t.Errorf("unexpected method %s", req.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": result}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	entry.URL = server.URL + "/mcp"
	cfg.MCPServers["parallel"] = entry
	p := &mcpCallingProvider{}
	ag := NewAgentLoop(chat.NewHub(10), p, p.GetDefaultModel(), 3, t.TempDir(), nil, cfg.MCPServers)
	defer ag.Close()
	response, err := ag.ProcessDirect("Search and read the Go HTTP documentation", 5*time.Second)
	if err != nil || !strings.Contains(response, "Client.Timeout") || p.calls != 3 {
		t.Fatalf("agent dispatch failed: response=%q calls=%d err=%v", response, p.calls, err)
	}
	if got := strings.Join(methods, ","); got != "initialize,notifications/initialized,tools/list,tools/call,tools/call" {
		t.Fatalf("unexpected MCP request sequence: %s", got)
	}
}
