package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MaxStoredMessages is a hard safety cap on messages kept per session,
// applied on top of the time-based retention so a very busy chat cannot
// grow its session file without bound.
const MaxStoredMessages = 10000

// chatsFile records per-chat state (active conversation, last number). It
// cannot clash with a session file because those always end in "-<hash>.json".
const chatsFile = "_chats.json"

// legacyActiveFile is the chat index written by an earlier development build;
// it is skipped when loading.
const legacyActiveFile = "_active.json"

// Message is a single chat turn persisted in a session.
type Message struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

// Session holds the history of one conversation. A chat ("channel:chatID",
// the base key) may have several conversations: the original one keyed by the
// base itself and later ones keyed "base#id". Number identifies the
// conversation within its chat; it never changes and is never reused.
type Session struct {
	Key      string
	Number   int
	Title    string
	Created  time.Time
	Messages []Message
}

// chatState is the persisted per-chat index entry.
type chatState struct {
	Active     string `json:"active,omitempty"`
	LastNumber int    `json:"lastNumber"`
}

// sessionFile is the on-disk format. History is the legacy format
// ("role: content" strings without timestamps) and is only read, never written.
type sessionFile struct {
	Key      string
	Number   int       `json:",omitempty"`
	Title    string    `json:",omitempty"`
	Created  time.Time `json:",omitzero"`
	Messages []Message `json:",omitempty"`
	History  []string  `json:",omitempty"`
}

// SessionManager stores sessions in memory and persists to disk under workspace.
// Sessions are loaded from disk on first use, so SetRetention should be called
// before that to have expired conversations cleaned up with the right window.
type SessionManager struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	files     map[string]string     // file name -> session key
	paths     map[string]string     // session key -> file name currently on disk
	chats     map[string]*chatState // base key -> chat index entry
	loaded    bool
	workspace string
	retention time.Duration
}

func NewSessionManager(workspace string) *SessionManager {
	return &SessionManager{
		sessions:  make(map[string]*Session),
		files:     make(map[string]string),
		paths:     make(map[string]string),
		chats:     make(map[string]*chatState),
		workspace: workspace,
	}
}

// SetRetention sets how long messages are kept. Non-positive keeps everything
// (subject to MaxStoredMessages).
func (sm *SessionManager) SetRetention(d time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.retention = d
}

// GetOrCreate returns the session for key, dropping messages that have aged
// past the retention window.
func (sm *SessionManager) GetOrCreate(key string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.ensureLoaded()
	return sm.getOrCreate(key)
}

func (sm *SessionManager) getOrCreate(key string) *Session {
	if s, ok := sm.sessions[key]; ok {
		s.prune(time.Now(), sm.retention)
		return s
	}
	s := &Session{Key: key, Created: time.Now(), Messages: make([]Message, 0)}
	sm.assignNumber(s)
	sm.sessions[key] = s
	return s
}

// baseOf returns the chat ("channel:chatID") a conversation key belongs to.
func baseOf(key string) string {
	base, _, _ := strings.Cut(key, "#")
	return base
}

func (sm *SessionManager) chat(base string) *chatState {
	c, ok := sm.chats[base]
	if !ok {
		c = &chatState{}
		sm.chats[base] = c
	}
	return c
}

// assignNumber gives s its permanent number: 1 for a chat's original
// conversation, otherwise the next unused number for the chat. The counter
// starts at 1 so #1 stays reserved for the original conversation.
func (sm *SessionManager) assignNumber(s *Session) {
	c := sm.chat(baseOf(s.Key))
	if s.Key == baseOf(s.Key) {
		s.Number = 1
	} else {
		s.Number = max(c.LastNumber, 1) + 1
	}
	c.LastNumber = max(c.LastNumber, s.Number)
}

// Active returns the active conversation for the chat identified by base.
func (sm *SessionManager) Active(base string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.ensureLoaded()
	return sm.getOrCreate(sm.activeKey(base))
}

func (sm *SessionManager) activeKey(base string) string {
	if c, ok := sm.chats[base]; ok && c.Active != "" {
		return c.Active
	}
	return base
}

