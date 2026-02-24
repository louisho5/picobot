package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/providers"
)

type scopedMemoryProvider struct {
	secret   string
	call     int
	seenLeak bool
}

func (p *scopedMemoryProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string) (providers.LLMResponse, error) {
	p.call++
	switch p.call {
	case 1:
		tc := providers.ToolCall{
			ID:   "1",
			Name: "write_memory",
			Arguments: map[string]interface{}{
				"target":  "today",
				"content": p.secret,
				"append":  true,
			},
		}
		return providers.LLMResponse{HasToolCalls: true, ToolCalls: []providers.ToolCall{tc}}, nil
	case 2:
		return providers.LLMResponse{Content: "saved"}, nil
	default:
		for _, m := range messages {
			if strings.Contains(m.Content, p.secret) {
				p.seenLeak = true
				return providers.LLMResponse{Content: "leak"}, nil
			}
		}
		return providers.LLMResponse{Content: "ok"}, nil
	}
}

func (p *scopedMemoryProvider) GetDefaultModel() string { return "test" }

func TestAgentMemoryIsScopedByChannelAndChat(t *testing.T) {
	hub := chat.NewHub(10)
	prov := &scopedMemoryProvider{secret: "top-secret-123"}
	workspace := t.TempDir()
	ag := NewAgentLoop(hub, prov, prov.GetDefaultModel(), 4, workspace, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go ag.Run(ctx)

	// First chat writes a secret via write_memory tool.
	hub.In <- chat.Inbound{Channel: "telegram", SenderID: "u1", ChatID: "chat-A", Content: "store secret please"}
	select {
	case out := <-hub.Out:
		if out.Content != "saved" {
			t.Fatalf("expected first response 'saved', got %q", out.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for first response")
	}

	// Second chat should not see first chat's memory.
	hub.In <- chat.Inbound{Channel: "telegram", SenderID: "u2", ChatID: "chat-B", Content: "what do you know?"}
	select {
	case out := <-hub.Out:
		if out.Content != "ok" {
			t.Fatalf("expected second response 'ok', got %q", out.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for second response")
	}

	if prov.seenLeak {
		t.Fatalf("memory leak detected: second chat observed first chat secret")
	}
}
