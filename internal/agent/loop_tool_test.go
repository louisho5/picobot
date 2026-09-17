package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
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
	ag := NewAgentLoop(b, p, p.GetDefaultModel(), 3, "", nil, nil)

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

func TestPruneOlderToolMessages(t *testing.T) {
	msgs := []providers.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "user query"},
		{Role: "assistant", Content: "web fetch", ToolCalls: []providers.ToolCall{{ID: "w1", Name: "web"}}},
		{Role: "tool", Content: strings.Repeat("A", 2000), ToolCallID: "w1"},
		{Role: "assistant", Content: "file write", ToolCalls: []providers.ToolCall{{ID: "f1", Name: "filesystem"}}},
		{Role: "tool", Content: "short response", ToolCallID: "f1"},
	}

	pruned := pruneOlderToolMessages(msgs)
	if len(pruned) != len(msgs) {
		t.Fatalf("expected same number of messages, got %d", len(pruned))
	}

	// The first tool message (index 3) is older and should be truncated.
	if !strings.Contains(pruned[3].Content, "truncated") {
		t.Errorf("expected first tool message to be truncated, got: %q", pruned[3].Content)
	}

	// The second tool message (index 5) is part of the latest turn and should be intact.
	if pruned[5].Content != "short response" {
		t.Errorf("expected latest tool message to be intact, got: %q", pruned[5].Content)
	}
}

func TestAgentPlanGuard(t *testing.T) {
	tmpDir := t.TempDir()
	hub := chat.NewHub(10)
	provider := &FakeProvider{}
	ag := NewAgentLoop(hub, provider, "fake", 3, tmpDir, nil, nil)

	// Case 1: PLAN.md does not exist. Check blocked tools.
	err := ag.checkPlanGuard("web", map[string]interface{}{"url": "http://foo"})
	if err == nil || !strings.Contains(err.Error(), "you must first create a 'PLAN.md' file") {
		t.Errorf("expected web tool to be blocked without PLAN.md, got error: %v", err)
	}

	err = ag.checkPlanGuard("filesystem", map[string]interface{}{"action": "write", "path": "code.go"})
	if err == nil || !strings.Contains(err.Error(), "you must first create a 'PLAN.md' file") {
		t.Errorf("expected file write to be blocked without PLAN.md, got error: %v", err)
	}

	// Case 2: Check allowed tools/actions when PLAN.md does not exist.
	err = ag.checkPlanGuard("filesystem", map[string]interface{}{"action": "read", "path": "code.go"})
	if err != nil {
		t.Errorf("expected filesystem read to be allowed, got error: %v", err)
	}

	err = ag.checkPlanGuard("filesystem", map[string]interface{}{"action": "write", "path": "PLAN.md"})
	if err != nil {
		t.Errorf("expected filesystem write to PLAN.md to be allowed, got error: %v", err)
	}

	err = ag.checkPlanGuard("message", map[string]interface{}{"content": "hello"})
	if err != nil {
		t.Errorf("expected message tool to be allowed, got error: %v", err)
	}

	// Case 3: Create PLAN.md and check that all tools are now allowed.
	if err := os.WriteFile(filepath.Join(tmpDir, "PLAN.md"), []byte("# Plan"), 0644); err != nil {
		t.Fatalf("failed to write dummy PLAN.md: %v", err)
	}

	err = ag.checkPlanGuard("web", map[string]interface{}{"url": "http://foo"})
	if err != nil {
		t.Errorf("expected web tool to be allowed once PLAN.md exists, got error: %v", err)
	}

	err = ag.checkPlanGuard("filesystem", map[string]interface{}{"action": "write", "path": "code.go"})
	if err != nil {
		t.Errorf("expected file write to be allowed once PLAN.md exists, got error: %v", err)
	}
}

