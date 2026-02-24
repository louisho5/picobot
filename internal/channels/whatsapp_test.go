//go:build !lite

package channels

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/local/picobot/internal/chat"
)

// mockWhatsAppSender records all outbound calls for assertions.
type mockWhatsAppSender struct {
	mu    sync.Mutex
	texts []struct {
		to   types.JID
		text string
	}
	chatPresences []struct {
		chat  types.JID
		state types.ChatPresence
	}
	markedRead []types.MessageID
	presences  []types.Presence
	sendErr    error
}

func (m *mockWhatsAppSender) SendText(_ context.Context, to types.JID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, struct {
		to   types.JID
		text string
	}{to, text})
	return m.sendErr
}

func (m *mockWhatsAppSender) SendChatPresence(_ context.Context, chat types.JID, state types.ChatPresence, _ types.ChatPresenceMedia) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chatPresences = append(m.chatPresences, struct {
		chat  types.JID
		state types.ChatPresence
	}{chat, state})
	return nil
}

func (m *mockWhatsAppSender) MarkRead(_ context.Context, ids []types.MessageID, _ time.Time, _ types.JID, _ types.JID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markedRead = append(m.markedRead, ids...)
	return nil
}

func (m *mockWhatsAppSender) SendPresence(_ context.Context, state types.Presence) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.presences = append(m.presences, state)
	return nil
}

func (m *mockWhatsAppSender) sentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.texts)
}

// makeWhatsAppMsg builds a minimal *events.Message for tests.
func makeWhatsAppMsg(senderUser string, isFromMe, isGroup bool, text string) *events.Message {
	jid := types.JID{User: senderUser, Server: "s.whatsapp.net"}
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     jid,
				Sender:   jid,
				IsFromMe: isFromMe,
				IsGroup:  isGroup,
			},
			ID:        "testmsg001",
			Timestamp: time.Now(),
		},
		Message: &waProto.Message{Conversation: &text},
	}
}

// --- StartWhatsApp / SetupWhatsApp guard tests ---

func TestStartWhatsApp_EmptyDBPath(t *testing.T) {
	err := StartWhatsApp(context.Background(), chat.NewHub(10), "", nil)
	if err == nil || err.Error() != "whatsapp database path not provided" {
		t.Fatalf("expected 'whatsapp database path not provided', got %v", err)
	}
}

func TestSetupWhatsApp_EmptyDBPath(t *testing.T) {
	err := SetupWhatsApp("")
	if err == nil || err.Error() != "whatsapp database path not provided" {
		t.Fatalf("expected 'whatsapp database path not provided', got %v", err)
	}
}

// --- handleMessage tests ---

func TestWhatsAppClient_HandleMessage_Inbound(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockWhatsAppSender{}
	c := newWhatsAppClient(ctx, mock, hub, nil, types.JID{}, types.JID{}) // no allowlist

	text := "hello bot"
	c.handleMessage(makeWhatsAppMsg("15551234567", false, false, text))

	select {
	case msg := <-hub.In:
		if msg.Content != text {
			t.Errorf("Content = %q, want %q", msg.Content, text)
		}
		if msg.SenderID != "15551234567" {
			t.Errorf("SenderID = %q, want %q", msg.SenderID, "15551234567")
		}
		if msg.Channel != "whatsapp" {
			t.Errorf("Channel = %q, want whatsapp", msg.Channel)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for inbound message")
	}

	mock.mu.Lock()
	readCount := len(mock.markedRead)
	mock.mu.Unlock()
	if readCount == 0 {
		t.Error("expected MarkRead to be called for read receipts")
	}
}

func TestWhatsAppClient_HandleMessage_SkipsFromMe(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newWhatsAppClient(ctx, &mockWhatsAppSender{}, hub, nil, types.JID{}, types.JID{})

	c.handleMessage(makeWhatsAppMsg("15551234567", true /* IsFromMe */, false, "ignore me"))

	select {
	case msg := <-hub.In:
		t.Errorf("should have dropped own message, got %q", msg.Content)
	case <-time.After(50 * time.Millisecond):
		// expected: dropped
	}
}

func TestWhatsAppClient_HandleMessage_SelfChat(t *testing.T) {
	// When the bot is linked to phone 85298765432 (LID 169032883908635),
	// the user can text themselves (Notes to Self) to interact with the bot.
	// The self-chat chat JID may use either the phone server or the LID server.

	phoneJID := types.JID{User: "85298765432", Server: "s.whatsapp.net"}
	lidJID := types.JID{User: "169032883908635", Server: "lid"}

	tests := []struct {
		name     string
		chatUser string // simulates msg.Info.Chat.User
		server   string
	}{
		{"phone server self-chat", "85298765432", "s.whatsapp.net"},
		{"LID server self-chat", "169032883908635", "lid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := chat.NewHub(10)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := newWhatsAppClient(ctx, &mockWhatsAppSender{}, hub, nil, phoneJID, lidJID)

			// Override the chat JID to simulate the server type under test.
			msg := makeWhatsAppMsg(tt.chatUser, true /* IsFromMe */, false, "remind me later")
			msg.Info.Chat = types.JID{User: tt.chatUser, Server: tt.server}

			c.handleMessage(msg)

			select {
			case in := <-hub.In:
				if in.Content != "remind me later" {
					t.Errorf("Content = %q, want %q", in.Content, "remind me later")
				}
			case <-time.After(time.Second):
				t.Fatalf("timeout: self-chat via %s should be processed", tt.server)
			}
		})
	}
}

