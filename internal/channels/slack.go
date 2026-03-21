package channels

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/local/picobot/internal/chat"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// slackPoster is the subset of *slack.Client used for posting outbound messages.
// It exists to enable testing without a live Slack connection, mirroring the
// discordSender pattern used by the Discord channel.
type slackPoster interface {
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
}

// StartSlack starts a Slack bot using Socket Mode.
// allowUsers restricts which Slack user IDs may send messages; empty means allow all.
// allowChannels restricts which Slack channel IDs may send messages; empty means allow all.
func StartSlack(ctx context.Context, hub *chat.Hub, appToken, botToken string, allowUsers, allowChannels []string) error {
	if appToken == "" {
		return fmt.Errorf("slack app token not provided")
	}
	if botToken == "" {
		return fmt.Errorf("slack bot token not provided")
	}
	if !strings.HasPrefix(appToken, "xapp-") {
		return fmt.Errorf("slack app token must start with xapp-")
	}
	if !strings.HasPrefix(botToken, "xoxb-") {
		return fmt.Errorf("slack bot token must start with xoxb-")
	}

	api := slack.New(
		botToken,
		slack.OptionAppLevelToken(appToken),
	)

	auth, err := api.AuthTest()
	if err != nil {
		return fmt.Errorf("slack auth test failed: %w", err)
	}
	if auth.UserID == "" {
		return fmt.Errorf("slack auth test returned empty user ID")
	}

	socketClient := socketmode.New(api)
	client := newSlackClient(ctx, socketClient, api, hub, auth.UserID, allowUsers, allowChannels)

	go client.runOutbound()
	go client.runEvents()

	go func() {
		if err := socketClient.RunContext(ctx); err != nil {
			log.Printf("slack: socket mode error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		log.Println("slack: shutting down")
	}()

	return nil
}

type slackClient struct {
	socket       *socketmode.Client
	poster       slackPoster
	hub          *chat.Hub
	outCh        <-chan chat.Outbound
	botID        string
	allowedUsers map[string]struct{}
	allowedChans map[string]struct{}
	ctx          context.Context
}

func newSlackClient(ctx context.Context, socket *socketmode.Client, poster slackPoster, hub *chat.Hub, botID string, allowUsers, allowChannels []string) *slackClient {
	allowedUsers := make(map[string]struct{}, len(allowUsers))
	for _, id := range allowUsers {
		allowedUsers[id] = struct{}{}
	}
	allowedChans := make(map[string]struct{}, len(allowChannels))
	for _, id := range allowChannels {
		allowedChans[id] = struct{}{}
	}

	return &slackClient{
		socket:       socket,
		poster:       poster,
		hub:          hub,
		outCh:        hub.Subscribe("slack"),
		botID:        botID,
		allowedUsers: allowedUsers,
		allowedChans: allowedChans,
		ctx:          ctx,
	}
}

func (c *slackClient) runEvents() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case evt, ok := <-c.socket.Events:
			if !ok {
				return
			}
			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					log.Printf("slack: unexpected event data: %T", evt.Data)
					continue
				}
				c.socket.Ack(*evt.Request)
				if eventsAPIEvent.Type != slackevents.CallbackEvent {
					continue
				}
				c.handleCallbackEvent(eventsAPIEvent.InnerEvent)
			case socketmode.EventTypeInvalidAuth:
				log.Println("slack: invalid auth")
				return
			}
		}
	}
}

func (c *slackClient) handleCallbackEvent(inner slackevents.EventsAPIInnerEvent) {
	switch ev := inner.Data.(type) {
	case *slackevents.AppMentionEvent:
		c.handleMention(ev)
	case *slackevents.MessageEvent:
		c.handleMessage(ev)
	}
}

func (c *slackClient) handleMention(ev *slackevents.AppMentionEvent) {
	if ev.User == "" || ev.User == c.botID || ev.BotID != "" {
		return
	}

	if !c.isAllowed(ev.User, ev.Channel, false) {
		c.logUnauthorized(ev.User, ev.Channel, false)
		return
	}

	content := stripSlackMention(ev.Text, c.botID)
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	threadTS := ev.ThreadTimeStamp
	chatID := formatSlackChatID(ev.Channel, threadTS)
	teamID := firstNonEmpty(ev.SourceTeam, ev.UserTeam)

	log.Printf("slack: mention from %s in %s: %s", ev.User, ev.Channel, truncate(content, 50))

	c.hub.In <- chat.Inbound{
		Channel:   "slack",
		SenderID:  ev.User,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"channel_id": ev.Channel,
			"team_id":    teamID,
			"thread_ts":  threadTS,
			"is_dm":      false,
		},
	}
}

