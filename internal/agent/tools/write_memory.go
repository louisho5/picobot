package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/local/picobot/internal/agent/memory"
)

// heartbeatPhrases contains lowercase substrings that identify heartbeat-status
// log entries. Any write_memory or edit_memory call whose content matches one
// of these is rejected — the heartbeat loop must never pollute memory files.
var heartbeatPhrases = []string{
	"heartbeat check",
	"heartbeat.md",
	"heartbeat log",
	"system status: healthy",
	"no pending tasks",
	"no actions required",
	"periodic tasks",
}

// isHeartbeatContent returns true if content looks like a heartbeat status log.
func isHeartbeatContent(content string) bool {
	l := strings.ToLower(content)
	for _, phrase := range heartbeatPhrases {
		if strings.Contains(l, phrase) {
			return true
		}
	}
	return false
}

// WriteMemoryTool writes to the agent's memory (today's note or long-term MEMORY.md)
type WriteMemoryTool struct {
	mem *memory.MemoryStore
}

func NewWriteMemoryTool(mem *memory.MemoryStore) *WriteMemoryTool {
	return &WriteMemoryTool{mem: mem}
}

func (w *WriteMemoryTool) Name() string { return "write_memory" }
func (w *WriteMemoryTool) Description() string {
	return "Write or append to memory (today's note or long-term MEMORY.md). NEVER store heartbeat status, health checks, or 'no pending tasks' results."
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
	if isHeartbeatContent(content) {
		return "", fmt.Errorf("write_memory: heartbeat status logs must not be stored in memory — skip this write")
	}
	appendFlag := true
	if a, ok := args["append"]; ok {
		if b, ok := a.(bool); ok {
			appendFlag = b
		}
	}

	switch target {
	case "today":
		if err := w.mem.AppendToday(content); err != nil {
			return "", err
		}
		return "appended to today", nil
	case "long":
		if appendFlag {
			prev, err := w.mem.ReadLongTerm()
			if err != nil {
				return "", err
			}
			new := prev + "\n" + content
			if err := w.mem.WriteLongTerm(new); err != nil {
				return "", err
			}
			return "appended to long-term memory", nil
		}
		if err := w.mem.WriteLongTerm(content); err != nil {
			return "", err
		}
		return "wrote long-term memory", nil
	default:
		return "", fmt.Errorf("write_memory: unknown target '%s'", target)
	}
}

