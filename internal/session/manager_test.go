package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func contents(msgs []Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Role + ": " + m.Content
	}
	return out
}

func TestSessionPersistsAcrossManagers(t *testing.T) {
	ws := t.TempDir()
	sm := NewSessionManager(ws)
	s := sm.GetOrCreate("telegram:123")
	s.AddMessage("user", "hi")
	s.AddMessage("assistant", "hello\nthere")
	if err := sm.Save(s); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Simulate a restart with a fresh manager.
	sm2 := NewSessionManager(ws)
	if err := sm2.LoadAll(); err != nil {
		t.Fatalf("load: %v", err)
	}
	got := sm2.GetOrCreate("telegram:123").Messages
	if strings.Join(contents(got), "|") != "user: hi|assistant: hello\nthere" {
		t.Fatalf("history mismatch: %q", contents(got))
	}
	if got[0].Time.IsZero() {
		t.Fatalf("expected timestamps to be persisted")
	}
}

func TestFileNamesDistinctForSimilarKeys(t *testing.T) {
	keys := []string{"chan:a/b", "chan:a_b", "chan:a:b", "slack:C1", "slack:c1"}
	seen := map[string]string{}
	for _, k := range keys {
		n := strings.ToLower(fileName(k)) // case-insensitive filesystems
		if other, ok := seen[n]; ok {
			t.Fatalf("keys %q and %q map to the same file %s", other, k, n)
		}
		seen[n] = k
		if strings.ContainsAny(fileName(k), "/\\:") {
			t.Fatalf("unsafe characters in file name %q", fileName(k))
		}
	}

	ws := t.TempDir()
	sm := NewSessionManager(ws)
	for _, k := range keys {
		s := sm.GetOrCreate(k)
		s.AddMessage("user", k)
		if err := sm.Save(s); err != nil {
			t.Fatalf("save %q: %v", k, err)
		}
	}
	sm2 := NewSessionManager(ws)
	if err := sm2.LoadAll(); err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if m := sm2.GetOrCreate(k).Messages; len(m) != 1 || m[0].Content != k {
			t.Fatalf("session %q not restored intact: %q", k, contents(m))
		}
	}
}

func TestSaveRefusesToOverwriteOtherSessionsFile(t *testing.T) {
	ws := t.TempDir()
	sm := NewSessionManager(ws)
	s := sm.GetOrCreate("telegram:1") // loads sessions
	s.AddMessage("user", "hi")

	// After loading, another process wrote a different session into the file
	// our key maps to.
	foreign := `{"Key":"someone:else","Messages":[{"role":"user","content":"keep me"}]}`
	fpath := filepath.Join(ws, "sessions", fileName("telegram:1"))
	if err := os.WriteFile(fpath, []byte(foreign), 0644); err != nil {
		t.Fatal(err)
	}

	if err := sm.Save(s); err == nil {
		t.Fatalf("expected collision error")
	}
	b, _ := os.ReadFile(fpath)
	if string(b) != foreign {
		t.Fatalf("foreign session file was modified: %s", b)
	}
}

func TestMisnamedFileAtLoadIsPreserved(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// A different session sits in the file telegram:1 maps to.
	foreign := `{"Key":"someone:else","Messages":[{"role":"user","content":"keep me","time":"` + time.Now().Format(time.RFC3339) + `"}]}`
	if err := os.WriteFile(filepath.Join(dir, fileName("telegram:1")), []byte(foreign), 0644); err != nil {
		t.Fatal(err)
	}

	sm := NewSessionManager(ws)
	s := sm.GetOrCreate("telegram:1")
	s.AddMessage("user", "hi")
	if err := sm.Save(s); err != nil {
		t.Fatalf("save: %v", err)
	}

	sm2 := NewSessionManager(ws)
	if m := sm2.GetOrCreate("someone:else").Messages; len(m) != 1 || m[0].Content != "keep me" {
		t.Fatalf("foreign session lost: %+v", m)
	}
	if m := sm2.GetOrCreate("telegram:1").Messages; len(m) != 1 || m[0].Content != "hi" {
		t.Fatalf("own session not saved: %+v", m)
	}
}

