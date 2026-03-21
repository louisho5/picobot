package providers

import (
	"testing"

	"github.com/local/picobot/internal/config"
)

func TestNewProviderFromConfig_PicksOpenAI(t *testing.T) {
	cfg := config.Config{}
	cfg.Providers.OpenAI = &config.ProviderConfig{APIKey: "test"}
	p := NewProviderFromConfig(cfg)
	_, ok := p.(*OpenAIProvider)
	if !ok {
		t.Fatalf("expected OpenAIProvider, got %T", p)
	}
}

func TestNewProviderFromConfig_FallbacksToStub(t *testing.T) {
	cfg := config.Config{}
	p := NewProviderFromConfig(cfg)
	_, ok := p.(*StubProvider)
	if !ok {
		t.Fatalf("expected StubProvider, got %T", p)
	}
}
