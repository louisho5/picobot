package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/local/picobot/internal/chat"
)

func TestSendFileToolExecute(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a dummy file in the temp workspace
	fileName := "test_script.py"
	filePath := filepath.Join(tmpDir, fileName)
	fileContent := "print('hello')"
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		t.Fatalf("failed to create dummy file: %v", err)
	}

	hub := chat.NewHub(10)
	tool := NewSendFileTool(hub, tmpDir)
	tool.SetContext("telegram", "12345")

	// Execute with correct path
	res, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    fileName,
		"caption": "Here is the code",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res != "file sent" {
		t.Errorf("expected result 'file sent', got %q", res)
	}

	// Verify message in hub
	select {
	case out := <-hub.Out:
		if out.Channel != "telegram" {
			t.Errorf("expected channel 'telegram', got %q", out.Channel)
		}
		if out.ChatID != "12345" {
			t.Errorf("expected ChatID '12345', got %q", out.ChatID)
		}
		if out.Content != "Here is the code" {
			t.Errorf("expected caption 'Here is the code', got %q", out.Content)
		}
		if len(out.Media) != 1 || out.Media[0] != filePath {
			t.Errorf("expected media slice with %q, got %v", filePath, out.Media)
		}
	default:
		t.Fatal("expected message in hub.Out")
	}

	// Execute with non-existent path
	_, err = tool.Execute(context.Background(), map[string]interface{}{
		"path": "does_not_exist.py",
	})
	if err == nil {
		t.Error("expected error for non-existent file path")
	}
}
