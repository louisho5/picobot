package heartbeat

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/local/picobot/internal/chat"
)

var htmlCommentRegex = regexp.MustCompile(`(?s)<!--.*?-->`)

// StartHeartbeat starts a periodic check that reads HEARTBEAT.md and pushes
// its content into the agent's inbound chat hub for processing.
func StartHeartbeat(ctx context.Context, workspace string, interval time.Duration, hub *chat.Hub) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		log.Printf("heartbeat: started (every %v)", interval)
		for {
			select {
			case <-ctx.Done():
				log.Println("heartbeat: stopping")
				return
			case <-ticker.C:
				path := filepath.Join(workspace, "HEARTBEAT.md")
				data, err := os.ReadFile(path)
				if err != nil {
					// file doesn't exist or can't be read — skip silently
					continue
				}
				content := strings.TrimSpace(string(data))
				if content == "" {
					continue
				}

				if !hasActiveTasks(content) {
					// No active tasks in HEARTBEAT.md — skip to avoid continuous LLM polling
					continue
				}

				// Push heartbeat content into the agent loop for processing
				log.Println("heartbeat: sending tasks to agent")
				hub.In <- chat.Inbound{
					Channel:  "heartbeat",
					ChatID:   "system",
					SenderID: "heartbeat",
					Content:  "[HEARTBEAT CHECK] Review and execute any pending tasks from HEARTBEAT.md:\n\n" + content,
				}
			}
		}
	}()
}

// hasActiveTasks checks if HEARTBEAT.md contains actual uncommented tasks.
func hasActiveTasks(content string) bool {
	// 1. Strip HTML comments
	cleaned := htmlCommentRegex.ReplaceAllString(content, "")

	// 2. Split into lines
	lines := strings.Split(cleaned, "\n")

	// We want to identify the tasks section if present.
	// If the headers exist, we prioritize looking specifically under the tasks section.
	hasTasksHeader := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.ToLower(line))
		if trimmed == "## periodic tasks" || trimmed == "## tasks" {
			hasTasksHeader = true
			break
		}
	}

	if hasTasksHeader {
		inTasksSection := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			trimmedLower := strings.ToLower(trimmed)
			if trimmedLower == "## periodic tasks" || trimmedLower == "## tasks" {
				inTasksSection = true
				continue
			}
			if inTasksSection {
				// If we encounter another header, we exit the tasks section
				if strings.HasPrefix(trimmed, "#") {
					inTasksSection = false
					continue
				}
				// Check if line has alphanumeric characters
				if hasAlphanumeric(trimmed) {
					return true
				}
			}
		}
		return false
	}

	// Fallback: If there is no explicit tasks header, check all lines,
	// but ignore known template lines/instructions.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmedLower := strings.ToLower(trimmed)

		// Skip rules header and template paragraphs
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmedLower, "this file is checked periodically") ||
			strings.Contains(trimmedLower, "add tasks here that should run") ||
			strings.Contains(trimmedLower, "after reviewing this file, take actions only") ||
			strings.Contains(trimmedLower, "if there are no tasks") ||
			strings.Contains(trimmedLower, "never log \"heartbeat check complete\"") ||
			strings.Contains(trimmedLower, "heartbeat results are ephemeral") {
			continue
		}

		if hasAlphanumeric(trimmed) {
			return true
		}
	}

	return false
}

func hasAlphanumeric(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
