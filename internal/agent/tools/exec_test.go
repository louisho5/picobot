package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	os.WriteFile(f, []byte("content"), 0644)
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

func TestExecRejectsProgramPathByDefault(t *testing.T) {
	e := NewExecTool(2)
	_, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"./script.sh"}})
	if err == nil {
		t.Fatalf("expected program path rejection")
	}
	if !strings.Contains(err.Error(), "program path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecRejectsNonAllowlistedProgramByDefault(t *testing.T) {
	e := NewExecTool(2)
	_, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"sh", "-c", "echo hi"}})
	if err == nil {
		t.Fatalf("expected non-allowlisted program rejection")
	}
	if !strings.Contains(err.Error(), "safe allowlist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecUnsafeOverrideAllowsNonAllowlistedProgram(t *testing.T) {
	t.Setenv("PICOBOT_EXEC_ALLOW_UNSAFE", "1")
	e := NewExecTool(2)
	out, err := e.Execute(context.Background(), map[string]interface{}{"cmd": []interface{}{"sh", "-c", "echo hi"}})
	if err != nil {
		t.Fatalf("expected command to pass with unsafe override: %v", err)
	}
	if out != "hi" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecRejectsGitAliasBypassByDefault(t *testing.T) {
	e := NewExecTool(2)
	_, err := e.Execute(context.Background(), map[string]interface{}{
		"cmd": []interface{}{"git", "-c", "alias.pwn=!echo bypassed", "pwn"},
	})
	if err == nil {
		t.Fatalf("expected git alias bypass payload to be rejected")
	}
	if !strings.Contains(err.Error(), "safe allowlist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecRejectsFindExecBypassByDefault(t *testing.T) {
	e := NewExecTool(2)
	_, err := e.Execute(context.Background(), map[string]interface{}{
		"cmd": []interface{}{"find", ".", "-maxdepth", "0", "-exec", "sh", "-c", "echo via_find", ";"},
	})
	if err == nil {
		t.Fatalf("expected find -exec payload to be rejected")
	}
	if !strings.Contains(err.Error(), "safe allowlist") {
		t.Fatalf("unexpected error: %v", err)
	}
}
