package channels

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/local/picobot/internal/chat"
)

// discordSender is the subset of *discordgo.Session used for outbound operations.
// It exists to enable testing without a live Discord WebSocket connection.
type discordSender interface {
	ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelTyping(channelID string, options ...discordgo.RequestOption) error
}

// StartDiscord starts a Discord bot using the discordgo library.
// allowFrom restricts which Discord user IDs may send messages; empty means allow all.
func StartDiscord(ctx context.Context, hub *chat.Hub, token string, allowFrom []string) error {
	if token == "" {
		return fmt.Errorf("discord token not provided")
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("failed to create discord session: %w", err)
	}

	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent

	if err := session.Open(); err != nil {
		return fmt.Errorf("failed to open discord connection: %w", err)
	}

	botUser, err := session.User("@me")
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to get bot user: %w", err)
	}
	log.Printf("discord: connected as %s (%s)", botUser.Username, botUser.ID)

	client := newDiscordClient(ctx, session, hub, botUser.ID, allowFrom)
	session.AddHandler(client.handleMessage)
	go client.runOutbound()
	go func() {
		<-ctx.Done()
		log.Println("discord: shutting down")
		client.stopAllTyping()
		session.Close()
	}()

	return nil
}

// discordClient handles Discord messaging using a discordSender.
type discordClient struct {
	sender     discordSender
	hub        *chat.Hub
	outCh      <-chan chat.Outbound
	botID      string
	allowed    map[string]struct{}
	ctx        context.Context
	typingMu   sync.Mutex
	typingStop map[string]chan struct{}
}

// newDiscordClient constructs a discordClient and registers it as the hub's
// "discord" outbound subscriber. Inject a mock discordSender for tests.
func newDiscordClient(ctx context.Context, sender discordSender, hub *chat.Hub, botID string, allowFrom []string) *discordClient {
	allowed := make(map[string]struct{}, len(allowFrom))
	for _, id := range allowFrom {
		allowed[id] = struct{}{}
	}
	return &discordClient{
		sender:     sender,
		hub:        hub,
		outCh:      hub.Subscribe("discord"),
		botID:      botID,
		allowed:    allowed,
		ctx:        ctx,
		typingStop: make(map[string]chan struct{}),
	}
}

// handleMessage is the discordgo MessageCreate event handler.
// The *discordgo.Session parameter is intentionally ignored; all bot-identity
// information is held in c.botID so that we can call this in tests without a
// live session.
func (c *discordClient) handleMessage(_ *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.Author.ID == c.botID {
		return
	}

	// Enforce allowlist when one is configured.
	if len(c.allowed) > 0 {
		if _, ok := c.allowed[m.Author.ID]; !ok {
			log.Printf("discord: dropped message from unauthorised user %s (%s)", m.Author.Username, m.Author.ID)
			return
		}
	}

	isDM := m.GuildID == ""

	// In guild channels only respond when the bot is @-mentioned.
	if !isDM {
		mentioned := false
		for _, u := range m.Mentions {
			if u.ID == c.botID {
				mentioned = true
				break
			}
		}
		if !mentioned {
			return
		}
	}

	// Strip bot @-mentions from the message text.
	content := m.Content
	for _, u := range m.Mentions {
		if u.ID == c.botID {
			content = strings.ReplaceAll(content, "<@"+u.ID+">", "")
			content = strings.ReplaceAll(content, "<@!"+u.ID+">", "")
		}
	}
	content = strings.TrimSpace(content)

	// Append file attachment URLs as inline references.
	for _, att := range m.Attachments {
		content += fmt.Sprintf("\n[attachment: %s]", att.URL)
	}

	if content == "" {
		return
	}

	senderName := senderDisplayName(m.Author)
	log.Printf("discord: message from %s (%s) in %s: %s", senderName, m.Author.ID, m.ChannelID, truncate(content, 50))

	c.startTyping(m.ChannelID)

	c.hub.In <- chat.Inbound{
		Channel:   "discord",
		SenderID:  m.Author.ID,
		ChatID:    m.ChannelID,
		Content:   content,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"username":   senderName,
			"guild_id":   m.GuildID,
			"channel_id": m.ChannelID,
			"is_dm":      isDM,
		},
	}
}

// runOutbound reads replies from the hub's discord subscription and sends them.
func (c *discordClient) runOutbound() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case out := <-c.outCh:
			c.stopTyping(out.ChatID)
			for _, chunk := range splitMessage(out.Content, 2000) {
				if _, err := c.sender.ChannelMessageSend(out.ChatID, chunk); err != nil {
					log.Printf("discord: send error: %v", err)
				}
			}
		}
	}
}

// startTyping begins (or resets) a continuous typing indicator for a channel.
// It stops automatically after 5 minutes or when stopTyping / stopAllTyping is called.
func (c *discordClient) startTyping(channelID string) {
	c.typingMu.Lock()
	if stop, ok := c.typingStop[channelID]; ok {
		close(stop)
	}
	stop := make(chan struct{})
	c.typingStop[channelID] = stop
	c.typingMu.Unlock()

	go func() {
		c.sender.ChannelTyping(channelID)

		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		timeout := time.NewTimer(5 * time.Minute)
		defer timeout.Stop()

		for {
			select {
			case <-stop:
				return
			case <-timeout.C:
				return
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				c.sender.ChannelTyping(channelID)
			}
		}
	}()
}

// stopTyping cancels the typing indicator for the given channel.
func (c *discordClient) stopTyping(channelID string) {
	c.typingMu.Lock()
	defer c.typingMu.Unlock()
	if stop, ok := c.typingStop[channelID]; ok {
		close(stop)
		delete(c.typingStop, channelID)
	}
}

// stopAllTyping cancels all active typing indicators.
func (c *discordClient) stopAllTyping() {
	c.typingMu.Lock()
	defer c.typingMu.Unlock()
	for _, stop := range c.typingStop {
		close(stop)
	}
	c.typingStop = make(map[string]chan struct{})
}

// senderDisplayName returns "Username" for new-style accounts or
// "Username#Discriminator" for legacy accounts.
func senderDisplayName(u *discordgo.User) string {
	if u.Discriminator != "" && u.Discriminator != "0" {
		return u.Username + "#" + u.Discriminator
	}
	return u.Username
}

// truncate returns s shortened to maxLen bytes with "..." appended when truncated.
// Used only for log messages.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// splitMessage splits content into chunks whose rune count does not exceed maxLen.
// It prefers splitting at newlines, then spaces, to avoid mid-word cuts.
func splitMessage(content string, maxLen int) []string {
	runes := []rune(content)
	if len(runes) <= maxLen {
		return []string{content}
	}

	var chunks []string
	for len(runes) > maxLen {
		idx := maxLen
		// Prefer a newline boundary.
		for i := maxLen - 1; i > 0; i-- {
			if runes[i] == '\n' {
				idx = i + 1
				break
			}
		}
		// Fall back to a space boundary.
		if idx == maxLen {
			for i := maxLen - 1; i > 0; i-- {
				if runes[i] == ' ' {
					idx = i + 1
					break
				}
			}
		}
		chunks = append(chunks, string(runes[:idx]))
		runes = runes[idx:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}
