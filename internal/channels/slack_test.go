package channels

import (
	"testing"

	"github.com/slack-go/slack/slackevents"
)

func TestSlackChatIDHelpers(t *testing.T) {
	channelID := "C123456"
	threadTS := "1699999999.123456"

	withThread := formatSlackChatID(channelID, threadTS)
	ch, ts := splitSlackChatID(withThread)
	if ch != channelID || ts != threadTS {
		t.Fatalf("expected %s/%s, got %s/%s", channelID, threadTS, ch, ts)
	}

	noThread := formatSlackChatID(channelID, "")
	ch, ts = splitSlackChatID(noThread)
	if ch != channelID || ts != "" {
		t.Fatalf("expected %s with empty thread, got %s/%s", channelID, ch, ts)
	}
}

func TestStripSlackMention(t *testing.T) {
	text := "<@U123> hello there"
	clean := stripSlackMention(text, "U123")
	if clean != " hello there" {
		t.Fatalf("unexpected cleaned text: %q", clean)
	}
}

func TestSlackAllowlists(t *testing.T) {
	c := &slackClient{
		allowedUsers: map[string]struct{}{"U1": {}},
		allowedChans: map[string]struct{}{"C1": {}},
	}

	if !c.isAllowed("U1", "C1", false) {
		t.Fatal("expected allowed user and channel")
	}
	if c.isAllowed("U2", "C1", false) {
		t.Fatal("expected user U2 to be blocked")
	}
	if c.isAllowed("U1", "C2", false) {
		t.Fatal("expected channel C2 to be blocked")
	}

	open := &slackClient{allowedUsers: map[string]struct{}{}, allowedChans: map[string]struct{}{}}
	if !open.isAllowed("U999", "C999", false) {
		t.Fatal("expected empty allowlists to permit all")
	}

	if !c.isAllowed("U1", "D1", true) {
		t.Fatal("expected DM to bypass channel allowlist")
	}
}

func TestSlackAttachmentAppend(t *testing.T) {
	files := []slackevents.File{
		{URLPrivate: "https://files.example.com/a"},
		{URLPrivateDownload: "https://files.example.com/b"},
		{Permalink: "https://files.example.com/c"},
	}

	content := appendSlackAttachments("", files)
	if content == "" {
		t.Fatal("expected attachment content")
	}
}
