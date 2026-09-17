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

func TestFilesystemPlanApproval(t *testing.T) {
	tmpDir := t.TempDir()
	hub := chat.NewHub(10)

	fs, err := NewFilesystemTool(hub, tmpDir)
	if err != nil {
		t.Fatalf("failed to create filesystem tool: %v", err)
	}
	defer fs.Close()

	// 1. Normal file write should bypass approval
	ctx := chat.WithContext(context.Background(), "telegram", "123")
	res, err := fs.Execute(ctx, map[string]interface{}{
		"action":  "write",
		"path":    "normal.txt",
		"content": "hello world",
	})
	if err != nil {
		t.Fatalf("expected no error writing normal file, got %v", err)
	}
	if res != "written" {
		t.Errorf("expected res 'written', got %q", res)
	}

	// Verify normal.txt exists
	if _, err := os.Stat(filepath.Join(tmpDir, "normal.txt")); err != nil {
		t.Errorf("expected normal.txt to exist, got error: %v", err)
	}

	// 2. PLAN.md write with 'yes' approval should succeed
	doneYes := make(chan struct{})
	var resYes string
	var errYes error
	go func() {
		resYes, errYes = fs.Execute(ctx, map[string]interface{}{
			"action":  "write",
			"path":    "PLAN.md",
			"content": "# Objective: Do X",
		})
		close(doneYes)
	}()

	// Wait for outbound message to hit hub
	select {
	case out := <-hub.Out:
		if !strings.Contains(out.Content, "Proposed Plan Update") {
			t.Errorf("expected proposed plan message, got %q", out.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for plan approval request")
	}

	// Trigger approval 'yes'
	if !chat.TriggerApproval("telegram:123", "yes") {
		t.Fatal("failed to trigger yes approval")
	}

	<-doneYes
	if errYes != nil {
		t.Fatalf("expected no error, got %v", errYes)
	}
	if resYes != "written" {
		t.Errorf("expected res 'written' for plan approval, got %q", resYes)
	}

	// Verify PLAN.md exists and has correct content
	b, err := os.ReadFile(filepath.Join(tmpDir, "PLAN.md"))
	if err != nil {
		t.Fatalf("failed to read PLAN.md: %v", err)
	}
	if string(b) != "# Objective: Do X" {
		t.Errorf("unexpected PLAN.md content: %q", string(b))
	}

	// 3. PLAN.md write with 'no' rejection should return rejection message and not update the file
	doneNo := make(chan struct{})
	var resNo string
	var errNo error
	go func() {
		resNo, errNo = fs.Execute(ctx, map[string]interface{}{
			"action":  "write",
			"path":    "PLAN.md",
			"content": "# Objective: Do Y",
		})
		close(doneNo)
	}()

	select {
	case <-hub.Out:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for second plan approval request")
	}

	// Trigger rejection 'no'
	if !chat.TriggerApproval("telegram:123", "no") {
		t.Fatal("failed to trigger no approval")
	}

	<-doneNo
	if errNo != nil {
		t.Fatalf("expected no error, got %v", errNo)
	}
	if !strings.Contains(resNo, "Plan rejected by user") {
		t.Errorf("expected 'Plan rejected by user', got %q", resNo)
	}

	// Verify PLAN.md is NOT updated
	b, _ = os.ReadFile(filepath.Join(tmpDir, "PLAN.md"))
	if string(b) != "# Objective: Do X" {
		t.Errorf("PLAN.md was incorrectly updated to %q", string(b))
	}

	// 4. PLAN.md write with custom feedback should return comment text and not update the file
	doneComment := make(chan struct{})
	var resComment string
	var errComment error
	go func() {
		resComment, errComment = fs.Execute(ctx, map[string]interface{}{
			"action":  "write",
			"path":    "PLAN.md",
			"content": "# Objective: Do Y",
		})
		close(doneComment)
	}()

	select {
	case <-hub.Out:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for third plan approval request")
	}

	// Trigger custom comment response
	comment := "No, let's use python instead of go"
	if !chat.TriggerApproval("telegram:123", comment) {
		t.Fatal("failed to trigger comment approval")
	}

	<-doneComment
	if errComment != nil {
		t.Fatalf("expected no error, got %v", errComment)
	}
	if !strings.Contains(resComment, "Plan rejected/modified by user with comment") || !strings.Contains(resComment, comment) {
		t.Errorf("expected result to contain comment feedback, got %q", resComment)
	}

	// Verify PLAN.md is still NOT updated
	b, _ = os.ReadFile(filepath.Join(tmpDir, "PLAN.md"))
	if string(b) != "# Objective: Do X" {
		t.Errorf("PLAN.md was incorrectly updated to %q", string(b))
	}
}
