package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/providers"
)

func TestNewAgentLoop_ConfiguresExecToolToWorkspace(t *testing.T) {
	ws := t.TempDir()
	hub := chat.NewHub(1)
	provider := providers.NewStubProvider()

	ag := NewAgentLoop(hub, provider, provider.GetDefaultModel(), 3, ws, nil)

	execTool := ag.tools.Get("exec")
	if execTool == nil {
		t.Fatalf("exec tool not registered")
	}

	out, err := execTool.Execute(context.Background(), map[string]interface{}{
		"cmd": []interface{}{"pwd"},
	})
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	if filepath.Clean(out) != filepath.Clean(ws) {
		t.Fatalf("exec working dir mismatch: got %q want %q", out, ws)
	}
}
