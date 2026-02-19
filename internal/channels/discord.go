package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/local/picobot/internal/chat"
)

// Discord Gateway opcodes
const (
	gatewayOpDispatch       = 0
	gatewayOpHeartbeat      = 1
	gatewayOpIdentify       = 2
	gatewayOpResume         = 6
	gatewayOpReconnect      = 7
	gatewayOpInvalidSession = 9
	gatewayOpHello          = 10
	gatewayOpHeartbeatAck   = 11
)

// Discord Gateway intents
const (
	intentGuilds         = 1 << 0
	intentGuildMessages  = 1 << 9
	intentDirectMessages = 1 << 12
	intentMessageContent = 1 << 15
)

// DiscordAPIBase is the base URL for Discord REST API
const DiscordAPIBase = "https://discord.com/api/v10"

// DiscordGatewayURL is the WebSocket gateway URL
const DiscordGatewayURL = "wss://gateway.discord.gg/?v=10&encoding=json"

// StartDiscord starts a Discord bot using the Gateway (WebSocket) for receiving
// and REST API for sending messages.
// allowFrom is a list of Discord user IDs permitted to interact with the bot.
// If empty, ALL users are allowed (open mode).
func StartDiscord(ctx context.Context, hub *chat.Hub, token string, allowFrom []string) error {
	if token == "" {
		return fmt.Errorf("discord token not provided")
	}

	// Build a fast lookup set for allowed user IDs.
	allowed := make(map[string]struct{}, len(allowFrom))
	for _, id := range allowFrom {
		allowed[id] = struct{}{}
	}

	// Create and start the Discord client
	client := newDiscordClient(token, DiscordGatewayURL, hub, allowed)

	// Start outbound handler
	outbound := newDiscordOutboundHandler(token)
	go outbound.run(ctx, hub)

	// Start connection in goroutine
	go client.connect(ctx)

	return nil
}

// discordClient handles the Discord Gateway connection
type discordClient struct {
	token       string
	gatewayURL  string
	hub         *chat.Hub
	allowed     map[string]struct{}
	botID       string

	mu           sync.Mutex
	conn         *websocket.Conn
	sessionID    string
	sequence     *int64
	heartbeat    time.Duration
	lastAck      time.Time
	shouldResume bool
}

func newDiscordClient(token, gatewayURL string, hub *chat.Hub, allowed map[string]struct{}) *discordClient {
	return &discordClient{
		token:      token,
		gatewayURL: gatewayURL,
		hub:        hub,
		allowed:    allowed,
	}
}

// connect establishes and maintains the Gateway connection
func (c *discordClient) connect(ctx context.Context) {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Println("discord: stopping connection loop")
			return
		default:
		}

		err := c.establishConnection(ctx)
		if err != nil {
			log.Printf("discord: connection error: %v", err)
		}

		// Exponential backoff on reconnect
		time.Sleep(backoff)
		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}

		// Reset backoff on successful connection
		c.mu.Lock()
		if c.sessionID != "" {
			backoff = 1 * time.Second
		}
		c.mu.Unlock()
	}
}

