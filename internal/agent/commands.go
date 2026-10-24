package agent

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/local/picobot/internal/chat"
)

// maxListedConversations caps /chats output so it stays well within
// Telegram's 4096-character message limit.
const maxListedConversations = 20

// supportsCommands reports whether conversation commands (/new, /chats, ...)
// are enabled for a channel. Other channels pass such text to the LLM as-is.
func supportsCommands(channel string) bool {
	return channel == "telegram"
}

// handleCommand handles conversation-management commands without calling the
// LLM. It reports whether msg was a command; unknown commands (e.g. Telegram's
// /start) fall through to the LLM.
func (a *AgentLoop) handleCommand(msg chat.Inbound) (string, bool) {
	if !supportsCommands(msg.Channel) {
		return "", false
	}
	fields := strings.Fields(msg.Content)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return "", false
	}
	// In group chats Telegram appends the bot name: /new@picobot
	name, _, _ := strings.Cut(strings.ToLower(fields[0][1:]), "@")
	args := fields[1:]
	base := msg.Channel + ":" + msg.ChatID

	switch name {
	case "new":
		s, err := a.sessions.NewConversation(base, strings.Join(args, " "))
		if err != nil {
			return "Sorry, I couldn't start a new conversation: " + err.Error(), true
		}
		return fmt.Sprintf("Started conversation #%d. Previous conversations are kept; use /chats to list them.", s.Number), true

	case "chats":
		convs := a.sessions.Conversations(base)
		var sb strings.Builder
		sb.WriteString("Conversations (most recent first):\n")
		for i, s := range convs {
			if i == maxListedConversations {
				fmt.Fprintf(&sb, "…and %d older (switch to them by number)\n", len(convs)-i)
				break
			}
			marker := ""
			if a.sessions.IsActive(base, s.Key) {
				marker = " (current)"
			}
			fmt.Fprintf(&sb, "#%d %s%s · %d messages · %s\n", s.Number, s.DisplayTitle(), marker, len(s.Messages), s.LastActivity().Format("Jan 2 15:04"))
		}
		sb.WriteString("\nUse /switch <number> to change, /new to start another.")
		return sb.String(), true

	case "switch":
		usage := "Usage: /switch <number>, using a conversation number from /chats."
		if len(args) != 1 {
			return usage, true
		}
		n, err := strconv.Atoi(strings.TrimPrefix(args[0], "#"))
		if err != nil || n < 1 {
			return usage, true
		}
		s, ok := a.sessions.Conversation(base, n)
		if !ok {
			return fmt.Sprintf("There is no conversation #%d. Use /chats to see your conversations.", n), true
		}
		if err := a.sessions.SetActive(base, s.Key); err != nil {
			return "Sorry, I couldn't switch conversations: " + err.Error(), true
		}
		return fmt.Sprintf("Switched to #%d: %s (%d messages).", s.Number, s.DisplayTitle(), len(s.Messages)), true

	case "help":
		var sb strings.Builder
		sb.WriteString("Commands:\n")
		for _, c := range chat.Commands {
			fmt.Fprintf(&sb, "/%s - %s\n", c.Name, c.Description)
		}
		return strings.TrimRight(sb.String(), "\n"), true
	}
	return "", false
}
