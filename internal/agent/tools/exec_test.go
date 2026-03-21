package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExecArrayEcho(t *testing.T) {
	e := NewExecTool(2)
	out, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"echo", "hello"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "hello" {
		t.Fatalf("unexpected out: %s", out)
	}
}

func TestExecStringDisallowed(t *testing.T) {
	e := NewExecTool(2)
	_, err := e.Execute(context.Background(), map[string]interface{}{"cmd": "ls -la"})
	if err == nil {
		t.Fatalf("expected error for string command")
	}
}

func TestExecDangerousProgRejected(t *testing.T) {
	e := NewExecTool(2)
	_, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"rm", "-rf", "/"}})
	if err == nil {
		t.Fatalf("expected error for dangerous program")
	}
}

func TestExecWithWorkspace(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "file.txt")
	if err := os.WriteFile(f, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	e := NewExecToolWithWorkspace(2, d)
	out, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"cat", "file.txt"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "content" {
		t.Fatalf("unexpected out: %s", out)
	}
}

func TestExecRejectsUnsafeArg(t *testing.T) {
	e := NewExecTool(2)
	_, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"ls", "/etc"}})
	if err == nil {
		t.Fatalf("expected error for absolute path arg")
	}
}

func TestExecTimeout(t *testing.T) {
	e := NewExecTool(1)
	_, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"sleep", "2"}})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
