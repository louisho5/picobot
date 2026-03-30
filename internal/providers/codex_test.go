package providers

import (
	"testing"

	"github.com/local/picobot/internal/config"
)

// TestCodexProviderPickedByFactory verifies that NewProviderFromConfig returns an
// OpenAICodexProvider when the configured model starts with "openai-codex/".
func TestCodexProviderPickedByFactory(t *testing.T) {
	cfg := config.Config{}
	cfg.Agents.Defaults.Model = "openai-codex/gpt-5.4-mini"
	cfg.Providers.Codex = &config.CodexProviderConfig{
		AccessToken:  "fake-access",
		RefreshToken: "fake-refresh",
		AccountID:    "acct-123",
	}

	p := NewProviderFromConfig(cfg, "openai-codex/gpt-5.4-mini")
	if _, ok := p.(*OpenAICodexProvider); !ok {
		t.Fatalf("expected *OpenAICodexProvider, got %T", p)
	}
}

// TestCodexDefaultModel verifies the default model name.
func TestCodexDefaultModel(t *testing.T) {
	p := NewOpenAICodexProvider(nil, 60, "")
	if got := p.GetDefaultModel(); got != codexDefaultModel {
		t.Errorf("expected default model %q, got %q", codexDefaultModel, got)
	}
}

// TestConvertMessagesToCodexInput verifies that messages are converted to the
// Codex Responses API input format correctly.
func TestConvertMessagesToCodexInput(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello!"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "What is 2+2?"},
	}

	instructions, items := convertMessagesToCodex(messages)

	if instructions != "You are a helpful assistant." {
		t.Errorf("expected instructions to be system message content, got %q", instructions)
	}

	// Should have: user, assistant (text), user = 3 items
	if len(items) != 3 {
		t.Fatalf("expected 3 input items, got %d", len(items))
	}

	// First item: user message
	first, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatal("first item is not a map")
	}
	if first["role"] != "user" {
		t.Errorf("first item role: expected %q, got %q", "user", first["role"])
	}

	// Second item: assistant message with type "message"
	second, ok := items[1].(map[string]interface{})
	if !ok {
		t.Fatal("second item is not a map")
	}
	if second["type"] != "message" {
		t.Errorf("second item type: expected %q, got %q", "message", second["type"])
	}
	if second["role"] != "assistant" {
		t.Errorf("second item role: expected %q, got %q", "assistant", second["role"])
	}
}

// TestConvertMessagesToCodexWithToolCall verifies that tool calls in assistant
// messages are emitted as function_call items.
func TestConvertMessagesToCodexWithToolCall(t *testing.T) {
	messages := []Message{
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{ID: "call_abc", Name: "web_search", Arguments: map[string]interface{}{"query": "golang"}},
			},
		},
		{Role: "tool", Content: `{"results": []}`, ToolCallID: "call_abc"},
	}

	_, items := convertMessagesToCodex(messages)

	// Expect: function_call item, function_call_output item = 2 items
	if len(items) != 2 {
		t.Fatalf("expected 2 input items, got %d", len(items))
	}

	fc, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatal("first item is not a map")
	}
	if fc["type"] != "function_call" {
		t.Errorf("first item type: expected %q, got %q", "function_call", fc["type"])
	}
	if fc["call_id"] != "call_abc" {
		t.Errorf("call_id: expected %q, got %q", "call_abc", fc["call_id"])
	}
	if fc["name"] != "web_search" {
		t.Errorf("name: expected %q, got %q", "web_search", fc["name"])
	}

	fo, ok := items[1].(map[string]interface{})
	if !ok {
		t.Fatal("second item is not a map")
	}
	if fo["type"] != "function_call_output" {
		t.Errorf("second item type: expected %q, got %q", "function_call_output", fo["type"])
	}
	if fo["call_id"] != "call_abc" {
		t.Errorf("call_id: expected %q, got %q", "call_abc", fo["call_id"])
	}
}

// TestConvertToolsToCodex verifies that ToolDefinitions are converted to the
// Codex flat tool format.
func TestConvertToolsToCodex(t *testing.T) {
	tools := []ToolDefinition{
		{
			Name:        "filesystem",
			Description: "Read or write files",
			Parameters:  map[string]interface{}{"type": "object"},
		},
	}

	out := convertToolsToCodex(tools)
	if len(out) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(out))
	}
	tool := out[0]
	if tool["type"] != "function" {
		t.Errorf("type: expected %q, got %q", "function", tool["type"])
	}
	if tool["name"] != "filesystem" {
		t.Errorf("name: expected %q, got %q", "filesystem", tool["name"])
	}
	if tool["description"] != "Read or write files" {
		t.Errorf("description mismatch: %q", tool["description"])
	}
}
