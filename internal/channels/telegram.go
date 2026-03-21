package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/local/picobot/internal/chat"
)

// StartTelegram is a convenience wrapper that uses the real polling implementation
// with the standard Telegram base URL.
// allowFrom is a list of Telegram user IDs permitted to interact with the bot.
// If empty, ALL users are allowed (open mode).
func StartTelegram(ctx context.Context, hub *chat.Hub, token string, allowFrom []string) error {
	if token == "" {
		return fmt.Errorf("telegram token not provided")
	}
	base := "https://api.telegram.org/bot" + token
	return StartTelegramWithBase(ctx, hub, token, base, allowFrom)
}

// StartTelegramWithBase starts long-polling against the given base URL (e.g., https://api.telegram.org/bot<TOKEN> or a test server URL).
// allowFrom restricts which Telegram user IDs may send messages. Empty means allow all.
func StartTelegramWithBase(ctx context.Context, hub *chat.Hub, token, base string, allowFrom []string) error {
	if base == "" {
		return fmt.Errorf("base URL is required")
	}

	// Build a fast lookup set for allowed user IDs.
	allowed := make(map[string]struct{}, len(allowFrom))
	for _, id := range allowFrom {
		allowed[id] = struct{}{}
	}

	client := &http.Client{Timeout: 45 * time.Second}

	// inbound polling goroutine
	go func() {
		offset := int64(0)
		for {
			select {
			case <-ctx.Done():
				log.Println("telegram: stopping inbound polling")
				return
			default:
			}

			values := url.Values{}
			values.Set("offset", strconv.FormatInt(offset, 10))
			values.Set("timeout", "30")
			u := base + "/getUpdates"
			resp, err := client.PostForm(u, values)
			if err != nil {
				log.Printf("telegram getUpdates error: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var gu struct {
				Ok     bool `json:"ok"`
				Result []struct {
					UpdateID int64 `json:"update_id"`
					Message  *struct {
						MessageID int64 `json:"message_id"`
						From      *struct {
							ID int64 `json:"id"`
						} `json:"from"`
						Chat struct {
							ID int64 `json:"id"`
						} `json:"chat"`
						Text string `json:"text"`
					} `json:"message"`
				} `json:"result"`
			}
			if err := json.Unmarshal(body, &gu); err != nil {
				log.Printf("telegram: invalid getUpdates response: %v", err)
				continue
			}
			for _, upd := range gu.Result {
				if upd.UpdateID >= offset {
					offset = upd.UpdateID + 1
				}
				if upd.Message == nil {
					continue
				}
				m := upd.Message
				fromID := ""
				if m.From != nil {
					fromID = strconv.FormatInt(m.From.ID, 10)
				}
				// Enforce allowFrom: if the list is non-empty, reject unknown senders.
				if len(allowed) > 0 {
					if _, ok := allowed[fromID]; !ok {
						log.Printf("telegram: dropping message from unauthorized user %s", fromID)
						continue
					}
				}
				chatID := strconv.FormatInt(m.Chat.ID, 10)
				hub.In <- chat.Inbound{
					Channel:   "telegram",
					SenderID:  fromID,
					ChatID:    chatID,
					Content:   m.Text,
					Timestamp: time.Now(),
				}
			}
		}
	}()

	// Subscribe to the outbound queue before launching the goroutine so the
	// registration is visible to the hub router from the moment this function returns.
	outCh := hub.Subscribe("telegram")

	// outbound sender goroutine
	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		for {
			select {
			case <-ctx.Done():
				log.Println("telegram: stopping outbound sender")
				return
			case out := <-outCh:
				u := base + "/sendMessage"
				v := url.Values{}
				v.Set("chat_id", out.ChatID)
				v.Set("text", out.Content)
				resp, err := client.PostForm(u, v)
				if err != nil {
					log.Printf("telegram sendMessage error: %v", err)
					continue
				}
				io.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}
	}()

	return nil
}
