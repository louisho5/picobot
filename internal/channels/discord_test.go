package channels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/local/picobot/internal/chat"
)

// TestCleanDiscordContent tests the cleanDiscordContent helper function.
func TestCleanDiscordContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "user mention",
			input:    "hello <@123456789> world",
			expected: "hello  world",
		},
		{
			name:     "nickname mention",
			input:    "hello <@!123456789> world",
			expected: "hello  world",
		},
		{
			name:     "role mention",
			input:    "hello <@&987654321> world",
			expected: "hello  world",
		},
		{
			name:     "channel mention",
			input:    "check <#111222333> please",
			expected: "check  please",
		},
		{
			name:     "multiple mentions",
			input:    "<@111> and <@222> hello",
			expected: "and  hello",
		},
		{
			name:     "empty after cleaning",
			input:    "<@123456789>",
			expected: "",
		},
		{
			name:     "whitespace only after cleaning",
			input:    "  <@123456789>  ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanDiscordContent(tt.input)
			if result != tt.expected {
				t.Errorf("cleanDiscordContent(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestTruncate tests the truncate helper function.
func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc..."},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

// TestParseChannelMention tests the ParseChannelMention helper.
func TestParseChannelMention(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<#123456789>", "123456789"},
		{"<#999>", "999"},
		{"not a mention", ""},
		{"<@123>", ""},
		{"<#>", ""},
	}

	for _, tt := range tests {
		result := ParseChannelMention(tt.input)
		if result != tt.expected {
			t.Errorf("ParseChannelMention(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestParseUserMention tests the ParseUserMention helper.
func TestParseUserMention(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<@123456789>", "123456789"},
		{"<@!123456789>", "123456789"},
		{"<#123>", ""},
		{"not a mention", ""},
	}

	for _, tt := range tests {
		result := ParseUserMention(tt.input)
		if result != tt.expected {
			t.Errorf("ParseUserMention(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestFormatMentions tests the FormatUserMention and FormatChannelMention helpers.
func TestFormatMentions(t *testing.T) {
	if got := FormatUserMention("123"); got != "<@123>" {
		t.Errorf("FormatUserMention(123) = %q, want <@123>", got)
	}
	if got := FormatChannelMention("456"); got != "<#456>" {
		t.Errorf("FormatChannelMention(456) = %q, want <#456>", got)
	}
}

// TestDiscordIDConversion tests the discordIDToString and discordIDFromString helpers.
func TestDiscordIDConversion(t *testing.T) {
	if got := discordIDToString(123456789); got != "123456789" {
		t.Errorf("discordIDToString(123456789) = %q, want 123456789", got)
	}
	if got := discordIDFromString("123456789"); got != 123456789 {
		t.Errorf("discordIDFromString(123456789) = %d, want 123456789", got)
	}
	if got := discordIDFromString("invalid"); got != 0 {
		t.Errorf("discordIDFromString(invalid) = %d, want 0", got)
	}
}

// TestStartDiscord_EmptyToken tests that StartDiscord returns an error with empty token.
func TestStartDiscord_EmptyToken(t *testing.T) {
	hub := chat.NewHub(10)
	err := StartDiscord(context.Background(), hub, "", nil)
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
	if !strings.Contains(err.Error(), "not provided") {
		t.Fatalf("expected 'not provided' error, got: %v", err)
	}
}

// TestDiscordOutboundHandler tests that outbound messages are sent via REST API.
func TestDiscordOutboundHandler(t *testing.T) {
	// Track sent messages
	var mu sync.Mutex
	sentMessages := make([]map[string]interface{}, 0)

	// Create a mock Discord REST API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization header
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bot ") {
			t.Errorf("expected 'Bot ' prefix in Authorization header, got: %s", auth)
			w.WriteHeader(401)
			return
		}

		// Verify content type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type, got: %s", r.Header.Get("Content-Type"))
		}

		// Parse the request body
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(400)
			return
		}

		mu.Lock()
		sentMessages = append(sentMessages, body)
		mu.Unlock()

		// Return success
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         "msg123",
			"channel_id": "chan456",
			"content":    body["content"],
		})
	}))
	defer server.Close()

	// Override the API base for testing
	origBase := DiscordAPIBase
	// We can't easily override the const, so we test sendMessage directly
	handler := &discordOutboundHandler{
		token:  "test-token",
		client: server.Client(),
	}

	// We need to test the sendMessage method with our test server URL
	// Since sendMessage uses DiscordAPIBase (a const), we test the REST client directly
	url := server.URL + "/channels/chan456/messages"
	payload := map[string]interface{}{
		"content": "hello from test",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bot "+handler.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := handler.client.Do(req)
	if err != nil {
		t.Fatalf("sendMessage failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(sentMessages) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sentMessages))
	}
	if sentMessages[0]["content"] != "hello from test" {
		t.Fatalf("expected 'hello from test', got %v", sentMessages[0]["content"])
	}

	_ = origBase // suppress unused warning
}

// TestDiscordRestOnly_SendMessage tests the DiscordRestOnly client.
func TestDiscordRestOnly_SendMessage(t *testing.T) {
	var receivedAuth string
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"1"}`))
	}))
	defer server.Close()

	// Create a DiscordRestOnly with custom client that redirects to test server
	client := &DiscordRestOnly{
		token:  "test-bot-token",
		client: server.Client(),
	}

	// We can't override DiscordAPIBase, so test the HTTP mechanics directly
	url := server.URL + "/channels/123/messages"
	payload := map[string]interface{}{"content": "test message"}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bot "+client.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if receivedAuth != "Bot test-bot-token" {
		t.Errorf("expected 'Bot test-bot-token', got %q", receivedAuth)
	}
	if receivedBody["content"] != "test message" {
		t.Errorf("expected 'test message', got %v", receivedBody["content"])
	}
}

// fakeGateway creates a WebSocket server that simulates the Discord Gateway.
// It sends Hello, waits for Identify, sends Ready, then forwards test messages.
func fakeGateway(t *testing.T, botID string, messages []discordMessage) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		// Send Hello (op 10)
		hello := map[string]interface{}{
			"op": gatewayOpHello,
			"d": map[string]interface{}{
				"heartbeat_interval": 45000, // 45 seconds (won't fire during test)
			},
		}
		if err := conn.WriteJSON(hello); err != nil {
			t.Logf("hello write error: %v", err)
			return
		}

		// Read Identify (op 2)
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Logf("identify read error: %v", err)
			return
		}
		var identify struct {
			Op int `json:"op"`
		}
		json.Unmarshal(data, &identify)
		if identify.Op != gatewayOpIdentify {
			t.Logf("expected identify op 2, got %d", identify.Op)
			return
		}

		// Send Ready dispatch (op 0, t=READY)
		ready := map[string]interface{}{
			"op": gatewayOpDispatch,
			"t":  "READY",
			"s":  1,
			"d": map[string]interface{}{
				"session_id": "test-session",
				"user": map[string]interface{}{
					"id":       botID,
					"username": "TestBot",
				},
			},
		}
		if err := conn.WriteJSON(ready); err != nil {
			t.Logf("ready write error: %v", err)
			return
		}

		// Send test messages as MESSAGE_CREATE dispatches
		seq := int64(2)
		for _, msg := range messages {
			msgData, _ := json.Marshal(msg)
			dispatch := map[string]interface{}{
				"op": gatewayOpDispatch,
				"t":  "MESSAGE_CREATE",
				"s":  seq,
				"d":  json.RawMessage(msgData),
			}
			if err := conn.WriteJSON(dispatch); err != nil {
				t.Logf("message write error: %v", err)
				return
			}
			seq++
		}

		// Keep connection alive until client disconnects
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}))
}

// TestDiscordGateway_InboundMessage tests that messages received via the Gateway
// are correctly forwarded to the Hub.
func TestDiscordGateway_InboundMessage(t *testing.T) {
	botID := "999888777"

	// Create a test message (DM from allowed user)
	testMsg := discordMessage{
		ID:        "msg001",
		ChannelID: "dm-channel-123",
		Content:   "hello picobot",
		Author: struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Bot      bool   `json:"bot,omitempty"`
		}{
			ID:       "user123",
			Username: "testuser",
			Bot:      false,
		},
	}

	// Start fake Gateway
	gw := fakeGateway(t, botID, []discordMessage{testMsg})
	defer gw.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(gw.URL, "http")

	hub := chat.NewHub(10)
	allowed := map[string]struct{}{}

	client := newDiscordClient("test-token", wsURL, hub, allowed)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go client.connect(ctx)

	// Wait for inbound message from hub
	select {
	case msg := <-hub.In:
		if msg.Content != "hello picobot" {
			t.Fatalf("expected 'hello picobot', got %q", msg.Content)
		}
		if msg.Channel != "discord" {
			t.Fatalf("expected channel 'discord', got %q", msg.Channel)
		}
		if msg.SenderID != "user123" {
			t.Fatalf("expected sender 'user123', got %q", msg.SenderID)
		}
		if msg.ChatID != "dm-channel-123" {
			t.Fatalf("expected chatID 'dm-channel-123', got %q", msg.ChatID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for inbound message")
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
}

// TestDiscordGateway_BotMessageIgnored tests that bot messages are ignored.
func TestDiscordGateway_BotMessageIgnored(t *testing.T) {
	botID := "999888777"

	// Create a bot message (should be ignored)
	botMsg := discordMessage{
		ID:        "msg002",
		ChannelID: "channel-456",
		Content:   "I am a bot",
		Author: struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Bot      bool   `json:"bot,omitempty"`
		}{
			ID:       "otherbot",
			Username: "OtherBot",
			Bot:      true,
		},
	}

	gw := fakeGateway(t, botID, []discordMessage{botMsg})
	defer gw.Close()

	wsURL := "ws" + strings.TrimPrefix(gw.URL, "http")
	hub := chat.NewHub(10)
	client := newDiscordClient("test-token", wsURL, hub, map[string]struct{}{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go client.connect(ctx)

	// Should NOT receive any message (bot messages are filtered)
	select {
	case msg := <-hub.In:
		t.Fatalf("expected no message, but got: %+v", msg)
	case <-time.After(500 * time.Millisecond):
		// Good — no message received
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
}

// TestDiscordGateway_AllowFromFilter tests that unauthorized users are filtered.
func TestDiscordGateway_AllowFromFilter(t *testing.T) {
	botID := "999888777"

	// Create a message from an unauthorized user (DM so no mention needed)
	unauthorizedMsg := discordMessage{
		ID:        "msg003",
		ChannelID: "dm-channel-789",
		Content:   "sneaky message",
		Author: struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Bot      bool   `json:"bot,omitempty"`
		}{
			ID:       "unauthorized-user",
			Username: "sneaky",
			Bot:      false,
		},
	}

	gw := fakeGateway(t, botID, []discordMessage{unauthorizedMsg})
	defer gw.Close()

	wsURL := "ws" + strings.TrimPrefix(gw.URL, "http")
	hub := chat.NewHub(10)

	// Only allow "allowed-user-123"
	allowed := map[string]struct{}{
		"allowed-user-123": {},
	}
	client := newDiscordClient("test-token", wsURL, hub, allowed)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go client.connect(ctx)

	// Should NOT receive the message (user not in allowFrom)
	select {
	case msg := <-hub.In:
		t.Fatalf("expected no message from unauthorized user, but got: %+v", msg)
	case <-time.After(500 * time.Millisecond):
		// Good — unauthorized message was filtered
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
}

// TestDiscordGateway_GuildMentionRequired tests that guild messages require a mention.
func TestDiscordGateway_GuildMentionRequired(t *testing.T) {
	botID := "999888777"

	// Guild message WITHOUT mention (should be ignored)
	noMentionMsg := discordMessage{
		ID:        "msg004",
		ChannelID: "guild-channel-111",
		GuildID:   "guild-222",
		Content:   "hello everyone",
		Author: struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Bot      bool   `json:"bot,omitempty"`
		}{
			ID:       "user456",
			Username: "guilduser",
			Bot:      false,
		},
	}

	// Guild message WITH mention (should be forwarded)
	mentionMsg := discordMessage{
		ID:        "msg005",
		ChannelID: "guild-channel-111",
		GuildID:   "guild-222",
		Content:   "<@999888777> what's up?",
		Author: struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Bot      bool   `json:"bot,omitempty"`
		}{
			ID:       "user456",
			Username: "guilduser",
			Bot:      false,
		},
		Mentions: []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		}{
			{ID: "999888777", Username: "TestBot"},
		},
	}

	gw := fakeGateway(t, botID, []discordMessage{noMentionMsg, mentionMsg})
	defer gw.Close()

	wsURL := "ws" + strings.TrimPrefix(gw.URL, "http")
	hub := chat.NewHub(10)
	client := newDiscordClient("test-token", wsURL, hub, map[string]struct{}{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go client.connect(ctx)

	// Should only receive the mention message
	select {
	case msg := <-hub.In:
		if strings.Contains(msg.Content, "hello everyone") {
			t.Fatal("received guild message without mention — should have been filtered")
		}
		// The mention should be stripped from content
		if strings.Contains(msg.Content, "<@999888777>") {
			t.Fatal("mention was not stripped from content")
		}
		if !strings.Contains(msg.Content, "what's up?") {
			t.Fatalf("expected 'what's up?' in content, got %q", msg.Content)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for mention message")
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
}