// NewConversation starts a new conversation for base and makes it active.
// Previous conversations are kept. An existing empty conversation (preferring
// the active one) is reused rather than piling up empty conversations.
func (sm *SessionManager) NewConversation(base, title string) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.ensureLoaded()

	reuse := sm.getOrCreate(sm.activeKey(base))
	if len(reuse.Messages) != 0 {
		reuse = nil
		for key, s := range sm.sessions {
			if baseOf(key) == base && len(s.Messages) == 0 && (reuse == nil || s.Number < reuse.Number) {
				reuse = s
			}
		}
	}
	if reuse != nil {
		reuse.Title = title
		if err := sm.save(reuse); err != nil {
			return nil, err
		}
		sm.chat(base).Active = reuse.Key
		return reuse, sm.saveChats()
	}

	now := time.Now()
	id := now.Format("20060102-150405")
	key := base + "#" + id
	for i := 2; sm.sessions[key] != nil; i++ {
		key = fmt.Sprintf("%s#%s-%d", base, id, i)
	}
	s := &Session{Key: key, Title: title, Created: now, Messages: make([]Message, 0)}
	sm.assignNumber(s)
	sm.sessions[key] = s
	if err := sm.save(s); err != nil {
		delete(sm.sessions, key)
		return nil, err
	}
	sm.chat(base).Active = key
	return s, sm.saveChats()
}

// Conversation returns the conversation of base with the given number.
func (sm *SessionManager) Conversation(base string, number int) (*Session, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.ensureLoaded()
	for key, s := range sm.sessions {
		if baseOf(key) == base && s.Number == number {
			s.prune(time.Now(), sm.retention)
			return s, true
		}
	}
	return nil, false
}

// Conversations lists the conversations of base, most recently active first.
// Empty conversations are omitted unless active.
func (sm *SessionManager) Conversations(base string) []*Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.ensureLoaded()

	activeKey := sm.activeKey(base)
	now := time.Now()
	var out []*Session
	for key, s := range sm.sessions {
		if baseOf(key) != base {
			continue
		}
		s.prune(now, sm.retention)
		if len(s.Messages) == 0 && key != activeKey {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastActivity().After(out[j].LastActivity())
	})
	return out
}

// SetActive makes key the active conversation for base.
func (sm *SessionManager) SetActive(base, key string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.ensureLoaded()
	if baseOf(key) != base {
		return fmt.Errorf("conversation %q does not belong to %q", key, base)
	}
	if _, ok := sm.sessions[key]; !ok {
		return fmt.Errorf("conversation %q not found", key)
	}
	sm.chat(base).Active = key
	return sm.saveChats()
}

// IsActive reports whether key is the active conversation for base.
func (sm *SessionManager) IsActive(base, key string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.activeKey(base) == key
}

func (sm *SessionManager) dir() string {
	return filepath.Join(sm.workspace, "sessions")
}

func (sm *SessionManager) Save(s *Session) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.ensureLoaded()
	return sm.save(s)
}

