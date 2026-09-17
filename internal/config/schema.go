package config

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Config holds picobot configuration (minimal for v0).
type Config struct {
	Agents     AgentsConfig               `json:"agents"`
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
	Channels   ChannelsConfig             `json:"channels"`
	Providers  ProvidersConfig            `json:"providers"`
}

// MCPServerConfig describes a single MCP server connection.
// Use Command+Args for stdio transport, or URL+Headers for HTTP transport.
type MCPServerConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
}

type AgentDefaults struct {
	Workspace                   string  `json:"workspace"`
	Model                       string  `json:"model"`
	MaxTokens                   int     `json:"maxTokens"`
	Temperature                 float64 `json:"temperature"`
	MaxToolIterations           int     `json:"maxToolIterations"`
	HeartbeatIntervalS          int     `json:"heartbeatIntervalS"`
	RequestTimeoutS             int     `json:"requestTimeoutS"`
	EnableToolActivityIndicator *bool   `json:"enableToolActivityIndicator,omitempty"`
	MemoryRanker                string  `json:"memoryRanker,omitempty"`
}

type ChannelsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
	Discord  DiscordConfig  `json:"discord"`
	Slack    SlackConfig    `json:"slack"`
	WhatsApp WhatsAppConfig `json:"whatsapp"`
}

type DiscordConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
}

func (d *DiscordConfig) UnmarshalJSON(data []byte) error {
	type Alias DiscordConfig
	aux := &struct {
		AllowFrom []interface{} `json:"allowFrom"`
		*Alias
	}{
		Alias: (*Alias)(d),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	d.AllowFrom = nil
	for _, item := range aux.AllowFrom {
		switch v := item.(type) {
		case string:
			d.AllowFrom = append(d.AllowFrom, v)
		case float64:
			d.AllowFrom = append(d.AllowFrom, strconv.FormatFloat(v, 'f', -1, 64))
		default:
			return fmt.Errorf("invalid type in Discord allowFrom: %T", item)
		}
	}
	return nil
}

type TelegramConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
}

func (t *TelegramConfig) UnmarshalJSON(data []byte) error {
	type Alias TelegramConfig
	aux := &struct {
		AllowFrom []interface{} `json:"allowFrom"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	t.AllowFrom = nil
	for _, item := range aux.AllowFrom {
		switch v := item.(type) {
		case string:
			t.AllowFrom = append(t.AllowFrom, v)
		case float64:
			t.AllowFrom = append(t.AllowFrom, strconv.FormatFloat(v, 'f', -1, 64))
		default:
			return fmt.Errorf("invalid type in Telegram allowFrom: %T", item)
		}
	}
	return nil
}

type SlackConfig struct {
	Enabled       bool     `json:"enabled"`
	AppToken      string   `json:"appToken"`
	BotToken      string   `json:"botToken"`
	AllowUsers    []string `json:"allowUsers"`
	AllowChannels []string `json:"allowChannels"`
}

func (s *SlackConfig) UnmarshalJSON(data []byte) error {
	type Alias SlackConfig
	aux := &struct {
		AllowUsers    []interface{} `json:"allowUsers"`
		AllowChannels []interface{} `json:"allowChannels"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	s.AllowUsers = nil
	for _, item := range aux.AllowUsers {
		switch v := item.(type) {
		case string:
			s.AllowUsers = append(s.AllowUsers, v)
		case float64:
			s.AllowUsers = append(s.AllowUsers, strconv.FormatFloat(v, 'f', -1, 64))
		default:
			return fmt.Errorf("invalid type in Slack allowUsers: %T", item)
		}
	}
	s.AllowChannels = nil
	for _, item := range aux.AllowChannels {
		switch v := item.(type) {
		case string:
			s.AllowChannels = append(s.AllowChannels, v)
		case float64:
			s.AllowChannels = append(s.AllowChannels, strconv.FormatFloat(v, 'f', -1, 64))
		default:
			return fmt.Errorf("invalid type in Slack allowChannels: %T", item)
		}
	}
	return nil
}

type WhatsAppConfig struct {
	Enabled   bool     `json:"enabled"`
	DBPath    string   `json:"dbPath"`
	AllowFrom []string `json:"allowFrom"`
}

func (w *WhatsAppConfig) UnmarshalJSON(data []byte) error {
	type Alias WhatsAppConfig
	aux := &struct {
		AllowFrom []interface{} `json:"allowFrom"`
		*Alias
	}{
		Alias: (*Alias)(w),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	w.AllowFrom = nil
	for _, item := range aux.AllowFrom {
		switch v := item.(type) {
		case string:
			w.AllowFrom = append(w.AllowFrom, v)
		case float64:
			w.AllowFrom = append(w.AllowFrom, strconv.FormatFloat(v, 'f', -1, 64))
		default:
			return fmt.Errorf("invalid type in WhatsApp allowFrom: %T", item)
		}
	}
	return nil
}

type ProvidersConfig struct {
	OpenAI *ProviderConfig `json:"openai,omitempty"`
}

type ProviderConfig struct {
	APIKey  string `json:"apiKey"`
	APIBase string `json:"apiBase"`
}