func TestLegacyFileLoadedAndMigrated(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "telegram:7.json")
	if err := os.WriteFile(legacy, []byte(`{"Key":"telegram:7","History":["user: old q","assistant: old a"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	sm := NewSessionManager(ws)
	sm.SetRetention(30 * 24 * time.Hour)
	if err := sm.LoadAll(); err != nil {
		t.Fatal(err)
	}
	s := sm.GetOrCreate("telegram:7")
	if strings.Join(contents(s.Messages), "|") != "user: old q|assistant: old a" {
		t.Fatalf("legacy history not loaded: %q", contents(s.Messages))
	}

	s.AddMessage("user", "new")
	if err := sm.Save(s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("expected legacy file to be removed after migration")
	}
	sm2 := NewSessionManager(ws)
	if err := sm2.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if n := len(sm2.GetOrCreate("telegram:7").Messages); n != 3 {
		t.Fatalf("expected 3 messages after migration, got %d", n)
	}
}

func TestLoadAllIgnoresNonJSONAndCorruptFiles(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "x.json.tmp"), []byte(`{"Key":"tmp","Messages":[]}`), 0644)
	_ = os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{not json`), 0644)

	sm := NewSessionManager(ws)
	if err := sm.LoadAll(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(sm.sessions) != 0 {
		t.Fatalf("expected no sessions loaded, got %d", len(sm.sessions))
	}
}

func TestRetentionPrunesOldMessages(t *testing.T) {
	sm := NewSessionManager(t.TempDir())
	sm.SetRetention(14 * 24 * time.Hour)
	s := sm.GetOrCreate("k")
	now := time.Now()
	s.Messages = []Message{
		{Role: "user", Content: "ancient", Time: now.Add(-20 * 24 * time.Hour)},
		{Role: "assistant", Content: "ancient reply", Time: now.Add(-20 * 24 * time.Hour)},
		{Role: "user", Content: "last week", Time: now.Add(-7 * 24 * time.Hour)},
	}
	// Pruned on access, not only on save, so idle chats don't resurface stale history.
	s = sm.GetOrCreate("k")
	if strings.Join(contents(s.Messages), "|") != "user: last week" {
		t.Fatalf("unexpected messages after prune: %q", contents(s.Messages))
	}
}

func TestSaveCapsStoredMessages(t *testing.T) {
	sm := NewSessionManager(t.TempDir())
	s := sm.GetOrCreate("k")
	for i := 0; i < MaxStoredMessages+10; i++ {
		s.AddMessage("user", "m")
	}
	if err := sm.Save(s); err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(s.Messages) != MaxStoredMessages {
		t.Fatalf("expected %d messages, got %d", MaxStoredMessages, len(s.Messages))
	}
}

func TestRecentRespectsCharBudget(t *testing.T) {
	s := &Session{}
	s.AddMessage("user", "aaaaaaaaaa")      // 10
	s.AddMessage("assistant", "bbbbbbbbbb") // 10
	s.AddMessage("user", "cccccccccc")      // 10
	s.AddMessage("assistant", "dddddddddd") // 10

	if got := contents(s.Recent(0)); len(got) != 4 {
		t.Fatalf("expected all messages with no budget, got %q", got)
	}
	// 25 chars fits the last two messages; the window starts at a user turn.
	if got := strings.Join(contents(s.Recent(25)), "|"); got != "user: cccccccccc|assistant: dddddddddd" {
		t.Fatalf("unexpected window: %q", got)
	}
	// 35 chars fits three messages, but the leading assistant turn is dropped.
	if got := strings.Join(contents(s.Recent(35)), "|"); got != "user: cccccccccc|assistant: dddddddddd" {
		t.Fatalf("expected leading assistant turn dropped, got %q", got)
	}
	if got := s.Recent(5); len(got) != 0 {
		t.Fatalf("expected empty window when nothing fits, got %q", contents(got))
	}
}