func (sm *SessionManager) save(s *Session) error {
	s.prune(time.Now(), sm.retention)

	dir := sm.dir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	name := fileName(s.Key)
	fpath := filepath.Join(dir, name)
	if err := sm.checkCollision(name, fpath, s.Key); err != nil {
		return err
	}

	b, err := json.MarshalIndent(sessionFile{Key: s.Key, Number: s.Number, Title: s.Title, Created: s.Created, Messages: s.Messages}, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(fpath, b); err != nil {
		return err
	}

	// Remove the previous file if the session was loaded from a differently
	// named one (e.g. the legacy "<key>.json" naming).
	if old, ok := sm.paths[s.Key]; ok && old != name {
		if err := os.Remove(filepath.Join(dir, old)); err != nil && !os.IsNotExist(err) {
			log.Printf("session: removing old file %s: %v", old, err)
		}
		delete(sm.files, old)
	}
	sm.files[name] = s.Key
	sm.paths[s.Key] = name
	return nil
}

func (sm *SessionManager) saveChats() error {
	if err := os.MkdirAll(sm.dir(), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sm.chats, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(sm.dir(), chatsFile), b)
}

// writeFileAtomic writes to a temp file and renames it into place so a crash
// mid-write never leaves a truncated file behind.
func writeFileAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// checkCollision refuses to write key's session into a file that belongs to a
// different session, whether tracked in memory or created on disk by another
// process since loading.
func (sm *SessionManager) checkCollision(name, fpath, key string) error {
	if owner, ok := sm.files[name]; ok {
		if owner != key {
			return fmt.Errorf("session file %s already belongs to session %q, refusing to overwrite with %q", name, owner, key)
		}
		return nil
	}
	b, err := os.ReadFile(fpath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking session file %s: %w", name, err)
	}
	var f sessionFile
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("session file %s exists but is unreadable, refusing to overwrite: %w", name, err)
	}
	if f.Key != key {
		return fmt.Errorf("session file %s already belongs to session %q, refusing to overwrite with %q", name, f.Key, key)
	}
	return nil
}

func (sm *SessionManager) ensureLoaded() {
	if sm.loaded {
		return
	}
	if err := sm.load(); err != nil {
		log.Printf("session: loading sessions: %v", err)
	}
}

// LoadAll loads persisted sessions from disk. It is called automatically on
// first use; calling it explicitly reloads from disk.
func (sm *SessionManager) LoadAll() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.load()
}

func (sm *SessionManager) load() error {
	sm.loaded = true
	dir := sm.dir()
	_ = os.MkdirAll(dir, 0755)

	if b, err := os.ReadFile(filepath.Join(dir, chatsFile)); err == nil {
		if err := json.Unmarshal(b, &sm.chats); err != nil {
			log.Printf("session: ignoring unreadable %s: %v", chatsFile, err)
		}
	}
	if sm.chats == nil {
		sm.chats = make(map[string]*chatState)
	}
	activeKeys := make(map[string]bool, len(sm.chats))
	for _, c := range sm.chats {
		activeKeys[c.Active] = true
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" || e.Name() == chatsFile || e.Name() == legacyActiveFile {
			continue
		}
		fpath := filepath.Join(dir, e.Name())
		s, err := readSessionFile(fpath)
		if err != nil {
			log.Printf("session: skipping %s: %v", e.Name(), err)
			continue
		}
		// Delete conversations whose whole history has aged out, unless the
		// chat is still using it.
		s.prune(now, sm.retention)
		if len(s.Messages) == 0 && !activeKeys[s.Key] && sm.retention > 0 && now.Sub(s.Created) > sm.retention {
			if err := os.Remove(fpath); err != nil {
				log.Printf("session: removing expired %s: %v", e.Name(), err)
			}
			continue
		}
		sm.files[e.Name()] = s.Key
		if existing, ok := sm.sessions[s.Key]; ok {
			// Same key in two files (e.g. a legacy file next to a migrated one):
			// keep whichever has the most recent activity.
			log.Printf("session: %q found in both %s and %s, keeping the newer", s.Key, sm.paths[s.Key], e.Name())
			if s.LastActivity().Before(existing.LastActivity()) {
				continue
			}
		}
		sm.sessions[s.Key] = s
		sm.paths[s.Key] = e.Name()
	}

	// Never hand out a number already in use, even if the index is stale.
	var unnumbered []*Session
	for _, s := range sm.sessions {
		if s.Number == 0 {
			unnumbered = append(unnumbered, s)
			continue
		}
		c := sm.chat(baseOf(s.Key))
		c.LastNumber = max(c.LastNumber, s.Number)
	}
	if len(unnumbered) == 0 {
		return nil
	}
	// Sessions saved before numbering existed get numbers once, oldest
	// first, and are saved right away so the numbers stick.
	sort.Slice(unnumbered, func(i, j int) bool {
		return unnumbered[i].Created.Before(unnumbered[j].Created)
	})
	for _, s := range unnumbered {
		sm.assignNumber(s)
		if err := sm.save(s); err != nil {
			log.Printf("session: saving number for %q: %v", s.Key, err)
		}
	}
	return sm.saveChats()
}

