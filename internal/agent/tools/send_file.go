package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/local/picobot/internal/chat"
)

// SendFileTool sends a local file from the workspace to the current channel.
type SendFileTool struct {
	hub       *chat.Hub
	workspace string
	mu        sync.RWMutex
	channel   string
	chatID    string
}

func NewSendFileTool(b *chat.Hub, workspace string) *SendFileTool {
	return &SendFileTool{
		hub:       b,
		workspace: workspace,
	}
}

func (s *SendFileTool) Name() string        { return "send_file" }
func (s *SendFileTool) Description() string { return "Send a file from the workspace to the current chat channel (e.g. Telegram)" }

func (s *SendFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The relative path to the file inside the workspace (e.g. project-abc/prime.py)",
			},
			"caption": map[string]interface{}{
				"type":        "string",
				"description": "Optional text caption/description to send along with the file",
			},
		},
		"required": []string{"path"},
	}
}

func (s *SendFileTool) SetContext(channel, chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channel = channel
	s.chatID = chatID
}

func (s *SendFileTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	pathRaw, ok := args["path"]
	if !ok {
		return "", fmt.Errorf("send_file: 'path' argument required")
	}
	pathStr, ok := pathRaw.(string)
	if !ok || pathStr == "" {
		return "", fmt.Errorf("send_file: 'path' must be a non-empty string")
	}

	caption := ""
	if capRaw, ok := args["caption"]; ok {
		if capStr, ok := capRaw.(string); ok {
			caption = capStr
		}
	}

	// Resolve absolute path in workspace
	absPath := filepath.Join(s.workspace, pathStr)
	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return "", fmt.Errorf("send_file: file does not exist: %s", pathStr)
	}

	channel, chatID := chat.FromContext(ctx)
	if channel == "" {
		s.mu.RLock()
		channel = s.channel
		s.mu.RUnlock()
	}
	if chatID == "" {
		s.mu.RLock()
		chatID = s.chatID
		s.mu.RUnlock()
	}

	// Publish outbound message to hub
	out := chat.Outbound{
		Channel: channel,
		ChatID:  chatID,
		Content: caption,
		Media:   []string{absPath},
	}

	select {
	case s.hub.Out <- out:
		return "file sent", nil
	default:
		return "", fmt.Errorf("outbound channel full")
	}
}
