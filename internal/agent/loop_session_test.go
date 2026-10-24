package agent

import (
	"context"
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/providers"
)

// recordingProvider captures the messages sent on each Chat call.
type recordingProvider struct {
	last []providers.Message
}

func (p *recordingProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string) (providers.LLMResponse, error) {
	p.last = messages
	return providers.LLMResponse{Content: "reply"}, nil
}
func (p *recordingProvider) GetDefaultModel() string { return "test" }

func hasMessage(msgs []providers.Message, role, content string) bool {
	for _, m := range msgs {
		if m.Role == role && m.Content == content {
			return true
		}
	}
	return false
}

func TestHubSessionHistorySurvivesRestart(t *testing.T) {
	ws := t.TempDir()
	in := chat.Inbound{Channel: "telegram", ChatID: "42", Content: "my name is Ada"}

	ag := NewAgentLoop(chat.NewHub(10), &recordingProvider{}, "test", 5, ws, nil, nil)
	ag.handleInbound(context.Background(), in)

	// New agent loop on the same workspace simulates a restart.
	prov := &recordingProvider{}
	ag2 := NewAgentLoop(chat.NewHub(10), prov, "test", 5, ws, nil, nil)
	in.Content = "what is my name?"
	ag2.handleInbound(context.Background(), in)

	if !hasMessage(prov.last, "user", "my name is Ada") || !hasMessage(prov.last, "assistant", "reply") {
		t.Fatalf("expected prior conversation in context after restart, got %+v", prov.last)
	}
}

func TestSystemChannelHistoryNotRestored(t *testing.T) {
	ws := t.TempDir()
	in := chat.Inbound{Channel: "heartbeat", ChatID: "hb", Content: "tick"}

	ag := NewAgentLoop(chat.NewHub(10), &recordingProvider{}, "test", 5, ws, nil, nil)
	ag.handleInbound(context.Background(), in)

	prov := &recordingProvider{}
	ag2 := NewAgentLoop(chat.NewHub(10), prov, "test", 5, ws, nil, nil)
	ag2.handleInbound(context.Background(), in)

	// system prompt + current message only
	if len(prov.last) != 2 {
		t.Fatalf("expected no history for system channel, got %d messages", len(prov.last))
	}
}

func TestProcessDirectHistorySurvivesRestart(t *testing.T) {
	ws := t.TempDir()

	ag := NewAgentLoop(chat.NewHub(10), &recordingProvider{}, "test", 5, ws, nil, nil)
	if _, err := ag.ProcessDirect("first question", time.Second); err != nil {
		t.Fatal(err)
	}

	prov := &recordingProvider{}
	ag2 := NewAgentLoop(chat.NewHub(10), prov, "test", 5, ws, nil, nil)
	if _, err := ag2.ProcessDirect("second question", time.Second); err != nil {
		t.Fatal(err)
	}

	if !hasMessage(prov.last, "user", "first question") {
		t.Fatalf("expected CLI history after restart, got %+v", prov.last)
	}
}

func TestHistoryContextCharsLimitsLLMHistory(t *testing.T) {
	ws := t.TempDir()
	prov := &recordingProvider{}
	ag := NewAgentLoop(chat.NewHub(10), prov, "test", 5, ws, nil, nil)
	// Budget fits only the most recent user+assistant pair ("third" + "reply").
	ag.SetHistoryLimits(30, len("third")+len("reply"))

	for _, c := range []string{"first", "second", "third", "now"} {
		ag.handleInbound(context.Background(), chat.Inbound{Channel: "telegram", ChatID: "1", Content: c})
	}

	// system + third + reply + now
	if len(prov.last) != 4 || !hasMessage(prov.last, "user", "third") || hasMessage(prov.last, "user", "second") {
		t.Fatalf("expected only the newest history within budget, got %+v", prov.last)
	}
}
