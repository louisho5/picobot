package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/local/picobot/internal/agent/memory"
)

// WriteMemoryTool writes to the agent's memory (today's note or long-term MEMORY.md)
type WriteMemoryTool struct {
	mu  sync.RWMutex
	mem *memory.MemoryStore
}

func NewWriteMemoryTool(mem *memory.MemoryStore) *WriteMemoryTool {
	return &WriteMemoryTool{mem: mem}
}

// SetStore switches the active memory store used by this tool.
func (w *WriteMemoryTool) SetStore(mem *memory.MemoryStore) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.mem = mem
}

func (w *WriteMemoryTool) Name() string { return "write_memory" }
func (w *WriteMemoryTool) Description() string {
	return "Write or append to memory (today's note or long-term MEMORY.md)"
}

func (w *WriteMemoryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Memory target: 'today' for daily note or 'long' for long-term memory",
				"enum":        []string{"today", "long"},
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write or append",
			},
			"append": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, append to existing content; if false, overwrite",
				"default":     true,
			},
		},
		"required": []string{"target", "content"},
	}
}

// Expected args:
// {"target": "today"|"long", "content": "...", "append": true|false }
func (w *WriteMemoryTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	w.mu.RLock()
	mem := w.mem
	w.mu.RUnlock()
	if mem == nil {
		return "", fmt.Errorf("write_memory: memory store is not configured")
	}

	targetI, ok := args["target"]
	if !ok {
		return "", fmt.Errorf("write_memory: 'target' argument required (today|long)")
	}
	target, ok := targetI.(string)
	if !ok {
		return "", fmt.Errorf("write_memory: 'target' must be a string")
	}
	contentI, ok := args["content"]
	if !ok {
		return "", fmt.Errorf("write_memory: 'content' argument required")
	}
	content, ok := contentI.(string)
	if !ok {
		return "", fmt.Errorf("write_memory: 'content' must be a string")
	}
	appendFlag := true
	if a, ok := args["append"]; ok {
		if b, ok := a.(bool); ok {
			appendFlag = b
		}
	}

	switch target {
	case "today":
		if err := mem.AppendToday(content); err != nil {
			return "", err
		}
		return "appended to today", nil
	case "long":
		if appendFlag {
			prev, err := mem.ReadLongTerm()
			if err != nil {
				return "", err
			}
			new := prev + "\n" + content
			if err := mem.WriteLongTerm(new); err != nil {
				return "", err
			}
			return "appended to long-term memory", nil
		}
		if err := mem.WriteLongTerm(content); err != nil {
			return "", err
		}
		return "wrote long-term memory", nil
	default:
		return "", fmt.Errorf("write_memory: unknown target '%s'", target)
	}
}