func TestWhatsAppClient_HandleMessage_SelfChat_OtherConversation(t *testing.T) {
	// IsFromMe=true but the chat is someone else ➜ must be dropped (echo of sent msg).
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ownJID := types.JID{User: "85298765432", Server: "s.whatsapp.net"}
	ownLID := types.JID{User: "169032883908635", Server: "lid"}
	c := newWhatsAppClient(ctx, &mockWhatsAppSender{}, hub, nil, ownJID, ownLID)

	// Sent to someone else's number.
	c.handleMessage(makeWhatsAppMsg("99999999999", true /* IsFromMe */, false, "echo"))

	select {
	case msg := <-hub.In:
		t.Errorf("should have dropped echo message, got %q", msg.Content)
	case <-time.After(50 * time.Millisecond):
		// expected: dropped
	}
}

func TestWhatsAppClient_HandleMessage_SkipsGroup(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newWhatsAppClient(ctx, &mockWhatsAppSender{}, hub, nil, types.JID{}, types.JID{})

	c.handleMessage(makeWhatsAppMsg("15551234567", false, true /* IsGroup */, "group msg"))

	select {
	case msg := <-hub.In:
		t.Errorf("should have dropped group message, got %q", msg.Content)
	case <-time.After(50 * time.Millisecond):
		// expected: dropped
	}
}

func TestWhatsAppClient_HandleMessage_SkipsEmpty(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newWhatsAppClient(ctx, &mockWhatsAppSender{}, hub, nil, types.JID{}, types.JID{})

	empty := ""
	evt := makeWhatsAppMsg("15551234567", false, false, "")
	evt.Message = &waProto.Message{Conversation: &empty}
	c.handleMessage(evt)

	select {
	case msg := <-hub.In:
		t.Errorf("should have dropped empty message, got %q", msg.Content)
	case <-time.After(50 * time.Millisecond):
		// expected: dropped
	}
}

func TestWhatsAppClient_HandleMessage_AllowList_Blocked(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newWhatsAppClient(ctx, &mockWhatsAppSender{}, hub, []string{"19999999999"}, types.JID{}, types.JID{})

	c.handleMessage(makeWhatsAppMsg("15551234567", false, false, "from blocked user"))

	select {
	case msg := <-hub.In:
		t.Errorf("should have dropped message from unlisted user, got %q", msg.Content)
	case <-time.After(50 * time.Millisecond):
		// expected: dropped
	}
}

