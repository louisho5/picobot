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

// StartDiscord starts a Discord bot using discordgo library.
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

	// Create discordgo session
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("failed to create discord session: %w", err)
	}

	// Create client
	client := &discordClient{
		session:   session,
		hub:       hub,
		allowed:   allowed,
		ctx:       ctx,
		typingMu:  sync.Mutex{},
		typingStop: make(map[string]chan struct{}),
	}

	// Add message handler
	session.AddHandler(client.handleMessage)

	// Set intents
	session.Identify.Intents = discordgo.IntentsGuilds | 
		discordgo.IntentsGuildMessages | 
		discordgo.IntentsDirectMessages | 
		discordgo.IntentsMessageContent

	// Open connection
	if err := session.Open(); err != nil {
		return fmt.Errorf("failed to open discord session: %w", err)
	}

	// Get bot user info
	botUser, err := session.User("@me")
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to get bot user: %w", err)
	}
	log.Printf("discord: connected as %s (%s)", botUser.Username, botUser.ID)

	// Start outbound handler
	go client.runOutbound(ctx)

	// Wait for context cancellation
	go func() {
		<-ctx.Done()
		log.Println("discord: shutting down")
		client.stopAllTyping()
		session.Close()
	}()

	return nil
}

// discordClient handles the Discord connection using discordgo
type discordClient struct {
	session    *discordgo.Session
	hub        *chat.Hub
	allowed    map[string]struct{}
	ctx        context.Context
	typingMu   sync.Mutex
	typingStop map[string]chan struct{}
}

// handleMessage handles incoming Discord messages
func (c *discordClient) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}

	// Skip own messages
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check allowlist
	if len(c.allowed) > 0 {
		if _, ok := c.allowed[m.Author.ID]; !ok {
			log.Printf("discord: message from %s (%s) rejected by allowlist", m.Author.Username, m.Author.ID)
			return
		}
	}

	// Determine if this is a DM or guild message
	isDM := m.GuildID == ""
	channelID := m.ChannelID

	// For guild messages, only respond when mentioned
	if !isDM {
		mentioned := false
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				mentioned = true
				break
			}
		}
		if !mentioned {
			return
		}
	}

	// Get sender name
	senderName := m.Author.Username
	if m.Author.Discriminator != "" && m.Author.Discriminator != "0" {
		senderName += "#" + m.Author.Discriminator
	}

	// Clean content (remove bot mention)
	content := m.Content
	for _, mention := range m.Mentions {
		if mention.ID == s.State.User.ID {
			content = strings.Replace(content, "<@"+mention.ID+">", "", -1)
			content = strings.Replace(content, "<@!"+mention.ID+">", "", -1)
		}
	}
	content = strings.TrimSpace(content)

	// Handle attachments
	if len(m.Attachments) > 0 {
		for _, att := range m.Attachments {
			content += fmt.Sprintf("\n[attachment: %s]", att.URL)
		}
	}

	if content == "" {
		return
	}

	// Start typing indicator
	c.startTyping(channelID)

	log.Printf("discord: received message from %s (%s) in %s: %s", 
		senderName, m.Author.ID, channelID, truncate(content, 50))

	// Send to hub
	c.hub.In <- chat.Inbound{
		Channel:   "discord",
		SenderID:  m.Author.ID,
		ChatID:    channelID,
		Content:   content,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"username":   senderName,
			"guild_id":   m.GuildID,
			"channel_id": channelID,
			"is_dm":      isDM,
		},
	}
}

// runOutbound handles outbound messages
func (c *discordClient) runOutbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case out := <-c.hub.Out:
			if out.Channel != "discord" {
				continue
			}
			
			c.stopTyping(out.ChatID)
			
			// Split message if too long (Discord limit: 2000 chars)
			chunks := splitMessage(out.Content, 2000)
			for _, chunk := range chunks {
				if _, err := c.session.ChannelMessageSend(out.ChatID, chunk); err != nil {
					log.Printf("discord: send message error: %v", err)
				}
			}
		}
	}
}

// startTyping starts a continuous typing indicator
func (c *discordClient) startTyping(channelID string) {
	c.typingMu.Lock()
	// Stop existing typing for this channel
	if stop, ok := c.typingStop[channelID]; ok {
		close(stop)
	}
	stop := make(chan struct{})
	c.typingStop[channelID] = stop
	c.typingMu.Unlock()

	go func() {
		// Initial typing trigger
		c.session.ChannelTyping(channelID)
		
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		timeout := time.After(5 * time.Minute)
		
		for {
			select {
			case <-stop:
				return
			case <-timeout:
				return
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				c.session.ChannelTyping(channelID)
			}
		}
	}()
}

// stopTyping stops the typing indicator for a channel
func (c *discordClient) stopTyping(channelID string) {
	c.typingMu.Lock()
	defer c.typingMu.Unlock()
	if stop, ok := c.typingStop[channelID]; ok {
		close(stop)
		delete(c.typingStop, channelID)
	}
}

// stopAllTyping stops all typing indicators
func (c *discordClient) stopAllTyping() {
	c.typingMu.Lock()
	defer c.typingMu.Unlock()
	for _, stop := range c.typingStop {
		close(stop)
	}
	c.typingStop = make(map[string]chan struct{})
}

// Helper functions

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func splitMessage(content string, maxLen int) []string {
	if len(content) <= maxLen {
		return []string{content}
	}

	var chunks []string
	runes := []rune(content)
	
	for len(runes) > 0 {
		if len(runes) <= maxLen {
			chunks = append(chunks, string(runes))
			break
		}
		
		// Try to split at newline
		splitIdx := maxLen
		for i := maxLen - 1; i >= 0; i-- {
			if runes[i] == '\n' {
				splitIdx = i + 1
				break
			}
		}
		
		// If no newline, split at space
		if splitIdx == maxLen {
			for i := maxLen - 1; i >= 0; i-- {
				if runes[i] == ' ' {
					splitIdx = i + 1
					break
				}
			}
		}
		
		chunks = append(chunks, string(runes[:splitIdx]))
		runes = runes[splitIdx:]
	}
	
	return chunks
}
