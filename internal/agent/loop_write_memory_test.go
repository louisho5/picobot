package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/local/picobot/internal/agent/memory"
	"github.com/local/picobot/internal/agent/tools"
	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/config"
	"github.com/local/picobot/internal/providers"
)

// provider that returns a tool call first, then a final assistant message on second call
type toolCallingProvider struct {
	calls int
}

func (p *toolCallingProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string) (providers.LLMResponse, error) {
	p.calls++
	if p.calls == 1 {
		// instruct a write_memory call
		args := map[string]interface{}{"target": "today", "content": "appointment tomorrow", "append": true}
		tc := providers.ToolCall{ID: "1", Name: "write_memory", Arguments: args}
		return providers.LLMResponse{Content: "Calling tool", HasToolCalls: true, ToolCalls: []providers.ToolCall{tc}}, nil
	}
	return providers.LLMResponse{Content: "Saved, thanks.", HasToolCalls: false}, nil
}
func (p *toolCallingProvider) GetDefaultModel() string { return "fake-model" }

func TestAgentExecutesWriteMemoryToolCall(t *testing.T) {
	b := chat.NewHub(10)
	p := &toolCallingProvider{}
	ag := NewAgentLoop(b, p, p.GetDefaultModel(), 5, "", nil, config.MCPConfig{})

	// replace memory with temp workspace and re-register write_memory tool
	tmp := t.TempDir()
	m := memory.NewMemoryStoreWithWorkspace(tmp, 100)
	ag.memory = m
	ag.tools.Register(tools.NewWriteMemoryTool(m))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go ag.Run(ctx)

	in := chat.Inbound{Channel: "cli", SenderID: "user", ChatID: "one", Content: "Please remember my appointment"}
	select {
	case b.In <- in:
	default:
		t.Fatalf("couldn't send inbound")
	}

	deadline := time.After(1 * time.Second)
	for {
		select {
		case out := <-b.Out:
			if out.Content == "Saved, thanks." {
				// verify today's file contains the note
				memCtx, _ := m.ReadToday()
				if memCtx == "" || !strings.Contains(memCtx, "appointment tomorrow") {
					t.Fatalf("expected today's memory to contain 'appointment tomorrow', got %q", memCtx)
				}
				return
			}
		case <-deadline:
			t.Fatalf("timeout waiting for final response")
		}
	}
}
