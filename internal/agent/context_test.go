package agent

import (
	"strings"
	"testing"

	"github.com/local/picobot/internal/agent/memory"
)

func TestBuildMessagesIncludesMemories(t *testing.T) {
	cb := NewContextBuilder(".", memory.NewSimpleRanker(), 5)
	history := []string{"user: hi"}
	mems := []memory.MemoryItem{{Kind: "short", Text: "remember this"}, {Kind: "long", Text: "big fact"}}
	memCtx := "Long-term memory: important fact"
	msgs := cb.BuildMessages(history, "hello", "telegram", "123", memCtx, mems)

	// Expect at least 1 system message + 1 user history + 1 current user message
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Fatalf("expected first message to be system prompt, got %s", msgs[0].Role)
	}
	// find a system message containing the memory context
	foundMemCtx := false
	foundSummary := false
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "Long-term memory: important fact") {
			foundMemCtx = true
		}
		if m.Role == "system" && strings.Contains(m.Content, "remember this") && strings.Contains(m.Content, "big fact") {
			foundSummary = true
		}
	}
	if !foundMemCtx {
		t.Fatalf("expected memory context system message to be present in messages: %v", msgs)
	}
	if !foundSummary {
		t.Fatalf("expected memory summary to be present in messages: %v", msgs)
	}
}
