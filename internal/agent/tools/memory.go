package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/local/picobot/internal/agent/memory"
)

// resolveMemoryTarget maps a user-friendly target to a memory filename.
// target: "today", "long", or "YYYY-MM-DD".
func resolveMemoryTarget(target string) (string, error) {
	switch target {
	case "today":
		return time.Now().UTC().Format("2006-01-02") + ".md", nil
	case "long":
		return "MEMORY.md", nil
	default:
		if _, err := time.Parse("2006-01-02", target); err == nil {
			return target + ".md", nil
		}
		return "", fmt.Errorf("invalid target %q: use 'today', 'long', or 'YYYY-MM-DD'", target)
	}
}

// ─── list_memory ────

// ListMemoryTool lists all files in the agent's memory directory.
type ListMemoryTool struct {
	mem *memory.MemoryStore
}

func NewListMemoryTool(mem *memory.MemoryStore) *ListMemoryTool {
	return &ListMemoryTool{mem: mem}
}

func (t *ListMemoryTool) Name() string { return "list_memory" }
func (t *ListMemoryTool) Description() string {
	return "List all memory files (daily notes and long-term memory)"
}
func (t *ListMemoryTool) Parameters() map[string]interface{} { return nil }

func (t *ListMemoryTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	files, err := t.mem.ListFiles()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "No memory files found.", nil
	}
	today := time.Now().UTC().Format("2006-01-02") + ".md"
	var sb strings.Builder
	fmt.Fprintf(&sb, "Memory files (%d):\n", len(files))
	for _, f := range files {
		switch f {
		case "MEMORY.md":
			fmt.Fprintf(&sb, "- %s (long-term)\n", f)
		case today:
			fmt.Fprintf(&sb, "- %s (today)\n", f)
		default:
			fmt.Fprintf(&sb, "- %s\n", f)
		}
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// ─── read_memory ────

// ReadMemoryTool reads the contents of a specific memory file.
type ReadMemoryTool struct {
	mem *memory.MemoryStore
}

func NewReadMemoryTool(mem *memory.MemoryStore) *ReadMemoryTool {
	return &ReadMemoryTool{mem: mem}
}

func (t *ReadMemoryTool) Name() string        { return "read_memory" }
func (t *ReadMemoryTool) Description() string { return "Read the contents of a memory file" }
func (t *ReadMemoryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target": map[string]interface{}{
				"type":        "string",
				"description": "'today' for today's note, 'long' for long-term memory, or a date 'YYYY-MM-DD'",
			},
		},
		"required": []string{"target"},
	}
}

func (t *ReadMemoryTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	target, ok := args["target"].(string)
	if !ok || target == "" {
		return "", fmt.Errorf("read_memory: 'target' argument required (today|long|YYYY-MM-DD)")
	}
	name, err := resolveMemoryTarget(target)
	if err != nil {
		return "", err
	}
	content, err := t.mem.ReadFile(name)
	if err != nil {
		return "", err
	}
	if content == "" {
		return fmt.Sprintf("(%s is empty or does not exist)", name), nil
	}
	return content, nil
}

// ─── edit_memory ────

// EditMemoryTool finds and replaces text within a memory file.
type EditMemoryTool struct {
	mem *memory.MemoryStore
}

func NewEditMemoryTool(mem *memory.MemoryStore) *EditMemoryTool {
	return &EditMemoryTool{mem: mem}
}

func (t *EditMemoryTool) Name() string        { return "edit_memory" }
func (t *EditMemoryTool) Description() string { return "Find and replace text within a memory file" }
func (t *EditMemoryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target": map[string]interface{}{
				"type":        "string",
				"description": "'today', 'long', or 'YYYY-MM-DD'",
			},
			"old_text": map[string]interface{}{
				"type":        "string",
				"description": "Exact text to find and replace",
			},
			"new_text": map[string]interface{}{
				"type":        "string",
				"description": "Replacement text (omit or set to empty string to delete the matched text)",
			},
		},
		"required": []string{"target", "old_text"},
	}
}

func (t *EditMemoryTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	target, ok := args["target"].(string)
	if !ok || target == "" {
		return "", fmt.Errorf("edit_memory: 'target' argument required (today|long|YYYY-MM-DD)")
	}
	oldText, ok := args["old_text"].(string)
	if !ok || oldText == "" {
		return "", fmt.Errorf("edit_memory: 'old_text' argument required")
	}
	newText, _ := args["new_text"].(string) // defaults to "" (deletion) if absent
	if isHeartbeatContent(newText) {
		// return "", fmt.Errorf("edit_memory: heartbeat status logs must not be stored in memory — skip this edit")
		return "", nil // skip sliently
	}

	name, err := resolveMemoryTarget(target)
	if err != nil {
		return "", err
	}
	content, err := t.mem.ReadFile(name)
	if err != nil {
		return "", err
	}
	if !strings.Contains(content, oldText) {
		return "", fmt.Errorf("edit_memory: text not found in %s", name)
	}
	updated := strings.ReplaceAll(content, oldText, newText)
	if err := t.mem.WriteFile(name, updated); err != nil {
		return "", err
	}
	return fmt.Sprintf("edited %s", name), nil
}

// ─── delete_memory ────

// DeleteMemoryTool deletes a dated daily memory file.
// Long-term memory (MEMORY.md) will be protected.
type DeleteMemoryTool struct {
	mem *memory.MemoryStore
}

func NewDeleteMemoryTool(mem *memory.MemoryStore) *DeleteMemoryTool {
	return &DeleteMemoryTool{mem: mem}
}

func (t *DeleteMemoryTool) Name() string { return "delete_memory" }
func (t *DeleteMemoryTool) Description() string {
	return "Delete a daily memory file (YYYY-MM-DD). Long-term memory (MEMORY.md) cannot be deleted this way."
}
func (t *DeleteMemoryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Date of the daily note to delete, in 'YYYY-MM-DD' format",
			},
		},
		"required": []string{"target"},
	}
}

func (t *DeleteMemoryTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	target, ok := args["target"].(string)
	if !ok || target == "" {
		return "", fmt.Errorf("delete_memory: 'target' argument required (YYYY-MM-DD)")
	}
	// Only dated files are accepted — "long" / "today" are rejected here.
	if _, err := time.Parse("2006-01-02", target); err != nil {
		return "", fmt.Errorf("delete_memory: target must be a date in YYYY-MM-DD format, got %q", target)
	}
	if err := t.mem.DeleteFile(target + ".md"); err != nil {
		return "", err
	}
	return fmt.Sprintf("deleted %s.md", target), nil
}
