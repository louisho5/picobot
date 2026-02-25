package config

// Config holds picobot configuration (minimal for v0).
type Config struct {
	Agents    AgentsConfig    `json:"agents"`
	Channels  ChannelsConfig  `json:"channels"`
	Providers ProvidersConfig `json:"providers"`
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
	Discord  DiscordConfig  `json:"discord"`
	WhatsApp WhatsAppConfig `json:"whatsapp"`
}

type DiscordConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
}

type TelegramConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
}

type WhatsAppConfig struct {
	Enabled   bool     `json:"enabled"`
	DBPath    string   `json:"dbPath"`
	AllowFrom []string `json:"allowFrom"`
}

type ProvidersConfig struct {
	OpenAI *ProviderConfig `json:"openai,omitempty"`
}

type ProviderConfig struct {
	APIKey  string `json:"apiKey"`
	APIBase string `json:"apiBase"`
}
