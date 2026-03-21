package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MemoryItem is a stored memory entry.
// Kind is "short" or "long". Timestamp is in UTC.
type MemoryItem struct {
	Kind      string
	Text      string
	Timestamp time.Time
}

// MemoryStore is a minimal in-memory memory system with simple query capabilities.
// - Long-term: append-only list (persisted in a real implementation)
// - Short-term: append-only list with a configurable limit (recent items kept)
// This is intentionally simple for v0 and unit-testable.
type MemoryStore struct {
	workspace string // workspace root (used for disk-backed memory)
	memoryDir string // workspace/memory/
	limit     int    // max short-term items to keep
	long      []MemoryItem
	short     []MemoryItem
	mu        sync.RWMutex
}

// NewMemoryStore creates an in-memory store with short-term limit (e.g., 100).
// Kept for tests and simple use-cases.
func NewMemoryStore(limit int) *MemoryStore {
	return NewMemoryStoreWithWorkspace(".", limit)
}

// NewMemoryStoreWithWorkspace creates a MemoryStore backed by files under workspace/memory/.
func NewMemoryStoreWithWorkspace(workspace string, limit int) *MemoryStore {
	if limit <= 0 {
		limit = 100
	}
	ms := &MemoryStore{
		workspace: workspace,
		memoryDir: workspace + "/memory",
		short:     make([]MemoryItem, 0, limit),
		long:      make([]MemoryItem, 0),
		limit:     limit,
	}
	// ensure memory directory exists
	_ = os.MkdirAll(ms.memoryDir, 0o755)
	return ms
}

// AddShort adds a short-term memory entry.
func (s *MemoryStore) AddShort(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it := MemoryItem{Timestamp: time.Now().UTC(), Text: text, Kind: "short"}
	s.short = append(s.short, it)
	// drop oldest if over limit
	if len(s.short) > s.limit {
		s.short = s.short[len(s.short)-s.limit:]
	}
}

// AddLong adds a long-term memory entry.
func (s *MemoryStore) AddLong(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it := MemoryItem{Timestamp: time.Now().UTC(), Text: text, Kind: "long"}
	s.long = append(s.long, it)
}

// Recent returns up to n most recent memory items, combining short and long (short first).
// Items are returned in most-recent-first order.
func (s *MemoryStore) Recent(n int) []MemoryItem {
	if n <= 0 {
		return nil
	}
	out := make([]MemoryItem, 0, n)
	s.mu.RLock()
	defer s.mu.RUnlock()
	// take from short (newest first)
	for i := len(s.short) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, s.short[i])
	}
	// then from long (newest first)
	for i := len(s.long) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, s.long[i])
	}
	return out
}

// QueryByKeyword returns up to n items containing the keyword (case-insensitive) in most-recent-first order.
// Preference follows Recent ordering: short (newest first) then long (newest first).
func (s *MemoryStore) QueryByKeyword(keyword string, n int) []MemoryItem {
	if n <= 0 || keyword == "" {
		return nil
	}
	k := strings.ToLower(keyword)
	out := make([]MemoryItem, 0, n)
	s.mu.RLock()
	defer s.mu.RUnlock()
	// scan short (newest first)
	for i := len(s.short) - 1; i >= 0 && len(out) < n; i-- {
		if strings.Contains(strings.ToLower(s.short[i].Text), k) {
			out = append(out, s.short[i])
		}
	}
	// then long (newest first)
	for i := len(s.long) - 1; i >= 0 && len(out) < n; i-- {
		if strings.Contains(strings.ToLower(s.long[i].Text), k) {
			out = append(out, s.long[i])
		}
	}
	return out
}

