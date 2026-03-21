package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/picobot/internal/agent/memory"
)

func TestWriteMemoryTool_TodayAndLong(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)
	w := NewWriteMemoryTool(mem)

	// append to today
	if _, err := w.Execute(context.Background(), map[string]interface{}{"target": "today", "content": "note A"}); err != nil {
		t.Fatalf("expected no error appending today, got %v", err)
	}
	files, _ := os.ReadDir(filepath.Join(tmp, "memory"))
	if len(files) == 0 {
		t.Fatalf("expected file in memory dir")
	}

	// append to long-term
	if _, err := w.Execute(context.Background(), map[string]interface{}{"target": "long", "content": "LT1", "append": true}); err != nil {
		t.Fatalf("expected no error appending long, got %v", err)
	}
	lt, err := mem.ReadLongTerm()
	if err != nil {
		t.Fatalf("ReadLongTerm error: %v", err)
	}
	if lt == "" || !strings.Contains(lt, "LT1") {
		t.Fatalf("expected LT1 in long-term memory, got %q", lt)
	}

	// overwrite long-term
	if _, err := w.Execute(context.Background(), map[string]interface{}{"target": "long", "content": "FULL", "append": false}); err != nil {
		t.Fatalf("expected no error writing long, got %v", err)
	}
	lt2, _ := mem.ReadLongTerm()
	if strings.Contains(lt2, "LT1") {
		t.Fatalf("expected LT1 to be gone after overwrite, got %q", lt2)
	}
}

func TestWriteMemoryTool_RejectsHeartbeatContent(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)
	w := NewWriteMemoryTool(mem)

	cases := []string{
		"HEARTBEAT CHECK: system status: HEALTHY",
		"Reviewed HEARTBEAT.md. No pending tasks. System status: HEALTHY. No actions required.",
		"heartbeat check complete",
		"[2026-03-07T00:12:36Z] HEARTBEAT CHECK at 2026-03-06: no pending tasks",
	}
	for _, c := range cases {
		_, err := w.Execute(context.Background(), map[string]interface{}{"target": "today", "content": c})
		if err == nil {
			t.Fatalf("expected heartbeat content to be rejected, but it was accepted: %q", c)
		}
	}

	// Confirm that legitimate content is still accepted.
	_, err := w.Execute(context.Background(), map[string]interface{}{"target": "today", "content": "user prefers dark mode"})
	if err != nil {
		t.Fatalf("expected legitimate content to be accepted, got: %v", err)
	}
}
