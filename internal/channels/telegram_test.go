package channels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
)

func TestStartTelegramWithBase(t *testing.T) {
	token := "testtoken"
	// channel to capture sendMessage posts
	sent := make(chan url.Values, 4)

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

	// send an outbound message and ensure server receives it
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

	// cancel and allow goroutines to stop
	cancel()
	// give a small grace period
	time.Sleep(50 * time.Millisecond)
}

func TestTelegramRegistersCommands(t *testing.T) {
	registered := make(chan string, 1)
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/setMyCommands"):
			_ = r.ParseForm()
			registered <- r.PostForm.Get("commands")
			w.Write([]byte(`{"ok":true,"result":true}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			w.Write([]byte(`{"ok":true,"result":[]}`))
		default:
			w.Write([]byte(`{"ok":true,"result":{}}`))
		}
	}))
	defer h.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := StartTelegramWithBase(ctx, chat.NewHub(10), "tok", h.URL+"/bottok", nil); err != nil {
		t.Fatal(err)
	}

	select {
	case raw := <-registered:
		var cmds []struct {
			Command     string `json:"command"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(raw), &cmds); err != nil {
			t.Fatalf("invalid commands payload %q: %v", raw, err)
		}
		if len(cmds) != len(chat.Commands) {
			t.Fatalf("expected %d commands, got %+v", len(chat.Commands), cmds)
		}
		for i, c := range chat.Commands {
			if cmds[i].Command != c.Name || cmds[i].Description == "" {
				t.Fatalf("unexpected command %d: %+v", i, cmds[i])
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for setMyCommands")
	}
}