func readSessionFile(path string) (*Session, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f sessionFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if f.Key == "" {
		return nil, fmt.Errorf("missing session key")
	}
	s := &Session{Key: f.Key, Number: f.Number, Title: f.Title, Created: f.Created, Messages: f.Messages}
	mtime := time.Now()
	if info, err := os.Stat(path); err == nil {
		mtime = info.ModTime()
	}
	if len(f.History) > 0 {
		// Legacy entries carry no timestamps; use the file's mtime so they
		// are not discarded immediately by retention.
		legacy := make([]Message, 0, len(f.History))
		for _, h := range f.History {
			role, content := parseLegacy(h)
			legacy = append(legacy, Message{Role: role, Content: content, Time: mtime})
		}
		s.Messages = append(legacy, s.Messages...)
	}
	if s.Messages == nil {
		s.Messages = make([]Message, 0)
	}
	if s.Created.IsZero() {
		s.Created = mtime
		if len(s.Messages) > 0 {
			s.Created = s.Messages[0].Time
		}
	}
	return s, nil
}

// parseLegacy splits a legacy "role: content" history entry.
func parseLegacy(h string) (string, string) {
	if idx := strings.Index(h, ": "); idx > 0 {
		if r := h[:idx]; r == "user" || r == "assistant" {
			return r, h[idx+2:]
		}
	}
	return "user", h
}

// fileName maps a session key to a unique, filesystem-safe file name: a
// readable prefix derived from the key plus a hash of the exact key, so keys
// that sanitize to the same prefix (or differ only in case on
// case-insensitive filesystems) still get distinct files.
func fileName(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.', r == '@':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 64 {
			break
		}
	}
	sum := sha256.Sum256([]byte(key))
	return b.String() + "-" + hex.EncodeToString(sum[:8]) + ".json"
}

func (s *Session) AddMessage(role, content string) {
	s.Messages = append(s.Messages, Message{Role: role, Content: content, Time: time.Now()})
}

// DisplayTitle returns the conversation's title, falling back to the start of
// its first user message.
func (s *Session) DisplayTitle() string {
	if s.Title != "" {
		return s.Title
	}
	for _, m := range s.Messages {
		if m.Role == "user" {
			t := strings.Join(strings.Fields(m.Content), " ")
			if r := []rune(t); len(r) > 40 {
				t = string(r[:40]) + "…"
			}
			return t
		}
	}
	return "(empty)"
}

// Recent returns the newest messages whose combined content fits within
// maxChars (all messages if maxChars <= 0). The result never starts with an
// assistant turn, since some chat templates require history to open with a
// user message.
func (s *Session) Recent(maxChars int) []Message {
	start := 0
	if maxChars > 0 {
		total := 0
		start = len(s.Messages)
		for start > 0 {
			total += len(s.Messages[start-1].Content)
			if total > maxChars {
				break
			}
			start--
		}
	}
	for start < len(s.Messages) && s.Messages[start].Role != "user" {
		start++
	}
	return s.Messages[start:]
}

// LastActivity returns the time of the newest message, or the creation time
// for an empty conversation.
func (s *Session) LastActivity() time.Time {
	if len(s.Messages) == 0 {
		return s.Created
	}
	return s.Messages[len(s.Messages)-1].Time
}

// prune drops messages older than retention and enforces MaxStoredMessages.
// Messages are stored in chronological order.
func (s *Session) prune(now time.Time, retention time.Duration) {
	start := 0
	if retention > 0 {
		cutoff := now.Add(-retention)
		for start < len(s.Messages) && s.Messages[start].Time.Before(cutoff) {
			start++
		}
	}
	if n := len(s.Messages) - start; n > MaxStoredMessages {
		start += n - MaxStoredMessages
	}
	if start > 0 {
		s.Messages = append([]Message(nil), s.Messages[start:]...)
	}
}
