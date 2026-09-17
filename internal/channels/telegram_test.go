package channels

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
)

func TestStartTelegramWithBase(t *testing.T) {
	token := "testtoken"
	// channels to capture sendMessage and sendMessageDraft posts
	sent := make(chan url.Values, 4)
	drafts := make(chan url.Values, 4)
	docsSent := make(chan string, 4)

	// simple stateful handler: first getUpdates returns one update, subsequent return empty
	first := true
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/getUpdates") {
			w.Header().Set("Content-Type", "application/json")
			if first {
				first = false
				w.Write([]byte(`{"ok":true,"result":[{"update_id":1,"message":{"message_id":1,"from":{"id":123},"chat":{"id":456,"type":"private"},"text":"hello"}}]}`))
				return
			}
			w.Write([]byte(`{"ok":true,"result":[]}`))
			return
		}
		if strings.HasSuffix(path, "/sendMessageDraft") {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			drafts <- r.PostForm
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true,"result":{}}`))
			return
		}
		if strings.HasSuffix(path, "/sendMessage") {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			sent <- r.PostForm
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true,"result":{}}`))
			return
		}
		if strings.HasSuffix(path, "/sendDocument") {
			if err := r.ParseMultipartForm(10 * 1024 * 1024); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			docsSent <- r.FormValue("chat_id")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true,"result":{}}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer h.Close()

	base := h.URL + "/bot" + token
	b := chat.NewHub(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := StartTelegramWithBase(ctx, b, token, base, nil); err != nil {
		t.Fatalf("StartTelegramWithBase failed: %v", err)
	}
	// Start the hub router so outbound messages sent to b.Out are dispatched
	// to each channel's subscription (telegram in this test).
	b.StartRouter(ctx)

	// Wait for the initial "Thinking..." draft from the inbound handler
	select {
	case v := <-drafts:
		if v.Get("chat_id") != "456" || v.Get("text") != "" {
			t.Fatalf("unexpected initial draft form: %v", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initial draft message")
	}

	// Wait for inbound from getUpdates
	select {
	case msg := <-b.In:
		if msg.Content != "hello" {
			t.Fatalf("unexpected inbound content: %s", msg.Content)
		}
		if msg.ChatID != "456" {
			t.Fatalf("unexpected chat id: %s", msg.ChatID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for inbound message")
	}

	// Send an outbound notification message
	notif := chat.Outbound{
		Channel:  "telegram",
		ChatID:   "456",
		Content:  "thinking 1",
		Metadata: map[string]interface{}{"is_notification": true},
	}
	b.Out <- notif

	select {
	case v := <-drafts:
		if v.Get("chat_id") != "456" || v.Get("text") != "thinking 1" {
			t.Fatalf("unexpected draft notification form: %v", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for draft notification message")
	}

	// send an outbound final message and ensure server receives it
	out := chat.Outbound{Channel: "telegram", ChatID: "456", Content: "reply"}
	b.Out <- out

	select {
	case v := <-sent:
		if v.Get("chat_id") != "456" || v.Get("text") != "reply" {
			t.Fatalf("unexpected sendMessage form: %v", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sendMessage to be posted")
	}

	// Test sending a file
	tmpFile, err := os.CreateTemp("", "picobot-test-media-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte("test file content")); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	outWithFile := chat.Outbound{
		Channel: "telegram",
		ChatID:  "456",
		Content: "here is a file",
		Media:   []string{tmpFile.Name()},
	}
	b.Out <- outWithFile

	// Expect the message text first
	select {
	case v := <-sent:
		if v.Get("chat_id") != "456" || v.Get("text") != "here is a file" {
			t.Fatalf("unexpected sendMessage form: %v", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sendMessage text")
	}

	// Expect the document next
	select {
	case chatID := <-docsSent:
		if chatID != "456" {
			t.Fatalf("unexpected sendDocument chatID: %s", chatID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sendDocument")
	}

	// cancel and allow goroutines to stop
	cancel()
	// give a small grace period
	time.Sleep(50 * time.Millisecond)
}

func TestTelegramCallbackQuery(t *testing.T) {
	token := "testtoken"
	answered := make(chan string, 1)
	edited := make(chan string, 1)

	first := true
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(path, "/getUpdates") {
			if first {
				first = false
				w.Write([]byte(`{
					"ok": true,
					"result": [{
						"update_id": 100,
						"callback_query": {
							"id": "query123",
							"from": {"id": 1269963921},
							"message": {
								"message_id": 987,
								"chat": {"id": 456}
							},
							"data": "yes"
						}
					}]
				}`))
				return
			}
			w.Write([]byte(`{"ok":true,"result":[]}`))
			return
		}
		if strings.HasSuffix(path, "/answerCallbackQuery") {
			r.ParseForm()
			answered <- r.FormValue("callback_query_id")
			w.Write([]byte(`{"ok":true}`))
			return
		}
		if strings.HasSuffix(path, "/editMessageReplyMarkup") {
			r.ParseForm()
			edited <- r.FormValue("message_id")
			w.Write([]byte(`{"ok":true}`))
			return
		}
	}))
	defer h.Close()

	b := chat.NewHub(2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := StartTelegramWithBase(ctx, b, token, h.URL, []string{"1269963921"})
	if err != nil {
		t.Fatalf("failed to start Telegram: %v", err)
	}

	select {
	case msg := <-b.In:
		if msg.Content != "yes" || msg.ChatID != "456" || msg.SenderID != "1269963921" {
			t.Fatalf("unexpected inbound message: %+v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for callback query to route to Inbound")
	}

	select {
	case id := <-answered:
		if id != "query123" {
			t.Fatalf("unexpected callback query ID answered: %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for callback query to be answered")
	}

	select {
	case id := <-edited:
		if id != "987" {
			t.Fatalf("unexpected message ID edited: %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message reply markup to be edited")
	}
}
