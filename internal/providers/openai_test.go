package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIFunctionCallParsing(t *testing.T) {
	// Build a fake server that returns a tool_calls style response
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{
		  "choices": [
		    {
		      "message": {
		        "role": "assistant",
		        "content": "",
		        "tool_calls": [
		          {
		            "id": "call_001",
		            "type": "function",
		            "function": {
		              "name": "message",
		              "arguments": "{\"content\": \"Hello from function\"}"
		            }
		          }
		        ]
		      }
		    }
		  ]
		}`))
	}))
	defer h.Close()

	p := NewOpenAIProvider("test-key", h.URL, 60)
	p.Client = &http.Client{Timeout: 5 * time.Second}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msgs := []Message{{Role: "user", Content: "trigger"}}
	resp, err := p.Chat(ctx, msgs, nil, "model-x")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !resp.HasToolCalls || len(resp.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got: has=%v len=%d", resp.HasToolCalls, len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "message" {
		t.Fatalf("expected tool name 'message', got '%s'", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["content"] != "Hello from function" {
		t.Fatalf("unexpected argument content: %v", resp.ToolCalls[0].Arguments)
	}
}