func TestNewConversationAndSwitchPersistAcrossRestart(t *testing.T) {
	ws := t.TempDir()
	sm := NewSessionManager(ws)
	base := "telegram:1"

	first := sm.Active(base)
	if first.Key != base {
		t.Fatalf("expected default conversation keyed by base, got %q", first.Key)
	}
	first.AddMessage("user", "trip planning")
	if err := sm.Save(first); err != nil {
		t.Fatal(err)
	}

	second, err := sm.NewConversation(base, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if second.Key == base || !strings.HasPrefix(second.Key, base+"#") {
		t.Fatalf("unexpected new conversation key %q", second.Key)
	}
	if sm.Active(base) != second || len(second.Messages) != 0 {
		t.Fatalf("expected new empty conversation to be active")
	}
	second.AddMessage("user", "standup notes")
	if err := sm.Save(second); err != nil {
		t.Fatal(err)
	}

	// Restart: the active conversation and both histories survive.
	sm2 := NewSessionManager(ws)
	if got := sm2.Active(base); got.Key != second.Key || got.Title != "Work" || len(got.Messages) != 1 {
		t.Fatalf("active conversation not restored: %+v", got)
	}
	convs := sm2.Conversations(base)
	if len(convs) != 2 || convs[0].Key != second.Key || convs[1].Key != base {
		t.Fatalf("expected both conversations, newest first, got %+v", convs)
	}

	if err := sm2.SetActive(base, base); err != nil {
		t.Fatal(err)
	}
	sm3 := NewSessionManager(ws)
	if got := sm3.Active(base); got.Key != base || got.Messages[0].Content != "trip planning" {
		t.Fatalf("switch not persisted: %+v", got)
	}
}

func TestNewConversationReusesEmptyActive(t *testing.T) {
	sm := NewSessionManager(t.TempDir())
	a, err := sm.NewConversation("telegram:1", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := sm.NewConversation("telegram:1", "renamed")
	if err != nil {
		t.Fatal(err)
	}
	if a != b || b.Title != "renamed" {
		t.Fatalf("expected empty active conversation to be reused")
	}
	if n := len(sm.Conversations("telegram:1")); n != 1 {
		t.Fatalf("expected 1 conversation, got %d", n)
	}
}

func TestConversationsScopedToChat(t *testing.T) {
	sm := NewSessionManager(t.TempDir())
	for _, k := range []string{"telegram:1", "telegram:12", "telegram:1#x"} {
		s := sm.GetOrCreate(k)
		s.AddMessage("user", k)
	}
	if n := len(sm.Conversations("telegram:1")); n != 2 {
		t.Fatalf("expected 2 conversations for telegram:1 (not telegram:12), got %d", n)
	}
	if err := sm.SetActive("telegram:1", "telegram:12"); err == nil {
		t.Fatalf("expected error switching to another chat's conversation")
	}
	if err := sm.SetActive("telegram:1", "telegram:1#missing"); err == nil {
		t.Fatalf("expected error switching to unknown conversation")
	}
}

func TestLoadDeletesExpiredConversations(t *testing.T) {
	ws := t.TempDir()
	sm := NewSessionManager(ws)
	old := time.Now().Add(-40 * 24 * time.Hour)
	for _, k := range []string{"telegram:1", "telegram:1#old", "telegram:1#keep"} {
		s := sm.GetOrCreate(k)
		s.Created = old
		s.Messages = []Message{{Role: "user", Content: k, Time: old}}
		if err := sm.Save(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := sm.SetActive("telegram:1", "telegram:1#keep"); err != nil {
		t.Fatal(err)
	}

	sm2 := NewSessionManager(ws)
	sm2.SetRetention(30 * 24 * time.Hour)
	sm2.Active("telegram:1") // triggers load
	for k, wantExists := range map[string]bool{"telegram:1": false, "telegram:1#old": false, "telegram:1#keep": true} {
		_, err := os.Stat(filepath.Join(ws, "sessions", fileName(k)))
		if exists := err == nil; exists != wantExists {
			t.Fatalf("%s: file exists=%v, want %v", k, exists, wantExists)
		}
	}
}

func TestDisplayTitle(t *testing.T) {
	s := &Session{}
	if s.DisplayTitle() != "(empty)" {
		t.Fatalf("unexpected title for empty session: %q", s.DisplayTitle())
	}
	s.AddMessage("user", "  what's   the weather\nin Paris tomorrow and the day after that please  ")
	if got := s.DisplayTitle(); got != "what's the weather in Paris tomorrow and…" {
		t.Fatalf("unexpected derived title %q", got)
	}
	s.Title = "Paris"
	if s.DisplayTitle() != "Paris" {
		t.Fatalf("explicit title should win")
	}
}

func TestConversationNumbersArePermanent(t *testing.T) {
	ws := t.TempDir()
	sm := NewSessionManager(ws)
	base := "telegram:1"

	first := sm.Active(base)
	first.AddMessage("user", "one")
	if err := sm.Save(first); err != nil {
		t.Fatal(err)
	}
	if first.Number != 1 {
		t.Fatalf("original conversation should be #1, got %d", first.Number)
	}
	keys := map[int]string{1: first.Key}
	for want := 2; want <= 5; want++ {
		s, err := sm.NewConversation(base, "")
		if err != nil {
			t.Fatal(err)
		}
		if s.Number != want {
			t.Fatalf("expected #%d, got #%d", want, s.Number)
		}
		s.AddMessage("user", "msg")
		if err := sm.Save(s); err != nil {
			t.Fatal(err)
		}
		keys[want] = s.Key
	}

	// Activity in an old conversation and a restart change nothing.
	if err := sm.SetActive(base, keys[2]); err != nil {
		t.Fatal(err)
	}
	s2 := sm.Active(base)
	s2.AddMessage("user", "more")
	if err := sm.Save(s2); err != nil {
		t.Fatal(err)
	}
	sm2 := NewSessionManager(ws)
	for n, key := range keys {
		s, ok := sm2.Conversation(base, n)
		if !ok || s.Key != key {
			t.Fatalf("#%d should still be %q, got %+v", n, key, s)
		}
	}

	// Numbers are not reused after a conversation disappears.
	if err := os.Remove(filepath.Join(ws, "sessions", fileName(keys[5]))); err != nil {
		t.Fatal(err)
	}
	sm3 := NewSessionManager(ws)
	if err := sm3.SetActive(base, keys[1]); err != nil {
		t.Fatal(err)
	}
	s, err := sm3.NewConversation(base, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.Number != 6 {
		t.Fatalf("expected #6 after #5 was removed, got #%d", s.Number)
	}
}

func TestNewConversationReusesAnyEmptyConversation(t *testing.T) {
	sm := NewSessionManager(t.TempDir())
	base := "telegram:1"
	first := sm.Active(base)
	first.AddMessage("user", "hi")
	empty, err := sm.NewConversation(base, "")
	if err != nil {
		t.Fatal(err)
	}
	// Switch away from the empty conversation, then ask for a new one again.
	if err := sm.SetActive(base, first.Key); err != nil {
		t.Fatal(err)
	}
	again, err := sm.NewConversation(base, "")
	if err != nil {
		t.Fatal(err)
	}
	if again != empty || again.Number != 2 || sm.Active(base) != empty {
		t.Fatalf("expected empty #2 to be reused and active, got #%d", again.Number)
	}
}

func TestUnnumberedSessionsNumberedOnceOldestFirst(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Written by a build without numbers: two extra conversations plus the original.
	files := map[string]time.Time{
		"telegram:1#b": now.Add(-1 * time.Hour),
		"telegram:1#a": now.Add(-2 * time.Hour),
		"telegram:1":   now.Add(-3 * time.Hour),
	}
	for key, created := range files {
		b := `{"Key":"` + key + `","Created":"` + created.Format(time.RFC3339Nano) + `","Messages":[{"role":"user","content":"x","time":"` + created.Format(time.RFC3339Nano) + `"}]}`
		if err := os.WriteFile(filepath.Join(dir, fileName(key)), []byte(b), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]int{"telegram:1": 1, "telegram:1#a": 2, "telegram:1#b": 3}
	for i := 0; i < 2; i++ { // numbers stick across restarts
		sm := NewSessionManager(ws)
		for key, n := range want {
			if got := sm.GetOrCreate(key).Number; got != n {
				t.Fatalf("load %d: %s got #%d, want #%d", i, key, got, n)
			}
		}
	}
}