func (c *slackClient) handleMessage(ev *slackevents.MessageEvent) {
	if ev.User == "" || ev.User == c.botID || ev.BotID != "" {
		return
	}
	if ev.SubType != "" {
		return
	}

	isDM := ev.ChannelType == "im"
	if !isDM {
		return
	}

	if !c.isAllowed(ev.User, ev.Channel, isDM) {
		c.logUnauthorized(ev.User, ev.Channel, isDM)
		return
	}

	content := strings.TrimSpace(ev.Text)
	content = appendSlackAttachments(content, ev.Files)
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	threadTS := ev.ThreadTimeStamp
	chatID := formatSlackChatID(ev.Channel, threadTS)
	teamID := firstNonEmpty(ev.SourceTeam, ev.UserTeam)

	log.Printf("slack: message from %s in %s: %s", ev.User, ev.Channel, truncate(content, 50))

	c.hub.In <- chat.Inbound{
		Channel:   "slack",
		SenderID:  ev.User,
		ChatID:    chatID,
		Content:   content,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"channel_id": ev.Channel,
			"team_id":    teamID,
			"thread_ts":  threadTS,
			"is_dm":      isDM,
		},
	}
}

func (c *slackClient) runOutbound() {
	for {
		select {
		case <-c.ctx.Done():
			log.Println("slack: stopping outbound sender")
			return
		case out := <-c.outCh:
			channelID, threadTS := splitSlackChatID(out.ChatID)
			if channelID == "" {
				log.Printf("slack: invalid chat ID %q", out.ChatID)
				continue
			}
			for _, chunk := range splitMessage(out.Content, 4000) {
				opts := []slack.MsgOption{slack.MsgOptionText(chunk, false)}
				if threadTS != "" {
					opts = append(opts, slack.MsgOptionTS(threadTS))
				}
				if _, _, err := c.poster.PostMessageContext(c.ctx, channelID, opts...); err != nil {
					log.Printf("slack: send error: %v", err)
				}
			}
		}
	}
}

func (c *slackClient) isAllowed(userID, channelID string, isDM bool) bool {
	if len(c.allowedUsers) > 0 {
		if _, ok := c.allowedUsers[userID]; !ok {
			return false
		}
	}
	if isDM {
		return true
	}
	if len(c.allowedChans) > 0 {
		if _, ok := c.allowedChans[channelID]; !ok {
			return false
		}
	}
	return true
}

func (c *slackClient) logUnauthorized(userID, channelID string, isDM bool) {
	userAllowed := true
	channelAllowed := true
	if len(c.allowedUsers) > 0 {
		_, userAllowed = c.allowedUsers[userID]
	}
	if isDM {
		channelAllowed = true
	} else if len(c.allowedChans) > 0 {
		_, channelAllowed = c.allowedChans[channelID]
	}
	log.Printf("slack: dropped message: user allowed=%t channel allowed=%t user=%s channel=%s", userAllowed, channelAllowed, userID, channelID)
}

func stripSlackMention(text, botID string) string {
	if botID == "" {
		return text
	}
	return strings.ReplaceAll(text, "<@"+botID+">", "")
}

func appendSlackAttachments(content string, files []slackevents.File) string {
	for _, file := range files {
		url := file.URLPrivate
		if url == "" {
			url = file.URLPrivateDownload
		}
		if url == "" {
			url = file.Permalink
		}
		if url == "" {
			continue
		}
		if content != "" {
			content += "\n"
		}
		content += fmt.Sprintf("[attachment: %s]", url)
	}
	return content
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func formatSlackChatID(channelID, threadTS string) string {
	if threadTS == "" {
		return channelID
	}
	return channelID + "::" + threadTS
}

func splitSlackChatID(chatID string) (string, string) {
	parts := strings.SplitN(chatID, "::", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return chatID, ""
}
