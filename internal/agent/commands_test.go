package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/local/picobot/internal/chat"
)

// send runs one inbound message through the agent and returns the final reply.
func send(t *testing.T, ag *AgentLoop, hub *chat.Hub, channel, content string) string {
	t.Helper()
	ag.handleInbound(context.Background(), chat.Inbound{Channel: channel, ChatID: "1", Content: content})
	select {
	case out := <-hub.Out:
		return out.Content
	default:
		t.Fatalf("no reply to %q", content)
		return ""
	}
}

func TestConversationCommands(t *testing.T) {
	ws := t.TempDir()
	hub := chat.NewHub(10)
	prov := &recordingProvider{}
	ag := NewAgentLoop(hub, prov, "test", 5, ws, nil, nil)
	ag.SetToolActivityIndicator(false)

	send(t, ag, hub, "telegram", "plan my trip to Rome")

	if r := send(t, ag, hub, "telegram", "/new"); !strings.Contains(r, "Started conversation #2") {
		t.Fatalf("unexpected /new reply: %q", r)
	}
	prov.last = nil
	send(t, ag, hub, "telegram", "write standup notes")
	if hasMessage(prov.last, "user", "plan my trip to Rome") {
		t.Fatalf("new conversation should not include previous history: %+v", prov.last)
	}

	list := send(t, ag, hub, "telegram", "/chats")
	if !strings.Contains(list, "#2 write standup notes (current)") || !strings.Contains(list, "#1 plan my trip to Rome ·") {
		t.Fatalf("unexpected /chats output:\n%s", list)
	}

	if r := send(t, ag, hub, "telegram", "/switch 1"); !strings.Contains(r, "Switched to #1: plan my trip to Rome") {
		t.Fatalf("unexpected /switch reply: %q", r)
	}
	send(t, ag, hub, "telegram", "what about hotels?")
	if !hasMessage(prov.last, "user", "plan my trip to Rome") || hasMessage(prov.last, "user", "write standup notes") {
		t.Fatalf("expected switched-to conversation history only: %+v", prov.last)
	}

	// Commands are not recorded as conversation turns, and the switch survives a restart.
	ag2 := NewAgentLoop(chat.NewHub(10), &recordingProvider{}, "test", 5, ws, nil, nil)
	active := ag2.sessions.Active("telegram:1")
	for _, m := range active.Messages {
		if strings.HasPrefix(m.Content, "/") {
			t.Fatalf("command stored in history: %+v", m)
		}
	}
	if active.Messages[0].Content != "plan my trip to Rome" || len(active.Messages) != 4 {
		t.Fatalf("unexpected active conversation after restart: %+v", active.Messages)
	}
}

func TestSwitchValidatesArgument(t *testing.T) {
	hub := chat.NewHub(10)
	ag := NewAgentLoop(hub, &recordingProvider{}, "test", 5, t.TempDir(), nil, nil)
	for _, arg := range []string{"/switch", "/switch 0", "/switch abc", "/switch 1 2"} {
		if r := send(t, ag, hub, "telegram", arg); !strings.HasPrefix(r, "Usage: /switch") {
			t.Fatalf("%q: expected usage message, got %q", arg, r)
		}
	}
	if r := send(t, ag, hub, "telegram", "/switch 5"); r != "There is no conversation #5. Use /chats to see your conversations." {
		t.Fatalf("unexpected reply for unknown number: %q", r)
	}
}

