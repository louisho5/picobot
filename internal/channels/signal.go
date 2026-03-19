package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/local/picobot/internal/chat"
)

// signalSender is the subset of outbound operations used by the Signal channel.
// It exists to enable testing without a live secured-signal-api instance.
type signalSender interface {
	Send(ctx context.Context, number, recipient, message string) error
}

// StartSignal starts a Signal channel that communicates through a
// secured-signal-api proxy (https://github.com/codeshelldev/secured-signal-api).
//
// Inbound messages are received via a WebSocket connection to /v1/receive/<number>
// on the underlying signal-cli-rest-api, which allows non-destructive reads so
// other consumers of the same Signal number are not affected.
//
// Outbound messages are sent via HTTP POST to /v2/send on the proxy with Bearer
// token authentication.
//
// allowFrom restricts which phone numbers may send messages; empty means allow all.
func StartSignal(ctx context.Context, hub *chat.Hub, apiURL, apiToken, number string, allowFrom []string) error {
	if apiURL == "" {
		return fmt.Errorf("signal API URL not provided")
	}
	if number == "" {
		return fmt.Errorf("signal phone number not provided")
	}

	sender := &httpSignalSender{
		apiURL:   strings.TrimRight(apiURL, "/"),
		apiToken: apiToken,
		client:   &http.Client{Timeout: 15 * time.Second},
	}

	// Build the WebSocket URL by converting the HTTP(S) scheme.
	wsURL, err := buildSignalWSURL(apiURL, number)
	if err != nil {
		return fmt.Errorf("signal: invalid API URL: %w", err)
	}

	return startSignalWithSender(ctx, hub, sender, wsURL, apiToken, number, allowFrom)
}

// startSignalWithSender is the testable core that accepts a signalSender interface
// and a pre-built WebSocket URL so tests can inject mocks.
func startSignalWithSender(ctx context.Context, hub *chat.Hub, sender signalSender, wsURL, apiToken, number string, allowFrom []string) error {
	// Build a fast lookup set for allowed phone numbers.
	allowed := make(map[string]struct{}, len(allowFrom))
	for _, id := range allowFrom {
		allowed[id] = struct{}{}
	}

	// inbound WebSocket goroutine
	go func() {
		header := http.Header{}
		if apiToken != "" {
			header.Set("Authorization", "Bearer "+apiToken)
		}

		backoff := 1 * time.Second
		maxBackoff := 30 * time.Second

		for {
			select {
			case <-ctx.Done():
				log.Println("signal: stopping inbound listener")
				return
			default:
			}

			log.Printf("signal: connecting to %s", wsURL)
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
			if err != nil {
				log.Printf("signal: websocket dial error: %v (retrying in %s)", err, backoff)
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				backoff = backoff * 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}
			log.Println("signal: websocket connected")
			backoff = 1 * time.Second // reset on successful connection

			readMessages(ctx, conn, hub, number, allowed)

			if err := conn.Close(); err != nil {
				log.Printf("signal: websocket close error: %v", err)
			}
			log.Println("signal: websocket disconnected, reconnecting...")
		}
	}()

	// Subscribe to the outbound queue before launching the goroutine so the
	// registration is visible to the hub router from the moment this function returns.
	outCh := hub.Subscribe("signal")

	// outbound sender goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("signal: stopping outbound sender")
				return
			case out := <-outCh:
				for _, chunk := range splitMessage(out.Content, 2000) {
					if err := sender.Send(ctx, number, out.ChatID, chunk); err != nil {
						log.Printf("signal: send error: %v", err)
					}
				}
			}
		}
	}()

	return nil
}

// readMessages reads from an established WebSocket connection and pushes
// parsed Signal messages into the hub. It returns when the connection drops
// or the context is cancelled.
func readMessages(ctx context.Context, conn *websocket.Conn, hub *chat.Hub, number string, allowed map[string]struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Println("signal: websocket closed normally")
			} else {
				log.Printf("signal: websocket read error: %v", err)
			}
			return
		}

		var envelope signalEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			log.Printf("signal: invalid message JSON: %v", err)
			continue
		}

		env := envelope.Envelope
		if env == nil {
			continue
		}

		// Only process data messages (ignore typing indicators, receipts, etc.)
		if env.DataMessage == nil {
			continue
		}

		message := env.DataMessage.Message
		if message == "" {
			continue
		}

		senderID := env.SourceNumber
		if senderID == "" {
			senderID = env.Source
		}

		// Skip messages sent by our own number (echo suppression).
		if senderID == number {
			continue
		}

		// Enforce allowFrom: if the list is non-empty, reject unknown senders.
		if len(allowed) > 0 {
			if _, ok := allowed[senderID]; !ok {
				log.Printf("signal: dropping message from unauthorized sender %s", senderID)
				continue
			}
		}

		// Use group ID as chat ID for group messages, sender number for DMs.
		chatID := senderID
		if env.DataMessage.GroupInfo != nil && env.DataMessage.GroupInfo.GroupID != "" {
			chatID = env.DataMessage.GroupInfo.GroupID
		}

		hub.In <- chat.Inbound{
			Channel:   "signal",
			SenderID:  senderID,
			ChatID:    chatID,
			Content:   message,
			Timestamp: time.Now(),
		}
	}
}

// signalEnvelope represents the JSON structure of a message received from
// the signal-cli-rest-api WebSocket endpoint.
type signalEnvelope struct {
	Envelope *signalEnvelopeData `json:"envelope"`
}

type signalEnvelopeData struct {
	Source       string             `json:"source"`
	SourceNumber string             `json:"sourceNumber"`
	SourceName   string             `json:"sourceName"`
	Timestamp    int64              `json:"timestamp"`
	DataMessage  *signalDataMessage `json:"dataMessage"`
}

type signalDataMessage struct {
	Message   string           `json:"message"`
	Timestamp int64            `json:"timestamp"`
	GroupInfo *signalGroupInfo `json:"groupInfo"`
}

type signalGroupInfo struct {
	GroupID string `json:"groupId"`
}

// httpSignalSender sends messages via the secured-signal-api HTTP endpoint.
type httpSignalSender struct {
	apiURL   string
	apiToken string
	client   *http.Client
}

func (s *httpSignalSender) Send(ctx context.Context, number, recipient, message string) error {
	payload := map[string]interface{}{
		"message":    message,
		"number":     number,
		"recipients": []string{recipient},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("signal: marshal error: %w", err)
	}

	u := s.apiURL + "/v2/send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("signal: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiToken)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("signal: send error: %w", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("signal: send returned status %d", resp.StatusCode)
	}

	return nil
}

// buildSignalWSURL converts an HTTP(S) base URL into a WebSocket URL for the
// signal-cli-rest-api receive endpoint.
func buildSignalWSURL(apiURL, number string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(apiURL, "/"))
	if err != nil {
		return "", err
	}

	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	default:
		parsed.Scheme = "ws"
	}

	// signal-cli-rest-api expects the '+' in phone numbers to be
	// percent-encoded as %2B. Go's PathEscape considers '+' valid in
	// paths so we replace it manually and set RawPath to preserve it.
	encodedNumber := strings.ReplaceAll(url.PathEscape(number), "+", "%2B")
	parsed.Path = "/v1/receive/" + number
	parsed.RawPath = "/v1/receive/" + encodedNumber
	return parsed.String(), nil
}
