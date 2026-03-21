package channels

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

// mockSlackPoster captures outbound Slack posts for testing without a live connection.
type mockSlackPoster struct {
	mu   sync.Mutex
	sent []string // channel IDs received by PostMessageContext
}

func (m *mockSlackPoster) PostMessageContext(_ context.Context, channelID string, _ ...slack.MsgOption) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, channelID)
	return "", "", nil
}

func TestStartSlack_EmptyTokens(t *testing.T) {
	hub := chat.NewHub(10)
	ctx := context.Background()

	err := StartSlack(ctx, hub, "", "xoxb-test-token", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "app token") {
		t.Fatalf("expected app token error, got: %v", err)
	}

	err = StartSlack(ctx, hub, "xapp-test-token", "", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "bot token") {
		t.Fatalf("expected bot token error, got: %v", err)
	}
}

func TestStartSlack_InvalidTokenPrefixes(t *testing.T) {
	hub := chat.NewHub(10)
	ctx := context.Background()

	err := StartSlack(ctx, hub, "bad-app-token", "xoxb-test", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "xapp-") {
		t.Fatalf("expected xapp- prefix error, got: %v", err)
	}

	err = StartSlack(ctx, hub, "xapp-test", "bad-bot-token", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "xoxb-") {
		t.Fatalf("expected xoxb- prefix error, got: %v", err)
	}
}

// TestSlackClient_Outbound verifies the outbound path delivers messages via the
// slackPoster interface, mirroring the testability of the Discord channel.
func TestSlackClient_Outbound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poster := &mockSlackPoster{}
	hub := chat.NewHub(10)
	c := &slackClient{
		poster: poster,
		hub:    hub,
		outCh:  hub.Subscribe("slack"),
		botID:  "UBOT",
		ctx:    ctx,
	}
	go c.runOutbound()
	hub.StartRouter(ctx)

	hub.Out <- chat.Outbound{Channel: "slack", ChatID: "C123", Content: "hello"}

	// Allow the goroutine time to process.
	time.Sleep(50 * time.Millisecond)

	poster.mu.Lock()
	defer poster.mu.Unlock()
	if len(poster.sent) != 1 {
		t.Fatalf("expected 1 post, got %d", len(poster.sent))
	}
	if poster.sent[0] != "C123" {
		t.Errorf("expected channel C123, got %s", poster.sent[0])
	}
}

// TestSlackClient_OutboundThread verifies that a chatID with a thread timestamp
// posts to the correct channel (the thread reply is carried via MsgOptionTS).
func TestSlackClient_OutboundThread(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poster := &mockSlackPoster{}
	hub := chat.NewHub(10)
	c := &slackClient{
		poster: poster,
		hub:    hub,
		outCh:  hub.Subscribe("slack"),
		botID:  "UBOT",
		ctx:    ctx,
	}
	go c.runOutbound()
	hub.StartRouter(ctx)

	hub.Out <- chat.Outbound{Channel: "slack", ChatID: "C123::1234567890.000001", Content: "threaded reply"}

	time.Sleep(50 * time.Millisecond)

	poster.mu.Lock()
	defer poster.mu.Unlock()
	if len(poster.sent) != 1 {
		t.Fatalf("expected 1 post, got %d", len(poster.sent))
	}
	if poster.sent[0] != "C123" {
		t.Errorf("expected channel C123, got %s", poster.sent[0])
	}
}

func TestSlackChatIDHelpers(t *testing.T) {
	channelID := "C123456"
	threadTS := "1699999999.123456"

	withThread := formatSlackChatID(channelID, threadTS)
	ch, ts := splitSlackChatID(withThread)
	if ch != channelID || ts != threadTS {
		t.Fatalf("expected %s/%s, got %s/%s", channelID, threadTS, ch, ts)
	}

	noThread := formatSlackChatID(channelID, "")
	ch, ts = splitSlackChatID(noThread)
	if ch != channelID || ts != "" {
		t.Fatalf("expected %s with empty thread, got %s/%s", channelID, ch, ts)
	}
}

func TestStripSlackMention(t *testing.T) {
	text := "<@U123> hello there"
	clean := stripSlackMention(text, "U123")
	if clean != " hello there" {
		t.Fatalf("unexpected cleaned text: %q", clean)
	}
}

func TestSlackAllowlists(t *testing.T) {
	c := &slackClient{
		allowedUsers: map[string]struct{}{"U1": {}},
		allowedChans: map[string]struct{}{"C1": {}},
	}

	if !c.isAllowed("U1", "C1", false) {
		t.Fatal("expected allowed user and channel")
	}
	if c.isAllowed("U2", "C1", false) {
		t.Fatal("expected user U2 to be blocked")
	}
	if c.isAllowed("U1", "C2", false) {
		t.Fatal("expected channel C2 to be blocked")
	}

	open := &slackClient{allowedUsers: map[string]struct{}{}, allowedChans: map[string]struct{}{}}
	if !open.isAllowed("U999", "C999", false) {
		t.Fatal("expected empty allowlists to permit all")
	}

	if !c.isAllowed("U1", "D1", true) {
		t.Fatal("expected DM to bypass channel allowlist")
	}
}

func TestSlackAttachmentAppend(t *testing.T) {
	files := []slackevents.File{
		{URLPrivate: "https://files.example.com/a"},
		{URLPrivateDownload: "https://files.example.com/b"},
		{Permalink: "https://files.example.com/c"},
	}

	content := appendSlackAttachments("", files)
	if content == "" {
		t.Fatal("expected attachment content")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		values []string
		want   string
	}{
		{[]string{"", "b", "c"}, "b"},
		{[]string{"a", "b"}, "a"},
		{[]string{"", ""}, ""},
		{[]string{}, ""},
	}
	for _, tt := range tests {
		got := firstNonEmpty(tt.values...)
		if got != tt.want {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
		}
	}
}
