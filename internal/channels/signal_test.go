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

// mockSignalSender captures outbound Signal posts for testing.
type mockSignalSender struct {
	mu   sync.Mutex
	sent []mockSignalMessage
}

type mockSignalMessage struct {
	Number    string
	Recipient string
	Message   string
}

func (m *mockSignalSender) Send(_ context.Context, number, recipient, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, mockSignalMessage{Number: number, Recipient: recipient, Message: message})
	return nil
}

func TestStartSignal_EmptyParams(t *testing.T) {
	hub := chat.NewHub(10)
	ctx := context.Background()

	err := StartSignal(ctx, hub, "", "token", "+1234", nil)
	if err == nil || !strings.Contains(err.Error(), "API URL") {
		t.Fatalf("expected API URL error, got: %v", err)
	}

	err = StartSignal(ctx, hub, "http://localhost:8880", "token", "", nil)
	if err == nil || !strings.Contains(err.Error(), "phone number") {
		t.Fatalf("expected phone number error, got: %v", err)
	}
}

func TestBuildSignalWSURL(t *testing.T) {
	tests := []struct {
		name     string
		apiURL   string
		number   string
		expected string
	}{
		{
			name:     "http to ws",
			apiURL:   "http://localhost:8880",
			number:   "+1234567890",
			expected: "ws://localhost:8880/v1/receive/%2B1234567890",
		},
		{
			name:     "https to wss",
			apiURL:   "https://signal.example.com",
			number:   "+1234567890",
			expected: "wss://signal.example.com/v1/receive/%2B1234567890",
		},
		{
			name:     "trailing slash stripped",
			apiURL:   "http://localhost:8880/",
			number:   "+49123456",
			expected: "ws://localhost:8880/v1/receive/%2B49123456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildSignalWSURL(tt.apiURL, tt.number)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("buildSignalWSURL(%q, %q) = %q, want %q", tt.apiURL, tt.number, got, tt.expected)
			}
		})
	}
}

// TestSignal_InboundOutbound verifies the full round-trip: a mock WebSocket
// server sends a Signal envelope, picobot receives it as Inbound, then an
// Outbound reply is sent through the mockSignalSender.
func TestSignal_InboundOutbound(t *testing.T) {
	// Prepare the envelope that the WebSocket will send.
	envelope := signalEnvelope{
		Envelope: &signalEnvelopeData{
			Source:       "+9876543210",
			SourceNumber: "+9876543210",
			SourceName:   "TestUser",
			Timestamp:    1234567890,
			DataMessage: &signalDataMessage{
				Message:   "hello from signal",
				Timestamp: 1234567890,
			},
		},
	}
	envelopeJSON, _ := json.Marshal(envelope)

	// Create a test WebSocket server that sends the envelope then waits.
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsSent := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("ws upgrade error: %v", err)
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Logf("ws close error: %v", err)
			}
		}()

		if err := conn.WriteMessage(websocket.TextMessage, envelopeJSON); err != nil {
			t.Logf("ws write error: %v", err)
			return
		}
		close(wsSent)

		// Keep the connection open until the test ends.
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/receive/%2B1234567890"

	sender := &mockSignalSender{}
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := startSignalWithSender(ctx, hub, sender, wsURL, "test-token", "+1234567890", nil)
	if err != nil {
		t.Fatalf("startSignalWithSender failed: %v", err)
	}
	hub.StartRouter(ctx)

	// Wait for the WebSocket server to send the envelope.
	select {
	case <-wsSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for WebSocket to send envelope")
	}

	// Wait for inbound message.
	select {
	case msg := <-hub.In:
		if msg.Content != "hello from signal" {
			t.Fatalf("unexpected inbound content: %s", msg.Content)
		}
		if msg.Channel != "signal" {
			t.Fatalf("unexpected channel: %s", msg.Channel)
		}
		if msg.SenderID != "+9876543210" {
			t.Fatalf("unexpected sender: %s", msg.SenderID)
		}
		if msg.ChatID != "+9876543210" {
			t.Fatalf("unexpected chat ID: %s", msg.ChatID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for inbound message")
	}

	// Send an outbound reply.
	hub.Out <- chat.Outbound{Channel: "signal", ChatID: "+9876543210", Content: "reply from picobot"}

	// Allow the goroutine time to process.
	time.Sleep(100 * time.Millisecond)

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sender.sent))
	}
	if sender.sent[0].Recipient != "+9876543210" {
		t.Errorf("expected recipient +9876543210, got %s", sender.sent[0].Recipient)
	}
	if sender.sent[0].Message != "reply from picobot" {
		t.Errorf("expected message 'reply from picobot', got %s", sender.sent[0].Message)
	}
	if sender.sent[0].Number != "+1234567890" {
		t.Errorf("expected number +1234567890, got %s", sender.sent[0].Number)
	}
}

