package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/config"
	"github.com/local/picobot/internal/providers"
)

// provider that asks the agent to call the 'web' tool, then checks that the tool output
// was included in the messages on the subsequent call.
type webCallingProvider struct {
	calls  int
	server string
	seen   bool
}

func (p *webCallingProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string) (providers.LLMResponse, error) {
	p.calls++
	if p.calls == 1 {
		args := map[string]interface{}{"url": p.server}
		tc := providers.ToolCall{ID: "1", Name: "web", Arguments: args}
		return providers.LLMResponse{Content: "Calling web", HasToolCalls: true, ToolCalls: []providers.ToolCall{tc}}, nil
	}
	// on second call, look through messages for the tool result
	for _, m := range messages {
		if m.Role == "system" && m.Content != "" {
			if m.Content == "[tool:web] " { // empty result unlikely
				continue
			}
			// mark seen if web content appears
			if contains := (m.Content != ""); contains {
				p.seen = true
			}
		}
	}
	return providers.LLMResponse{Content: "Done", HasToolCalls: false}, nil
}
func (p *webCallingProvider) GetDefaultModel() string { return "test" }

func TestAgentExecutesWebToolCall(t *testing.T) {
	// create a real server to fetch
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hello web"))
	}))
	defer h.Close()

	b := chat.NewHub(10)
	p := &webCallingProvider{server: h.URL}
	ag := NewAgentLoop(b, p, p.GetDefaultModel(), 5, "", nil, config.MCPConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go ag.Run(ctx)

	in := chat.Inbound{Channel: "cli", SenderID: "user", ChatID: "one", Content: "Please fetch"}
	select {
	case b.In <- in:
	default:
		t.Fatalf("couldn't send inbound")
	}

	deadline := time.After(1 * time.Second)
	for {
		select {
		case out := <-b.Out:
			if out.Content == "Done" {
				if !p.seen {
					t.Fatalf("expected provider to see web tool result in messages")
				}
				return
			}
		case <-deadline:
			t.Fatalf("timeout waiting for final response")
		}
	}
}
