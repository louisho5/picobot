package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/picobot/internal/agent/memory"
)

// ─── list_memory ────

func TestListMemoryTool_Empty(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)
	tool := NewListMemoryTool(mem)

	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No memory") {
		t.Fatalf("expected 'No memory' message, got %q", out)
	}
}

func TestListMemoryTool_WithFiles(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)

	if err := mem.WriteLongTerm("hello"); err != nil {
		t.Fatal(err)
	}
	if err := mem.AppendToday("note"); err != nil {
		t.Fatal(err)
	}

	tool := NewListMemoryTool(mem)
	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "MEMORY.md") {
		t.Fatalf("expected MEMORY.md in output, got %q", out)
	}
	if !strings.Contains(out, "(long-term)") {
		t.Fatalf("expected (long-term) label, got %q", out)
	}
	if !strings.Contains(out, "(today)") {
		t.Fatalf("expected (today) label, got %q", out)
	}
}

// ─── read_memory ────

func TestReadMemoryTool_Long(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)

	if err := mem.WriteLongTerm("remember everything"); err != nil {
		t.Fatal(err)
	}

	tool := NewReadMemoryTool(mem)
	out, err := tool.Execute(context.Background(), map[string]interface{}{"target": "long"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "remember everything" {
		t.Fatalf("unexpected content: %q", out)
	}
}

func TestReadMemoryTool_Empty(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)
	tool := NewReadMemoryTool(mem)

	out, err := tool.Execute(context.Background(), map[string]interface{}{"target": "long"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "empty") {
		t.Fatalf("expected 'empty' message, got %q", out)
	}
}

func TestReadMemoryTool_ByDate(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)

	// write a dated file directly
	date := "2026-01-15"
	if err := os.WriteFile(filepath.Join(tmp, "memory", date+".md"), []byte("old note"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadMemoryTool(mem)
	out, err := tool.Execute(context.Background(), map[string]interface{}{"target": date})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "old note" {
		t.Fatalf("unexpected content: %q", out)
	}
}

func TestReadMemoryTool_InvalidTarget(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)
	tool := NewReadMemoryTool(mem)

	_, err := tool.Execute(context.Background(), map[string]interface{}{"target": "bad-input"})
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
}

// ─── edit_memory ────

func TestEditMemoryTool_Replace(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)

	if err := mem.WriteLongTerm("hello world, hello go"); err != nil {
		t.Fatal(err)
	}

	tool := NewEditMemoryTool(mem)
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"target":   "long",
		"old_text": "hello",
		"new_text": "hi",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, _ := mem.ReadLongTerm()
	if content != "hi world, hi go" {
		t.Fatalf("unexpected content after edit: %q", content)
	}
}

func TestEditMemoryTool_Delete(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)

	if err := mem.WriteLongTerm("line1\nredundant\nline3"); err != nil {
		t.Fatal(err)
	}

	tool := NewEditMemoryTool(mem)
	// omitting new_text deletes the matched text
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"target":   "long",
		"old_text": "\nredundant",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, _ := mem.ReadLongTerm()
	if strings.Contains(content, "redundant") {
		t.Fatalf("expected 'redundant' to be removed, got %q", content)
	}
}

func TestEditMemoryTool_TextNotFound(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)

	if err := mem.WriteLongTerm("hello world"); err != nil {
		t.Fatal(err)
	}

	tool := NewEditMemoryTool(mem)
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"target":   "long",
		"old_text": "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error when text is not found")
	}
}

// ─── delete_memory ────

func TestDeleteMemoryTool(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)

	date := "2026-01-15"
	memDir := filepath.Join(tmp, "memory")
	if err := os.WriteFile(filepath.Join(memDir, date+".md"), []byte("old note"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewDeleteMemoryTool(mem)
	out, err := tool.Execute(context.Background(), map[string]interface{}{"target": date})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, date) {
		t.Fatalf("expected date in confirmation message, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(memDir, date+".md")); !os.IsNotExist(err) {
		t.Fatal("expected file to be deleted but it still exists")
	}
}

func TestDeleteMemoryTool_RejectsLong(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)
	tool := NewDeleteMemoryTool(mem)

	_, err := tool.Execute(context.Background(), map[string]interface{}{"target": "long"})
	if err == nil {
		t.Fatal("expected error when attempting to delete long-term memory")
	}
}

func TestDeleteMemoryTool_RejectsToday(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)
	tool := NewDeleteMemoryTool(mem)

	_, err := tool.Execute(context.Background(), map[string]interface{}{"target": "today"})
	if err == nil {
		t.Fatal("expected error for 'today' shorthand (must use explicit date)")
	}
}

func TestDeleteMemoryTool_NotFound(t *testing.T) {
	tmp := t.TempDir()
	mem := memory.NewMemoryStoreWithWorkspace(tmp, 10)
	tool := NewDeleteMemoryTool(mem)

	_, err := tool.Execute(context.Background(), map[string]interface{}{"target": "2020-01-01"})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
