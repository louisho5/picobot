package providers

import (
	"context"
	"encoding/json"
	"io"
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

	p := NewOpenAIProvider("test-key", h.URL, 60, 0)
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

// simpleOKResponse returns a minimal chat completion JSON with a text reply.
func simpleOKResponse() string {
	return `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`
}

// TestOpenAI_AssistantToolCallContentNull verifies that assistant messages with
// tool_calls serialize content as JSON null, not an empty string.
func TestOpenAI_AssistantToolCallContentNull(t *testing.T) {
	var captured chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Unmarshal into a raw structure to check null vs ""
		var raw struct {
			Messages []json.RawMessage `json:"messages"`
		}
		json.Unmarshal(body, &raw)

		// Also decode into typed struct for later assertions
		json.Unmarshal(body, &captured)

		// Find the assistant message with tool_calls and check raw JSON
		for _, m := range raw.Messages {
			var peek struct {
				Role      string            `json:"role"`
				Content   json.RawMessage   `json:"content"`
				ToolCalls []json.RawMessage `json:"tool_calls"`
			}
			json.Unmarshal(m, &peek)
			if peek.Role == "assistant" && len(peek.ToolCalls) > 0 {
				if string(peek.Content) != "null" {
					t.Errorf("expected assistant tool_call content to be null, got %s", string(peek.Content))
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(simpleOKResponse()))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL, 60, 0)
	p.Client = &http.Client{Timeout: 5 * time.Second}

	msgs := []Message{
		{Role: "user", Content: "hello"},
		{
			Role:    "assistant",
			Content: "", // empty content with tool calls → must become null
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "get_weather", Arguments: map[string]interface{}{"city": "Paris"}},
			},
		},
		{Role: "tool", Content: `{"temp":22}`, ToolCallID: "call_1"},
	}

	ctx := context.Background()
	_, err := p.Chat(ctx, msgs, nil, "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Double-check the typed struct: assistant message should have nil Content
	if len(captured.Messages) < 2 {
		t.Fatal("expected at least 2 messages in request")
	}
	assistantMsg := captured.Messages[1]
	if assistantMsg.Content != nil {
		t.Errorf("expected nil Content pointer for assistant tool_call message, got %q", *assistantMsg.Content)
	}
}

// TestOpenAI_ToolMessageContentPresent verifies that tool-role messages have
// their content as a string, not null.
func TestOpenAI_ToolMessageContentPresent(t *testing.T) {
	var captured chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(simpleOKResponse()))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL, 60, 0)
	p.Client = &http.Client{Timeout: 5 * time.Second}

	msgs := []Message{
		{Role: "user", Content: "hello"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{ID: "call_1", Name: "lookup", Arguments: map[string]interface{}{"q": "test"}},
			},
		},
		{Role: "tool", Content: `{"result":"found"}`, ToolCallID: "call_1"},
	}

	ctx := context.Background()
	_, err := p.Chat(ctx, msgs, nil, "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tool message is at index 2
	toolMsg := captured.Messages[2]
	if toolMsg.Content == nil {
		t.Fatal("expected tool message content to be non-nil")
	}
	if *toolMsg.Content != `{"result":"found"}` {
		t.Errorf("expected tool content %q, got %q", `{"result":"found"}`, *toolMsg.Content)
	}
}

// TestOpenAI_KeylessLocalServer verifies that an empty API key is allowed
// and no Authorization header is sent.
func TestOpenAI_KeylessLocalServer(t *testing.T) {
	var authHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(simpleOKResponse()))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("", srv.URL, 60, 0) // no API key
	p.Client = &http.Client{Timeout: 5 * time.Second}

	ctx := context.Background()
	resp, err := p.Chat(ctx, []Message{{Role: "user", Content: "hi"}}, nil, "local-model")
	if err != nil {
		t.Fatalf("expected no error with empty API key, got: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected content 'ok', got %q", resp.Content)
	}
	if authHeader != "" {
		t.Errorf("expected no Authorization header, got %q", authHeader)
	}
}

// TestOpenAI_SpecialCharsInModelName verifies model names with special
// characters (like qwen3.5-35b-a3b@8bit) are sent unchanged.
func TestOpenAI_SpecialCharsInModelName(t *testing.T) {
	var captured chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(simpleOKResponse()))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("key", srv.URL, 60, 0)
	p.Client = &http.Client{Timeout: 5 * time.Second}

	modelName := "qwen3.5-35b-a3b@8bit"
	ctx := context.Background()
	_, err := p.Chat(ctx, []Message{{Role: "user", Content: "hi"}}, nil, modelName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Model != modelName {
		t.Errorf("expected model %q, got %q", modelName, captured.Model)
	}
}

// TestOpenAI_MultiTurnToolCallConversation simulates a full multi-turn flow:
// user → assistant(tool_call) → tool(result) → assistant(final).
// It validates the message array structure on the second API call.
func TestOpenAI_MultiTurnToolCallConversation(t *testing.T) {
	var lastCaptured chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &lastCaptured)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"The weather is 22C"}}]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL, 60, 0)
	p.Client = &http.Client{Timeout: 5 * time.Second}

	// Simulate the second call in a tool-call loop: all 4 messages are sent
	msgs := []Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "What is the weather in Paris?"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []ToolCall{
				{ID: "call_abc", Name: "get_weather", Arguments: map[string]interface{}{"city": "Paris"}},
			},
		},
		{Role: "tool", Content: `{"temperature":22,"unit":"C"}`, ToolCallID: "call_abc"},
	}

	ctx := context.Background()
	resp, err := p.Chat(ctx, msgs, nil, "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "The weather is 22C" {
		t.Errorf("unexpected final content: %q", resp.Content)
	}

	// Validate message structure
	if len(lastCaptured.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(lastCaptured.Messages))
	}

	// system message
	if lastCaptured.Messages[0].Role != "system" || lastCaptured.Messages[0].Content == nil {
		t.Error("system message malformed")
	}

	// user message
	if lastCaptured.Messages[1].Role != "user" || lastCaptured.Messages[1].Content == nil {
		t.Error("user message malformed")
	}

	// assistant message with tool_calls: content must be nil
	am := lastCaptured.Messages[2]
	if am.Role != "assistant" {
		t.Errorf("expected assistant role, got %q", am.Role)
	}
	if am.Content != nil {
		t.Errorf("expected nil content on assistant tool_call message, got %q", *am.Content)
	}
	if len(am.ToolCalls) != 1 || am.ToolCalls[0].ID != "call_abc" {
		t.Errorf("assistant tool_calls malformed: %+v", am.ToolCalls)
	}

	// tool message: content must be present
	tm := lastCaptured.Messages[3]
	if tm.Role != "tool" {
		t.Errorf("expected tool role, got %q", tm.Role)
	}
	if tm.Content == nil || *tm.Content != `{"temperature":22,"unit":"C"}` {
		t.Errorf("tool message content wrong: %v", tm.Content)
	}
	if tm.ToolCallID != "call_abc" {
		t.Errorf("expected tool_call_id 'call_abc', got %q", tm.ToolCallID)
	}
}
