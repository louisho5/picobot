package agent

import (
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/config"
	"github.com/local/picobot/internal/providers"
)

func TestProcessDirectWithStub(t *testing.T) {
	b := chat.NewHub(10)
	p := providers.NewStubProvider()

	ag := NewAgentLoop(b, p, p.GetDefaultModel(), 5, "", nil, config.MCPConfig{})

	resp, err := ag.ProcessDirect("hello", 1*time.Second)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp == "" {
		t.Fatalf("expected response, got empty string")
	}
}