func TestWhatsAppClient_HandleMessage_AllowList_Permitted(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newWhatsAppClient(ctx, &mockWhatsAppSender{}, hub, []string{"15551234567"}, types.JID{}, types.JID{})

	text := "permitted message"
	c.handleMessage(makeWhatsAppMsg("15551234567", false, false, text))

	select {
	case msg := <-hub.In:
		if msg.Content != text {
			t.Errorf("Content = %q, want %q", msg.Content, text)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for inbound message from permitted user")
	}
}

func TestWhatsAppClient_HandleMessage_AllowList_OpenAccess(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newWhatsAppClient(ctx, &mockWhatsAppSender{}, hub, nil, types.JID{}, types.JID{}) // nil allowlist = allow all

	c.handleMessage(makeWhatsAppMsg("19876543210", false, false, "anyone can message"))

	select {
	case <-hub.In:
		// expected: message accepted
	case <-time.After(time.Second):
		t.Fatal("timeout: open-access should accept any sender")
	}
}

// --- Outbound tests ---

func TestWhatsAppClient_Outbound(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockWhatsAppSender{}
	c := newWhatsAppClient(ctx, mock, hub, nil, types.JID{}, types.JID{})
	hub.StartRouter(ctx)
	go c.runOutbound()

	hub.Out <- chat.Outbound{
		Channel: "whatsapp",
		ChatID:  "15551234567@s.whatsapp.net",
		Content: "hello",
	}

	deadline := time.After(2 * time.Second)
	for {
		if mock.sentCount() >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout: expected 1 sent text, got %d", mock.sentCount())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.texts[0].text != "hello" {
		t.Errorf("sent text = %q, want %q", mock.texts[0].text, "hello")
	}
}

func TestWhatsAppClient_Outbound_OtherChannelIgnored(t *testing.T) {
	// Messages destined for a different channel must not be sent by this client.
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockWhatsAppSender{}
	c := newWhatsAppClient(ctx, mock, hub, nil, types.JID{}, types.JID{})
	hub.StartRouter(ctx)
	go c.runOutbound()

	// Send to the telegram channel – the whatsapp subscriber must not pick it up.
	hub.Out <- chat.Outbound{Channel: "telegram", ChatID: "123456", Content: "not for whatsapp"}

	time.Sleep(100 * time.Millisecond)
	if mock.sentCount() != 0 {
		t.Errorf("expected 0 sent messages for wrong-channel outbound, got %d", mock.sentCount())
	}
}

func TestWhatsAppClient_Outbound_LongMessageSplit(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockWhatsAppSender{}
	c := newWhatsAppClient(ctx, mock, hub, nil, types.JID{}, types.JID{})
	hub.StartRouter(ctx)
	go c.runOutbound()

	// Message over the 4096-rune chunk limit.
	longText := strings.Repeat("a", 5000)
	hub.Out <- chat.Outbound{Channel: "whatsapp", ChatID: "15551234567@s.whatsapp.net", Content: longText}

	deadline := time.After(2 * time.Second)
	for {
		if mock.sentCount() >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout: expected ≥2 chunks for 5000-char message, got %d", mock.sentCount())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// --- extractMessageText tests ---

func TestExtractMessageText(t *testing.T) {
	hello := "Hello"
	caption := "look at this"
	docName := "report.pdf"

	tests := []struct {
		name     string
		msg      *waProto.Message
		contains string
		empty    bool
	}{
		{"nil message", nil, "", true},
		{"conversation", &waProto.Message{Conversation: &hello}, "Hello", false},
		{"extended text", &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: &hello}}, "Hello", false},
		{"image no caption", &waProto.Message{ImageMessage: &waProto.ImageMessage{}}, "[Image received", false},
		{"image with caption", &waProto.Message{ImageMessage: &waProto.ImageMessage{Caption: &caption}}, caption, false},
		{"document with filename", &waProto.Message{DocumentMessage: &waProto.DocumentMessage{FileName: &docName}}, "report.pdf", false},
		{"empty proto", &waProto.Message{}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMessageText(tt.msg)
			if tt.empty {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
			} else if !strings.Contains(got, tt.contains) {
				t.Errorf("extractMessageText() = %q, want to contain %q", got, tt.contains)
			}
		})
	}
}

// --- handleEvent tests ---

func TestWhatsAppClient_HandleEvent_SendsPresence(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockWhatsAppSender{}
	c := newWhatsAppClient(ctx, mock, hub, nil, types.JID{}, types.JID{})

	c.handleEvent(&events.PushNameSetting{})

	time.Sleep(20 * time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.presences) == 0 {
		t.Error("expected SendPresence to be called on PushNameSetting event")
	}
	if mock.presences[0] != types.PresenceAvailable {
		t.Errorf("presence = %v, want PresenceAvailable", mock.presences[0])
	}
}

// --- typing indicator tests ---

func TestWhatsAppClient_StopTyping_NoPanic(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newWhatsAppClient(ctx, &mockWhatsAppSender{}, hub, nil, types.JID{}, types.JID{})

	// Stopping a chat that never had typing started should not panic.
	c.stopTyping("15551234567@s.whatsapp.net")
}

func TestWhatsAppClient_StopAllTyping(t *testing.T) {
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newWhatsAppClient(ctx, &mockWhatsAppSender{}, hub, nil, types.JID{}, types.JID{})

	// Manually inject stops to simulate active typing indicators.
	c.typingMu.Lock()
	c.typingStop["chat1@s.whatsapp.net"] = make(chan struct{})
	c.typingStop["chat2@s.whatsapp.net"] = make(chan struct{})
	c.typingMu.Unlock()

	c.stopAllTyping() // should close all channels without panic

	c.typingMu.Lock()
	remaining := len(c.typingStop)
	c.typingMu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 typing stops after stopAllTyping, got %d", remaining)
	}
}