// establishConnection creates a single Gateway connection
func (c *discordClient) establishConnection(ctx context.Context) error {
	// Connect to Gateway
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(c.gatewayURL, nil)
	if err != nil {
		return fmt.Errorf("gateway dial error: %w", err)
	}
	defer conn.Close()

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// Set read limit to 1MB (Discord can send large messages)
	conn.SetReadLimit(1 << 20)

	// Wait for Hello
	_, data, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("hello read error: %w", err)
	}

	var hello struct {
		Op int `json:"op"`
		D  struct {
			HeartbeatInterval int `json:"heartbeat_interval"`
		} `json:"d"`
	}
	if err := json.Unmarshal(data, &hello); err != nil {
		return fmt.Errorf("hello parse error: %w", err)
	}
	if hello.Op != gatewayOpHello {
		return fmt.Errorf("expected hello op, got %d", hello.Op)
	}

	c.heartbeat = time.Duration(hello.D.HeartbeatInterval) * time.Millisecond
	c.lastAck = time.Now()
	log.Printf("discord: connected, heartbeat interval: %v", c.heartbeat)

	// Send Identify or Resume
	c.mu.Lock()
	sessionID := c.sessionID
	sequence := c.sequence
	c.mu.Unlock()

	if sessionID != "" && sequence != nil {
		// Resume
		resume := map[string]interface{}{
			"op": gatewayOpResume,
			"d": map[string]interface{}{
				"token":      c.token,
				"session_id": sessionID,
				"seq":        sequence,
			},
		}
		if err := conn.WriteJSON(resume); err != nil {
			return fmt.Errorf("resume send error: %w", err)
		}
		log.Println("discord: resuming session")
	} else {
		// Identify
		identify := map[string]interface{}{
			"op": gatewayOpIdentify,
			"d": map[string]interface{}{
				"token": c.token,
				"intents": intentGuilds | intentGuildMessages | intentDirectMessages | intentMessageContent,
				"properties": map[string]string{
					"os":      "linux",
					"browser": "picobot",
					"device":  "picobot",
				},
			},
		}
		if err := conn.WriteJSON(identify); err != nil {
			return fmt.Errorf("identify send error: %w", err)
		}
		log.Println("discord: identified")
	}

	// Start heartbeat ticker
	heartbeatTicker := time.NewTicker(c.heartbeat)
	defer heartbeatTicker.Stop()

	// Create done channel for this connection
	done := make(chan struct{})

	// Read goroutine
	readCh := make(chan []byte, 100)
	go func() {
		defer close(done)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("discord: read error: %v", err)
				}
				return
			}
			select {
			case readCh <- data:
			default:
				// Channel full, drop message
			}
		}
	}()

	// Event loop
	for {
		select {
		case <-ctx.Done():
			log.Println("discord: stopping event loop")
			// Send close frame
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return nil

		case <-heartbeatTicker.C:
			c.mu.Lock()
			seq := c.sequence
			c.mu.Unlock()

			// Check for missed heartbeat acks (zombie connection detection)
			if time.Since(c.lastAck) > c.heartbeat*3 {
				log.Println("discord: missed heartbeat acks, reconnecting")
				return nil
			}

			heartbeat := map[string]interface{}{
				"op": gatewayOpHeartbeat,
				"d":  seq,
			}
			if err := conn.WriteJSON(heartbeat); err != nil {
				log.Printf("discord: heartbeat error: %v", err)
				return err
			}

		case data := <-readCh:
			var event gatewayEvent
			if err := json.Unmarshal(data, &event); err != nil {
				log.Printf("discord: event parse error: %v", err)
				continue
			}

			// Update sequence
			if event.Sequence != 0 {
				c.mu.Lock()
				c.sequence = &event.Sequence
				c.mu.Unlock()
			}

			switch event.Op {
			case gatewayOpDispatch:
				c.handleDispatch(event.Type, event.Data)

			case gatewayOpHeartbeatAck:
				c.lastAck = time.Now()

			case gatewayOpReconnect:
				log.Println("discord: server requested reconnect")
				return nil // Will reconnect

			case gatewayOpInvalidSession:
				var resumable bool
				json.Unmarshal(event.Data, &resumable)
				if !resumable {
					log.Println("discord: invalid session, will re-identify")
					c.mu.Lock()
					c.sessionID = ""
					c.sequence = nil
					c.mu.Unlock()
				}
				return nil // Will reconnect
			}

		case <-done:
			log.Println("discord: connection closed")
			return nil
		}
	}
}

// gatewayEvent represents a Gateway event
type gatewayEvent struct {
	Op       int             `json:"op"`
	Type     string          `json:"t"`
	Data     json.RawMessage `json:"d"`
	Sequence int64           `json:"s"`
}