// ReadLongTerm reads the long-term MEMORY.md file under workspace/memory/MEMORY.md
func (s *MemoryStore) ReadLongTerm() (string, error) {
	path := filepath.Join(s.memoryDir, "MEMORY.md")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

// WriteLongTerm writes content to MEMORY.md (overwrites).
func (s *MemoryStore) WriteLongTerm(content string) error {
	if err := os.MkdirAll(s.memoryDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.memoryDir, "MEMORY.md")
	return os.WriteFile(path, []byte(content), 0o644)
}

// ReadToday reads today's memory note file (YYYY-MM-DD.md)
func (s *MemoryStore) ReadToday() (string, error) {
	name := time.Now().UTC().Format("2006-01-02") + ".md"
	path := filepath.Join(s.memoryDir, name)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

// AppendToday appends a line (with timestamp) to today's memory note file.
func (s *MemoryStore) AppendToday(text string) error {
	if err := os.MkdirAll(s.memoryDir, 0o755); err != nil {
		return err
	}
	name := time.Now().UTC().Format("2006-01-02") + ".md"
	path := filepath.Join(s.memoryDir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintf(f, "[%s] %s\n", time.Now().UTC().Format(time.RFC3339), text)
	return err
}

// GetRecentMemories reads last N days' files and joins them with separators.
func (s *MemoryStore) GetRecentMemories(days int) (string, error) {
	if days <= 0 {
		days = 1
	}
	parts := make([]string, 0, days)
	for i := 0; i < days; i++ {
		d := time.Now().UTC().AddDate(0, 0, -i)
		name := d.Format("2006-01-02") + ".md"
		path := filepath.Join(s.memoryDir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		parts = append(parts, string(b))
	}
	return strings.Join(parts, "\n---\n"), nil
}

// isValidMemoryFile reports whether name is a safe, recognised memory filename
// (either "MEMORY.md" or a date file "YYYY-MM-DD.md").
func isValidMemoryFile(name string) bool {
	if name == "MEMORY.md" {
		return true
	}
	// Must be exactly "YYYY-MM-DD.md" (13 chars).
	if len(name) != 13 || name[4] != '-' || name[7] != '-' || name[10:] != ".md" {
		return false
	}
	_, err := time.Parse("2006-01-02", name[:10])
	return err == nil
}

// ListFiles returns the filenames of all files in the memory directory.
func (s *MemoryStore) ListFiles() ([]string, error) {
	entries, err := os.ReadDir(s.memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// ReadFile reads a named file from the memory directory.
// name must be "MEMORY.md" or a date file "YYYY-MM-DD.md".
// Returns ("", nil) if the file does not exist.
func (s *MemoryStore) ReadFile(name string) (string, error) {
	if !isValidMemoryFile(name) {
		return "", fmt.Errorf("invalid memory filename: %q", name)
	}
	b, err := os.ReadFile(filepath.Join(s.memoryDir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

// WriteFile writes content to a named file in the memory directory.
// name must be "MEMORY.md" or a date file "YYYY-MM-DD.md".
func (s *MemoryStore) WriteFile(name, content string) error {
	if !isValidMemoryFile(name) {
		return fmt.Errorf("invalid memory filename: %q", name)
	}
	if err := os.MkdirAll(s.memoryDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.memoryDir, name), []byte(content), 0o644)
}

// DeleteFile deletes a dated memory file (YYYY-MM-DD.md only).
// Long-term memory (MEMORY.md) is protected and cannot be deleted via this method.
func (s *MemoryStore) DeleteFile(name string) error {
	// Only dated files may be deleted — never MEMORY.md.
	if len(name) != 13 || name[4] != '-' || name[7] != '-' || name[10:] != ".md" {
		return fmt.Errorf("delete_memory: only dated files (YYYY-MM-DD) can be deleted, got %q", name)
	}
	if _, err := time.Parse("2006-01-02", name[:10]); err != nil {
		return fmt.Errorf("delete_memory: only dated files (YYYY-MM-DD) can be deleted, got %q", name)
	}
	if err := os.Remove(filepath.Join(s.memoryDir, name)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("memory file not found: %q", name)
		}
		return err
	}
	return nil
}

// GetMemoryContext returns combined long-term memory + today's notes for the system prompt.
func (s *MemoryStore) GetMemoryContext() (string, error) {
	lt, err := s.ReadLongTerm()
	if err != nil {
		return "", err
	}
	td, err := s.ReadToday()
	if err != nil {
		return "", err
	}
	if lt == "" && td == "" {
		return "", nil
	}
	if lt == "" {
		return td, nil
	}
	if td == "" {
		return lt, nil
	}
	return lt + "\n\n---\n\n" + td, nil
}
