package agent

import (
	"context"
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/config"
	"github.com/local/picobot/internal/providers"
)

// Fake provider that returns a tool call on first chat, then returns a final message on second chat.
type FakeProvider struct {
	count int
}

func (f *FakeProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string) (providers.LLMResponse, error) {
	f.count++
	if f.count == 1 {
		// request message tool
		return providers.LLMResponse{
			Content:      "Invoking message tool",
			HasToolCalls: true,
			ToolCalls:    []providers.ToolCall{{ID: "1", Name: "message", Arguments: map[string]interface{}{"content": "hello from tool"}}},
		}, nil
	}
	return providers.LLMResponse{Content: "All done!"}, nil
}
func (f *FakeProvider) GetDefaultModel() string { return "fake" }

func TestAgentExecutesToolCall(t *testing.T) {
	b := chat.NewHub(10)
	p := &FakeProvider{}
	ag := NewAgentLoop(b, p, p.GetDefaultModel(), 3, "", nil, config.MCPConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go ag.Run(ctx)

	// send inbound
	in := chat.Inbound{Channel: "cli", SenderID: "user", ChatID: "one", Content: "trigger"}
	select {
	case b.In <- in:
	default:
		t.Fatalf("couldn't send inbound")
	}

	// expect outbound
	deadline := time.After(1 * time.Second)
	for {
		select {
		case out := <-b.Out:
			if out.Content == "All done!" {
				return
			}
			// otherwise continue waiting until timeout
		case <-deadline:
			t.Fatalf("timeout waiting for final outbound message")
		}
	}
}