// handleDispatch handles Gateway dispatch events
func (c *discordClient) handleDispatch(eventType string, data json.RawMessage) {
	switch eventType {
	case "READY":
		var ready struct {
			SessionID string `json:"session_id"`
			User      struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"user"`
		}
		if err := json.Unmarshal(data, &ready); err != nil {
			log.Printf("discord: ready parse error: %v", err)
			return
		}
		c.mu.Lock()
		c.sessionID = ready.SessionID
		c.botID = ready.User.ID
		c.mu.Unlock()
		log.Printf("discord: ready as %s (%s)", ready.User.Username, ready.User.ID)

	case "MESSAGE_CREATE":
		var msg discordMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("discord: message parse error: %v", err)
			return
		}

		// Ignore own messages
		if msg.Author.Bot {
			return
		}

		// Check permissions
		if len(c.allowed) > 0 {
			if _, ok := c.allowed[msg.Author.ID]; !ok {
				log.Printf("discord: dropping message from unauthorized user %s", msg.Author.ID)
				return
			}
		}

		// Determine if this is a DM
		isDM := msg.GuildID == ""

		// For guild messages, check if bot is mentioned or it's a reply
		content := msg.Content
		if !isDM {
			// Check if bot is mentioned
			isMentioned := false
			c.mu.Lock()
			botID := c.botID
			c.mu.Unlock()

			for _, mention := range msg.Mentions {
				if mention.ID == botID {
					isMentioned = true
					break
				}
			}

			// Also check for mention in content
			if strings.Contains(content, "<@"+botID+">") || strings.Contains(content, "<@!"+botID+">") {
				isMentioned = true
			}

			// In guilds, only respond when mentioned or it's a reply to the bot
			if !isMentioned && msg.MessageReference == nil {
				return
			}

			// Remove the mention from content
			content = strings.ReplaceAll(content, "<@"+botID+">", "")
			content = strings.ReplaceAll(content, "<@!"+botID+">", "")
		}

		// Clean content
		content = cleanDiscordContent(content)
		if content == "" {
			return
		}

		log.Printf("discord: received message from %s (%s) in %s: %s",
			msg.Author.Username, msg.Author.ID,
			msg.ChannelID,
			truncate(content, 50))

		// Send to hub
		c.hub.In <- chat.Inbound{
			Channel:   "discord",
			SenderID:  msg.Author.ID,
			ChatID:    msg.ChannelID,
			Content:   content,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"username":   msg.Author.Username,
				"guild_id":   msg.GuildID,
				"channel_id": msg.ChannelID,
				"message_id": msg.ID,
				"is_dm":      isDM,
			},
		}

	case "RESUMED":
		log.Println("discord: session resumed")
	}
}

// discordMessage represents a Discord message
type discordMessage struct {
	ID              string `json:"id"`
	ChannelID       string `json:"channel_id"`
	GuildID         string `json:"guild_id,omitempty"`
	Content         string `json:"content"`
	Author          struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Bot      bool   `json:"bot,omitempty"`
	} `json:"author"`
	Mentions         []struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"mentions"`
	MessageReference *struct {
		MessageID string `json:"message_id"`
		ChannelID string `json:"channel_id"`
		GuildID   string `json:"guild_id,omitempty"`
	} `json:"message_reference,omitempty"`
}

// cleanDiscordContent removes Discord-specific formatting from content
func cleanDiscordContent(content string) string {
	// Remove user mentions <@123456789> or <@!123456789>
	for {
		start := strings.Index(content, "<@")
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], ">")
		if end == -1 {
			break
		}
		content = content[:start] + content[start+end+1:]
	}

	// Remove role mentions <@&123456789>
	for {
		start := strings.Index(content, "<@&")
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], ">")
		if end == -1 {
			break
		}
		content = content[:start] + content[start+end+1:]
	}

	// Remove channel mentions <#123456789> (keep channel name for context)
	for {
		start := strings.Index(content, "<#")
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], ">")
		if end == -1 {
			break
		}
		content = content[:start] + content[start+end+1:]
	}

	// Remove emoji <:name:id> and <a:name:id>
	for {
		idx := strings.Index(content, "<")
		if idx == -1 {
			break
		}
		rest := content[idx:]
		if !strings.HasPrefix(rest, "<:") && !strings.HasPrefix(rest, "<a:") {
			break
		}
		end := strings.Index(rest, ">")
		if end == -1 {
			break
		}
		content = content[:idx] + content[idx+end+1:]
	}

	return strings.TrimSpace(content)
}

