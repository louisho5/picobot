# Channel Signaling Architecture Plan

## Problem
The agent loop currently has direct knowledge of Telegram-specific features (typing indicator), breaking the clean architecture where agent loop should be channel-agnostic.

## Current Violation
```go
// internal/agent/loop.go
func (a *AgentLoop) startTypingIndicator(channel, chatID string) func() {
    if channel != "telegram" || a.telegramToken == "" {
        return func() {}
    }
    // Directly imports channels package and calls Telegram API
    channels.SendTypingIndicator(a.telegramToken, chatID)
}
```

## Solution: Generic Signal System

### 1. Hub Changes (already done in chat/chat.go)
```go
type Signal struct {
    Type     string                 // "processing_started", "processing_done", "error", "progress"
    Channel  string                 // "telegram", "email", "cli", etc.
    ChatID   string                 // Specific conversation ID
    Metadata map[string]interface{} // Extensible payload
}

type Hub struct {
    In      chan Inbound
    Out     chan Outbound
    Signal  chan Signal  // NEW: Broadcast channel for signals
}
```

### 2. Agent Loop Changes
**Remove:**
- `telegramToken` field from AgentLoop
- `startTypingIndicator()` method
- Import of `channels` package

**Add:**
- Send signals at appropriate times

### 3. Telegram Channel Changes
Add signal handler goroutine that listens for "processing_started" / "processing_done"

## Migration Steps

1. Remove `telegramToken` from AgentLoop struct and NewAgentLoop params
2. Remove `startTypingIndicator()` method
3. Remove `channels` import from loop.go
4. Add signal sending in agent loop
5. Add signal handler in telegram.go
6. Update main.go to not pass telegram token to agent loop
7. Update tests

## Files to Modify

- `internal/agent/loop.go` - Remove Telegram-specific code, add signals
- `internal/channels/telegram.go` - Add signal handler
- `cmd/picobot/main.go` - Don't pass telegram token to agent loop
- `internal/agent/*_test.go` - Remove telegram token parameter