func TestSwitchUsesStableNumbers(t *testing.T) {
	ws := t.TempDir()
	hub := chat.NewHub(10)
	ag := NewAgentLoop(hub, &recordingProvider{}, "test", 5, ws, nil, nil)

	send(t, ag, hub, "telegram", "topic one")
	for _, topic := range []string{"topic two", "topic three", "topic four"} {
		send(t, ag, hub, "telegram", "/new")
		send(t, ag, hub, "telegram", topic)
	}
	// Starting another conversation and touching an old one must not renumber anything.
	send(t, ag, hub, "telegram", "/new")
	send(t, ag, hub, "telegram", "topic five")
	send(t, ag, hub, "telegram", "/switch 2")
	send(t, ag, hub, "telegram", "back to two")

	topics := map[int]string{1: "topic one", 2: "topic two", 3: "topic three", 4: "topic four", 5: "topic five"}
	for n, topic := range topics {
		r := send(t, ag, hub, "telegram", fmt.Sprintf("/switch #%d", n))
		if !strings.HasPrefix(r, fmt.Sprintf("Switched to #%d: %s", n, topic)) {
			t.Fatalf("/switch #%d: got %q", n, r)
		}
	}

	// Same numbers after a restart, and the next new conversation is #6.
	hub2 := chat.NewHub(10)
	ag2 := NewAgentLoop(hub2, &recordingProvider{}, "test", 5, ws, nil, nil)
	for n, topic := range topics {
		r := send(t, ag2, hub2, "telegram", fmt.Sprintf("/switch %d", n))
		if !strings.HasPrefix(r, fmt.Sprintf("Switched to #%d: %s", n, topic)) {
			t.Fatalf("after restart, /switch %d: got %q", n, r)
		}
	}
	if r := send(t, ag2, hub2, "telegram", "/new"); !strings.HasPrefix(r, "Started conversation #6") {
		t.Fatalf("after restart, unexpected /new reply: %q", r)
	}
}

func TestChatsListIsCapped(t *testing.T) {
	hub := chat.NewHub(10)
	ag := NewAgentLoop(hub, &recordingProvider{}, "test", 5, t.TempDir(), nil, nil)
	total := maxListedConversations + 5
	send(t, ag, hub, "telegram", "conversation 1")
	for i := 2; i <= total; i++ {
		send(t, ag, hub, "telegram", "/new")
		send(t, ag, hub, "telegram", fmt.Sprintf("conversation %d", i))
	}
	list := send(t, ag, hub, "telegram", "/chats")
	if n := strings.Count(list, "\n#"); n != maxListedConversations {
		t.Fatalf("expected %d listed conversations, got %d:\n%s", maxListedConversations, n, list)
	}
	if !strings.Contains(list, "…and 5 older") || len(list) > 4096 {
		t.Fatalf("expected overflow note within Telegram's limit (len %d):\n%s", len(list), list)
	}
	// Hidden conversations are still reachable by number.
	if r := send(t, ag, hub, "telegram", "/switch 1"); !strings.HasPrefix(r, "Switched to #1: conversation 1") {
		t.Fatalf("unexpected reply: %q", r)
	}
}

func TestCommandWithBotSuffixAndHelp(t *testing.T) {
	hub := chat.NewHub(10)
	ag := NewAgentLoop(hub, &recordingProvider{}, "test", 5, t.TempDir(), nil, nil)
	if r := send(t, ag, hub, "telegram", "/new@picobot_bot Groceries"); !strings.Contains(r, "Started conversation #1") {
		t.Fatalf("group-style command not handled: %q", r)
	}
	if got := ag.sessions.Active("telegram:1").Title; got != "Groceries" {
		t.Fatalf("expected title from /new argument, got %q", got)
	}
	help := send(t, ag, hub, "telegram", "/help")
	for _, c := range chat.Commands {
		if !strings.Contains(help, "/"+c.Name) {
			t.Fatalf("help missing /%s:\n%s", c.Name, help)
		}
	}
}

func TestCommandsOnlyOnSupportedChannels(t *testing.T) {
	hub := chat.NewHub(10)
	prov := &recordingProvider{}
	ag := NewAgentLoop(hub, prov, "test", 5, t.TempDir(), nil, nil)

	// Not a commands channel: goes to the LLM like any other text.
	if r := send(t, ag, hub, "discord", "/new"); r != "reply" {
		t.Fatalf("expected LLM reply on discord, got %q", r)
	}
	// Unknown command on telegram (e.g. /start) also goes to the LLM.
	if r := send(t, ag, hub, "telegram", "/start"); r != "reply" {
		t.Fatalf("expected LLM reply for /start, got %q", r)
	}
}