// TestSignal_GroupMessage verifies that group messages use the group ID as ChatID.
func TestSignal_GroupMessage(t *testing.T) {
	envelope := signalEnvelope{
		Envelope: &signalEnvelopeData{
			Source:       "+9876543210",
			SourceNumber: "+9876543210",
			DataMessage: &signalDataMessage{
				Message: "hello group",
				GroupInfo: &signalGroupInfo{
					GroupID: "group.abc123def456",
				},
			},
		},
	}
	envelopeJSON, _ := json.Marshal(envelope)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Logf("ws close error: %v", err)
			}
		}()
		if err := conn.WriteMessage(websocket.TextMessage, envelopeJSON); err != nil {
			t.Logf("ws write error: %v", err)
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/receive/%2B1234567890"
	sender := &mockSignalSender{}
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := startSignalWithSender(ctx, hub, sender, wsURL, "token", "+1234567890", nil); err != nil {
		t.Fatalf("startSignalWithSender failed: %v", err)
	}

	select {
	case msg := <-hub.In:
		if msg.ChatID != "group.abc123def456" {
			t.Fatalf("expected group chat ID, got %s", msg.ChatID)
		}
		if msg.SenderID != "+9876543210" {
			t.Fatalf("expected sender +9876543210, got %s", msg.SenderID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for inbound message")
	}
}

// TestSignal_AllowFrom verifies that messages from unauthorized senders are dropped.
func TestSignal_AllowFrom(t *testing.T) {
	envelope := signalEnvelope{
		Envelope: &signalEnvelopeData{
			Source:       "+unauthorized",
			SourceNumber: "+unauthorized",
			DataMessage: &signalDataMessage{
				Message: "should be dropped",
			},
		},
	}
	envelopeJSON, _ := json.Marshal(envelope)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Logf("ws close error: %v", err)
			}
		}()
		if err := conn.WriteMessage(websocket.TextMessage, envelopeJSON); err != nil {
			t.Logf("ws write error: %v", err)
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/receive/%2B1234567890"
	sender := &mockSignalSender{}
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Only allow +alloweduser
	if err := startSignalWithSender(ctx, hub, sender, wsURL, "token", "+1234567890", []string{"+alloweduser"}); err != nil {
		t.Fatalf("startSignalWithSender failed: %v", err)
	}

	// The message should NOT arrive because the sender is not in the allowlist.
	select {
	case msg := <-hub.In:
		t.Fatalf("expected no message, but got: %s", msg.Content)
	case <-time.After(500 * time.Millisecond):
		// Expected: no message received.
	}
}

// TestSignal_IgnoresNonDataMessages verifies that typing indicators, receipts,
// and other non-data envelopes are silently skipped.
func TestSignal_IgnoresNonDataMessages(t *testing.T) {
	// Typing indicator (no dataMessage)
	typing := signalEnvelope{
		Envelope: &signalEnvelopeData{
			Source:       "+9876543210",
			SourceNumber: "+9876543210",
		},
	}
	typingJSON, _ := json.Marshal(typing)

	// Followed by a real message
	real := signalEnvelope{
		Envelope: &signalEnvelopeData{
			Source:       "+9876543210",
			SourceNumber: "+9876543210",
			DataMessage: &signalDataMessage{
				Message: "real message",
			},
		},
	}
	realJSON, _ := json.Marshal(real)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Logf("ws close error: %v", err)
			}
		}()
		// Send typing indicator first, then real message.
		if err := conn.WriteMessage(websocket.TextMessage, typingJSON); err != nil {
			t.Logf("ws write error: %v", err)
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, realJSON); err != nil {
			t.Logf("ws write error: %v", err)
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/receive/%2B1234567890"
	sender := &mockSignalSender{}
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := startSignalWithSender(ctx, hub, sender, wsURL, "token", "+1234567890", nil); err != nil {
		t.Fatalf("startSignalWithSender failed: %v", err)
	}

	// Only the real message should arrive.
	select {
	case msg := <-hub.In:
		if msg.Content != "real message" {
			t.Fatalf("expected 'real message', got %s", msg.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for inbound message")
	}
}

// TestSignal_EchoSuppression verifies that messages from the bot's own number
// are not delivered to the hub.
func TestSignal_EchoSuppression(t *testing.T) {
	// Message from the bot's own number
	echo := signalEnvelope{
		Envelope: &signalEnvelopeData{
			Source:       "+1234567890",
			SourceNumber: "+1234567890",
			DataMessage: &signalDataMessage{
				Message: "echo message",
			},
		},
	}
	echoJSON, _ := json.Marshal(echo)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Logf("ws close error: %v", err)
			}
		}()
		if err := conn.WriteMessage(websocket.TextMessage, echoJSON); err != nil {
			t.Logf("ws write error: %v", err)
			return
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/receive/%2B1234567890"
	sender := &mockSignalSender{}
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := startSignalWithSender(ctx, hub, sender, wsURL, "token", "+1234567890", nil); err != nil {
		t.Fatalf("startSignalWithSender failed: %v", err)
	}

	select {
	case msg := <-hub.In:
		t.Fatalf("expected no message (echo suppression), but got: %s", msg.Content)
	case <-time.After(500 * time.Millisecond):
		// Expected: echo is suppressed.
	}
}

// TestSignal_BearerAuth verifies that the WebSocket connection includes the
// Authorization header when a token is provided.
func TestSignal_BearerAuth(t *testing.T) {
	authReceived := make(chan string, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authReceived <- r.Header.Get("Authorization")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Logf("ws close error: %v", err)
			}
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/receive/%2B1234567890"
	sender := &mockSignalSender{}
	hub := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := startSignalWithSender(ctx, hub, sender, wsURL, "my-secret-token", "+1234567890", nil); err != nil {
		t.Fatalf("startSignalWithSender failed: %v", err)
	}

	select {
	case auth := <-authReceived:
		if auth != "Bearer my-secret-token" {
			t.Fatalf("expected 'Bearer my-secret-token', got %q", auth)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for auth header")
	}
}