// truncate truncates a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// discordOutboundHandler handles outbound messages for Discord
type discordOutboundHandler struct {
	token  string
	client *http.Client
}

func newDiscordOutboundHandler(token string) *discordOutboundHandler {
	return &discordOutboundHandler{
		token:  token,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *discordOutboundHandler) run(ctx context.Context, hub *chat.Hub) {
	for {
		select {
		case <-ctx.Done():
			log.Println("discord: stopping outbound handler")
			return
		case out := <-hub.Out:
			if out.Channel != "discord" {
				continue
			}
			if err := h.sendMessage(out.ChatID, out.Content); err != nil {
				log.Printf("discord: send message error: %v", err)
			}
		}
	}
}

// sendMessage sends a message to a Discord channel via REST API
func (h *discordOutboundHandler) sendMessage(channelID, content string) error {
	url := fmt.Sprintf("%s/channels/%s/messages", DiscordAPIBase, channelID)

	payload := map[string]interface{}{
		"content": content,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bot "+h.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord API error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// DiscordRestOnly provides a REST-only Discord client for sending messages
// This is useful when you only need to send messages (e.g., notifications)
// and don't need to receive them via Gateway
type DiscordRestOnly struct {
	token  string
	client *http.Client
}

// NewDiscordRestOnly creates a REST-only Discord client
func NewDiscordRestOnly(token string) *DiscordRestOnly {
	return &DiscordRestOnly{
		token:  token,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// SendMessage sends a message to a Discord channel
func (d *DiscordRestOnly) SendMessage(channelID, content string) error {
	url := fmt.Sprintf("%s/channels/%s/messages", DiscordAPIBase, channelID)

	payload := map[string]interface{}{
		"content": content,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bot "+d.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord API error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// GetChannel gets information about a channel
func (d *DiscordRestOnly) GetChannel(channelID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/channels/%s", DiscordAPIBase, channelID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bot "+d.token)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("discord API error: %s", resp.Status)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetUser gets information about a user
func (d *DiscordRestOnly) GetUser(userID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/users/%s", DiscordAPIBase, userID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bot "+d.token)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("discord API error: %s", resp.Status)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// Helper functions for Discord mentions

// ParseChannelMention extracts a channel ID from a channel mention <#123456789>
func ParseChannelMention(mention string) string {
	if !strings.HasPrefix(mention, "<#") || !strings.HasSuffix(mention, ">") {
		return ""
	}
	return mention[2 : len(mention)-1]
}

// ParseUserMention extracts a user ID from a user mention <@123456789> or <@!123456789>
func ParseUserMention(mention string) string {
	if !strings.HasPrefix(mention, "<@") || !strings.HasSuffix(mention, ">") {
		return ""
	}
	inner := mention[2 : len(mention)-1]
	// Remove ! prefix if present (nickname mention)
	if strings.HasPrefix(inner, "!") {
		return inner[1:]
	}
	return inner
}

// FormatUserMention creates a user mention from a user ID
func FormatUserMention(userID string) string {
	return "<@" + userID + ">"
}

// FormatChannelMention creates a channel mention from a channel ID
func FormatChannelMention(channelID string) string {
	return "<#" + channelID + ">"
}

// String helpers for Discord IDs
func discordIDToString(id int64) string {
	return strconv.FormatInt(id, 10)
}

func discordIDFromString(s string) int64 {
	id, _ := strconv.ParseInt(s, 10, 64)
	return id
}