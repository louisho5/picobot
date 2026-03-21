package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// MaxHistorySize is the maximum number of messages kept in a session.
// Older messages are trimmed on save to keep the session file small
// and avoid blowing up the LLM context window.
// Important information should be persisted via write_memory, not session history.
const MaxHistorySize = 50

// Session holds a short chat history.
type Session struct {
	Key     string
	History []string
}

// SessionManager stores sessions in memory and persists to disk under workspace.
type SessionManager struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	workspace string
}

func NewSessionManager(workspace string) *SessionManager {
	return &SessionManager{sessions: make(map[string]*Session), workspace: workspace}
}

func (sm *SessionManager) GetOrCreate(key string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[key]; ok {
		return s
	}
	s := &Session{Key: key, History: make([]string, 0)}
	sm.sessions[key] = s
	return s
}

func (sm *SessionManager) Save(s *Session) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	// Trim history to the most recent messages
	s.trim()
	path := filepath.Join(sm.workspace, "sessions")
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	fpath := filepath.Join(path, s.Key+".json")
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fpath, b, 0644)
}

func (sm *SessionManager) LoadAll() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	path := filepath.Join(sm.workspace, "sessions")
	_ = os.MkdirAll(path, 0755)
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(path, e.Name()))
		if err != nil {
			continue
		}
		var s Session
		if err := json.Unmarshal(b, &s); err != nil {
			continue
		}
		sm.sessions[s.Key] = &s
	}
	return nil
}

func (s *Session) AddMessage(role, content string) {
	s.History = append(s.History, role+": "+content)
}

// GetHistory returns the session history.
func (s *Session) GetHistory() []string {
	return s.History
}

// trim keeps only the last MaxHistorySize messages, discarding the oldest.
func (s *Session) trim() {
	if len(s.History) > MaxHistorySize {
		s.History = s.History[len(s.History)-MaxHistorySize:]
	}
}
