package config

// Config holds picobot configuration (minimal for v0).
type Config struct {
	Agents    AgentsConfig    `json:"agents"`
	Channels  ChannelsConfig  `json:"channels"`
	Providers ProvidersConfig `json:"providers"`
	MCP       MCPConfig       `json:"mcp"`
}

// MCPConfig holds MCP server configurations.
type MCPConfig struct {
	Servers map[string]MCPServerConfig `json:"servers"` // Example server provided by default
}

// MCPServerConfig defines a single MCP server connection.
// Supports both stdio transport (Command/Args/Env) and HTTP transport (URL).
type MCPServerConfig struct {
	Command string            `json:"command,omitempty"` // For stdio transport: executable
	Args    []string          `json:"args,omitempty"`    // For stdio transport: arguments
	Env     map[string]string `json:"env,omitempty"`     // For stdio transport: environment
	URL     string            `json:"url,omitempty"`     // For HTTP transport: endpoint URL
}

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
}

type AgentDefaults struct {
	Workspace          string  `json:"workspace"`
	Model              string  `json:"model"`
	MaxTokens          int     `json:"maxTokens"`
	Temperature        float64 `json:"temperature"`
	MaxToolIterations  int     `json:"maxToolIterations"`
	HeartbeatIntervalS int     `json:"heartbeatIntervalS"`
	RequestTimeoutS    int     `json:"requestTimeoutS"`
}

type ChannelsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
}

type TelegramConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
}

type ProvidersConfig struct {
	OpenAI *ProviderConfig `json:"openai,omitempty"`
}

type ProviderConfig struct {
	APIKey  string `json:"apiKey"`
	APIBase string `json:"apiBase"`
}
