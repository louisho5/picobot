package providers

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/local/picobot/internal/config"
)

// NewProviderFromConfig creates a provider based on the resolved model name and config.
// model is the final model string after all overrides (flag > env > config default) have
// been applied by the caller. Selection rules:
//   - model starts with "openai-codex/" → Codex OAuth provider
//   - OpenAI API key or API base is set  → OpenAI-compatible provider
//   - else                               → stub (echo) provider
func NewProviderFromConfig(cfg config.Config, model string) LLMProvider {
	// Codex: detected by model name prefix, uses OAuth tokens from config
	if strings.HasPrefix(model, "openai-codex/") {
		cfgPath := resolveDefaultConfigPath()
		return NewOpenAICodexProvider(cfg.Providers.Codex, cfg.Agents.Defaults.RequestTimeoutS, cfgPath)
	}

	if cfg.Providers.OpenAI != nil && (cfg.Providers.OpenAI.APIKey != "" || cfg.Providers.OpenAI.APIBase != "") {
		return NewOpenAIProvider(
			cfg.Providers.OpenAI.APIKey,
			cfg.Providers.OpenAI.APIBase,
			cfg.Agents.Defaults.RequestTimeoutS,
			cfg.Agents.Defaults.MaxTokens,
		)
	}
	return NewStubProvider()
}

// resolveDefaultConfigPath returns the default path to config.json.
func resolveDefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".picobot", "config.json")
}
