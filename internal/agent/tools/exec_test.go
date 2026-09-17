package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/local/picobot/internal/chat"
)

func TestExecArrayEcho(t *testing.T) {
	e := NewExecTool(nil, 2)
	out, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"echo", "hello"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "hello" {
		t.Fatalf("unexpected out: %s", out)
	}
}

func TestExecStringDisallowed(t *testing.T) {
	e := NewExecTool(nil, 2)
	_, err := e.Execute(context.Background(), map[string]interface{}{"cmd": "ls -la"})
	if err == nil {
		t.Fatalf("expected error for string command")
	}
}

func TestExecDangerousProgRejected(t *testing.T) {
	e := NewExecTool(nil, 2)
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
	e := NewExecToolWithWorkspace(nil, 2, d)
	out, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"cat", "file.txt"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "content" {
		t.Fatalf("unexpected out: %s", out)
	}
}

func TestExecRejectsUnsafeArg(t *testing.T) {
	e := NewExecTool(nil, 2)
	_, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"ls", "/etc"}})
	if err == nil {
		t.Fatalf("expected error for absolute path arg")
	}
}

func TestExecTimeout(t *testing.T) {
	e := NewExecTool(nil, 1)
	_, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"sleep", "2"}})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestExecAllowsSafeAbsoluteArg(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "file.txt")
	if err := os.WriteFile(f, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	e := NewExecToolWithWorkspace(nil, 2, d)
	out, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"cat", f}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "content" {
		t.Fatalf("unexpected out: %s", out)
	}
}

func TestExecShellCmdApproval(t *testing.T) {
	hub := chat.NewHub(10)
	e := NewExecTool(hub, 2)
	e.SetContext("telegram", "456")

	ctx := context.Background()
	done := make(chan struct{})
	var out string
	var err error

	go func() {
		out, err = e.Execute(ctx, map[string]interface{}{"shell_cmd": "echo hello_shell"})
		close(done)
	}()

	// Wait for the approval message to hit hub.Out
	select {
	case msg := <-hub.Out:
		if !strings.Contains(msg.Content, "Approval Required") {
			t.Fatalf("expected approval request message, got %v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for approval request")
	}

	// Trigger the approval response
	key := "telegram:456"
	if !chat.TriggerApproval(key, "yes") {
		t.Fatal("failed to trigger approval")
	}

	<-done
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "hello_shell" {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestExecCmdArrayApproval(t *testing.T) {
	hub := chat.NewHub(10)
	e := NewExecTool(hub, 2)
	e.SetContext("telegram", "456")

	ctx := context.Background()
	done := make(chan struct{})
	var out string
	var err error

	go func() {
		out, err = e.Execute(ctx, map[string]interface{}{"cmd": []interface{}{"echo", "hello_cmd"}})
		close(done)
	}()

	// Wait for the approval message to hit hub.Out
	select {
	case msg := <-hub.Out:
		if !strings.Contains(msg.Content, "Approval Required") {
			t.Fatalf("expected approval request message, got %v", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for approval request")
	}

	// Trigger the approval response
	key := "telegram:456"
	if !chat.TriggerApproval(key, "yes") {
		t.Fatal("failed to trigger approval")
	}

	<-done
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out != "hello_cmd" {
		t.Fatalf("unexpected output: %s", out)
	}
}
