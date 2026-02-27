//go:build !lite

package channels

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	qrterminal "github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite"

	"github.com/local/picobot/internal/chat"
)

// whatsappSender is the subset of *whatsmeow.Client used for outbound operations.
// It exists to enable testing without a live WhatsApp WebSocket connection.
type whatsappSender interface {
	SendText(ctx context.Context, to types.JID, text string) error
	SendChatPresence(ctx context.Context, chat types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error
	MarkRead(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID) error
	SendPresence(ctx context.Context, state types.Presence) error
}

// realWhatsAppSender wraps *whatsmeow.Client to implement whatsappSender.
type realWhatsAppSender struct {
	c *whatsmeow.Client
}

func (r *realWhatsAppSender) SendText(ctx context.Context, to types.JID, text string) error {
	_, err := r.c.SendMessage(ctx, to, &waProto.Message{Conversation: &text})
	return err
}

func (r *realWhatsAppSender) SendChatPresence(ctx context.Context, chat types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error {
	return r.c.SendChatPresence(ctx, chat, state, media)
}

func (r *realWhatsAppSender) MarkRead(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID) error {
	return r.c.MarkRead(ctx, ids, timestamp, chat, sender)
}

func (r *realWhatsAppSender) SendPresence(ctx context.Context, state types.Presence) error {
	return r.c.SendPresence(ctx, state)
}

// whatsappLogger adapts the whatsmeow logger to use Go's standard logger.
type whatsappLogger struct{}

func (l whatsappLogger) Errorf(msg string, args ...interface{}) {
	log.Printf("[whatsapp] ERROR: "+msg, args...)
}
func (l whatsappLogger) Warnf(msg string, args ...interface{}) {
	log.Printf("[whatsapp] WARN: "+msg, args...)
}
func (l whatsappLogger) Infof(msg string, args ...interface{}) {
	log.Printf("[whatsapp] INFO: "+msg, args...)
}
func (l whatsappLogger) Debugf(msg string, args ...interface{}) {}
func (l whatsappLogger) Sub(module string) waLog.Logger         { return l }

// quietLogger only logs errors, used during onboarding to keep output clean.
type quietLogger struct{}

func (l quietLogger) Errorf(msg string, args ...interface{}) {
	log.Printf("[whatsapp] ERROR: "+msg, args...)
}
func (l quietLogger) Warnf(msg string, args ...interface{})  {}
func (l quietLogger) Infof(msg string, args ...interface{})  {}
func (l quietLogger) Debugf(msg string, args ...interface{}) {}
func (l quietLogger) Sub(module string) waLog.Logger         { return l }

// StartWhatsApp starts a WhatsApp bot using the whatsmeow library.
// dbPath is the path to the SQLite database for storing session data.
// allowFrom restricts which phone numbers (digits only, e.g. "15551234567") may
// send messages; empty means allow all.
func StartWhatsApp(ctx context.Context, hub *chat.Hub, dbPath string, allowFrom []string) error {
	if dbPath == "" {
		return fmt.Errorf("whatsapp database path not provided")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return fmt.Errorf("failed to create whatsapp db directory: %w", err)
	}

	container, err := sqlstore.New(ctx, "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)", whatsappLogger{})
	if err != nil {
		return fmt.Errorf("failed to connect to whatsapp database: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("failed to get whatsapp device: %w", err)
	}

	rawClient := whatsmeow.NewClient(deviceStore, whatsappLogger{})
	if rawClient.Store.ID == nil {
		return fmt.Errorf("whatsapp not authenticated - please run 'picobot channels login' and select WhatsApp")
	}

	sender := &realWhatsAppSender{c: rawClient}
	own := *rawClient.Store.ID
	ownLID := rawClient.Store.GetLID()
	waClient := newWhatsAppClient(ctx, sender, hub, allowFrom, own, ownLID)
	rawClient.AddEventHandler(waClient.handleEvent)

	if err := rawClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect to whatsapp: %w", err)
	}
	if ownLID.IsEmpty() {
		log.Printf("whatsapp: connected as %s", own.User)
	} else {
		log.Printf("whatsapp: connected as %s (LID: %s)", own.User, ownLID.User)
	}

	go waClient.runOutbound()
	go func() {
		<-ctx.Done()
		log.Println("whatsapp: shutting down")
		waClient.stopAllTyping()
		rawClient.Disconnect()
	}()

	return nil
}

// SetupWhatsApp displays a QR code for WhatsApp authentication.
// Run once per device before enabling the channel in the config.
func SetupWhatsApp(dbPath string) error {
	if dbPath == "" {
		return fmt.Errorf("whatsapp database path not provided")
	}

	ctx := context.Background()

	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return fmt.Errorf("failed to create whatsapp db directory: %w", err)
	}

	// Use quiet logger to keep onboarding output clean. (Add 5 seconds to wait when the DB is locked during initial migration)
	container, err := sqlstore.New(ctx, "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)", quietLogger{})
	if err != nil {
		return fmt.Errorf("failed to connect to whatsapp database: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("failed to get whatsapp device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, quietLogger{})

	if client.Store.ID != nil {
		fmt.Printf("Already authenticated as %s\n", client.Store.ID.User)
		fmt.Println("To re-authenticate, delete the database file and run setup again.")
		return nil
	}

	// Listen for the Connected event that fires after the post-pairing reconnect.
	connected := make(chan struct{}, 1)
	client.AddEventHandler(func(evt interface{}) {
		if _, ok := evt.(*events.Connected); ok {
			select {
			case connected <- struct{}{}:
			default:
			}
		}
	})

	qrChan, _ := client.GetQRChannel(context.Background())

	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect to whatsapp: %w", err)
	}
	defer client.Disconnect()

	fmt.Println("Scan the QR code below with WhatsApp on your phone:")
	fmt.Println("(Open WhatsApp > Settings > Linked Devices > Link a Device)")
	fmt.Println()

	for evt := range qrChan {
		switch evt.Event {
		case "code":
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			fmt.Println()
		case "success":
			fmt.Println("Pairing successful, finishing setup...")
		case "timeout":
			return fmt.Errorf("QR code timed out, please try again")
		}
	}

	// Wait for the post-pairing reconnection, then hold for initial device sync.
	select {
	case <-connected:
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timed out waiting for connection after pairing")
	}

	fmt.Println("Syncing with phone...")
	time.Sleep(15 * time.Second)

	fmt.Println("Successfully authenticated!")
	if client.Store.ID != nil {
		fmt.Printf("Logged in as: %s\n", client.Store.ID.User)
	}
	return nil
}

// whatsappClient handles WhatsApp messaging.
type whatsappClient struct {
	sender     whatsappSender
	hub        *chat.Hub
	outCh      <-chan chat.Outbound
	allowed    map[string]struct{}
	own        types.JID // phone JID  (e.g. 85298765432@s.whatsapp.net)
	ownLID     types.JID // LID JID    (e.g. 169032883908635@lid) — may be empty
	ctx        context.Context
	typingMu   sync.Mutex
	typingStop map[string]chan struct{}
}

// newWhatsAppClient constructs a whatsappClient and registers it as the hub's
// "whatsapp" outbound subscriber. Inject a mock whatsappSender for tests.
// ownJID  = rawClient.Store.ID   (phone JID)  — pass types.JID{} in tests.
// ownLID  = rawClient.Store.GetLID() (LID JID) — pass types.JID{} in tests.
func newWhatsAppClient(ctx context.Context, sender whatsappSender, hub *chat.Hub, allowFrom []string, ownJID, ownLID types.JID) *whatsappClient {
	allowed := make(map[string]struct{}, len(allowFrom))
	for _, num := range allowFrom {
		allowed[num] = struct{}{}
	}
	return &whatsappClient{
		sender:     sender,
		hub:        hub,
		outCh:      hub.Subscribe("whatsapp"),
		allowed:    allowed,
		own:        ownJID,
		ownLID:     ownLID,
		ctx:        ctx,
		typingStop: make(map[string]chan struct{}),
	}
}

// handleEvent processes WhatsApp events.
func (c *whatsappClient) handleEvent(evt interface{}) {
	switch evt.(type) {
	case *events.PushNameSetting:
		// PushName is now available — safe to advertise online presence.
		if err := c.sender.SendPresence(c.ctx, types.PresenceAvailable); err != nil {
			log.Printf("whatsapp: failed to send available presence: %v", err)
		}
	case *events.Message:
		c.handleMessage(evt.(*events.Message))
	}
}

// isSelfChat reports whether msg is the user messaging themselves (Notes to Self).
// WhatsApp uses the sender's own JID as the chat JID for self-chat messages.
// On newer accounts the chat JID uses the @lid server, so we match against both
// the phone JID and the LID JID.
func (c *whatsappClient) isSelfChat(msg *events.Message) bool {
	if !msg.Info.IsFromMe {
		return false
	}
	chatUser := msg.Info.Chat.User
	if chatUser == "" {
		return false
	}
	// Match phone JID (s.whatsapp.net) or LID JID (@lid).
	return (c.own.User != "" && chatUser == c.own.User) ||
		(c.ownLID.User != "" && chatUser == c.ownLID.User)
}

// handleMessage processes an incoming WhatsApp direct message.
func (c *whatsappClient) handleMessage(msg *events.Message) {
	if msg.Info.IsFromMe {
		// Only allow self-chat (Notes to Self); drop echoes of messages sent elsewhere.
		if !c.isSelfChat(msg) {
			return
		}
		// Self-chat: it is always the owner. Skip allowlist and fall through.
	} else {
		// Regular inbound message — enforce allowlist.
		if msg.Info.IsGroup {
			return
		}
		senderID := msg.Info.Sender.User
		if len(c.allowed) > 0 {
			if _, ok := c.allowed[senderID]; !ok {
				log.Printf("whatsapp: dropped message from unauthorized sender %s (add '%s' to allowFrom to permit)",
					msg.Info.Sender.String(), senderID)
				return
			}
		}
	}

	// Use the full JID string for logging; the User part is used as SenderID in the hub.
	senderJID := msg.Info.Sender.String()
	senderID := msg.Info.Sender.User

	// Send read receipt (blue ticks) before processing.
	_ = c.sender.MarkRead(c.ctx, []types.MessageID{msg.Info.ID}, msg.Info.Timestamp, msg.Info.Chat, msg.Info.Sender)

	content := extractMessageText(msg.Message)
	if content == "" {
		return
	}
	content = strings.TrimSpace(content)
	chatID := msg.Info.Chat.String()

	log.Printf("whatsapp: message from %s in chat %s: %s", senderJID, chatID, truncate(content, 50))

	c.startTyping(msg.Info.Chat)

	c.hub.In <- chat.Inbound{
		Channel:   "whatsapp",
		SenderID:  senderID,
		ChatID:    chatID,
		Content:   content,
		Timestamp: msg.Info.Timestamp,
		Metadata: map[string]interface{}{
			"message_id": msg.Info.ID,
			"is_group":   msg.Info.IsGroup,
		},
	}
}

// extractMessageText returns the plain-text content from a WhatsApp proto message.
// Returns an empty string for unsupported or empty message types.
func extractMessageText(m *waProto.Message) string {
	if m == nil {
		return ""
	}
	if m.Conversation != nil {
		return *m.Conversation
	}
	if m.ExtendedTextMessage != nil && m.ExtendedTextMessage.Text != nil {
		return *m.ExtendedTextMessage.Text
	}
	if m.ImageMessage != nil {
		caption := ""
		if m.ImageMessage.Caption != nil {
			caption = *m.ImageMessage.Caption
		}
		return caption + "\n[Image received - images not yet supported]"
	}
	if m.DocumentMessage != nil {
		caption := ""
		if m.DocumentMessage.Caption != nil {
			caption = *m.DocumentMessage.Caption
		}
		if m.DocumentMessage.FileName != nil {
			caption += fmt.Sprintf("\n[Document: %s - documents not yet supported]", *m.DocumentMessage.FileName)
		}
		return caption
	}
	return ""
}

// runOutbound reads replies from the hub's whatsapp subscription and sends them.
func (c *whatsappClient) runOutbound() {
	for {
		select {
		case <-c.ctx.Done():
			log.Println("whatsapp: stopping outbound sender")
			return
		case out := <-c.outCh:
			recipient, err := types.ParseJID(out.ChatID)
			if err != nil {
				log.Printf("whatsapp: invalid chat ID %s: %v", out.ChatID, err)
				continue
			}
			c.stopTyping(out.ChatID)
			// WhatsApp has a ~65 KB hard limit; use 4096 runes as a safe chunk size.
			for i, chunk := range splitMessage(out.Content, 4096) {
				if err := c.sender.SendText(c.ctx, recipient, chunk); err != nil {
					log.Printf("whatsapp: send error (chunk %d): %v", i+1, err)
				}
			}
		}
	}
}

// startTyping begins (or resets) a continuous "composing" presence for a chat.
// It stops automatically after 5 minutes or when stopTyping / stopAllTyping is called.
func (c *whatsappClient) startTyping(jid types.JID) {
	key := jid.String()
	c.typingMu.Lock()
	if stop, ok := c.typingStop[key]; ok {
		close(stop)
	}
	stop := make(chan struct{})
	c.typingStop[key] = stop
	c.typingMu.Unlock()

	go func() {
		_ = c.sender.SendChatPresence(c.ctx, jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)

		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		timeout := time.NewTimer(5 * time.Minute)
		defer timeout.Stop()

		for {
			select {
			case <-stop:
				_ = c.sender.SendChatPresence(c.ctx, jid, types.ChatPresencePaused, types.ChatPresenceMediaText)
				return
			case <-timeout.C:
				return
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				_ = c.sender.SendChatPresence(c.ctx, jid, types.ChatPresenceComposing, types.ChatPresenceMediaText)
			}
		}
	}()
}

// stopTyping cancels the typing indicator for the given chat.
func (c *whatsappClient) stopTyping(chatID string) {
	c.typingMu.Lock()
	defer c.typingMu.Unlock()
	if stop, ok := c.typingStop[chatID]; ok {
		close(stop)
		delete(c.typingStop, chatID)
	}
}

// stopAllTyping cancels all active typing indicators.
func (c *whatsappClient) stopAllTyping() {
	c.typingMu.Lock()
	defer c.typingMu.Unlock()
	for _, stop := range c.typingStop {
		close(stop)
	}
	c.typingStop = make(map[string]chan struct{})
}
