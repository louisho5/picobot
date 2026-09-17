package agent

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/local/picobot/internal/agent/memory"
	"github.com/local/picobot/internal/agent/skills"
	"github.com/local/picobot/internal/providers"
)

// ContextBuilder builds messages for the LLM from session history and current message.
type ContextBuilder struct {
	workspace    string
	ranker       memory.Ranker
	topK         int
	skillsLoader *skills.Loader
}

func NewContextBuilder(workspace string, r memory.Ranker, topK int) *ContextBuilder {
	return &ContextBuilder{
		workspace:    workspace,
		ranker:       r,
		topK:         topK,
		skillsLoader: skills.NewLoader(workspace),
	}
}

func (cb *ContextBuilder) BuildMessages(history []string, currentMessage string, channel, chatID string, memoryContext string, memories []memory.MemoryItem) []providers.Message {
	msgs := make([]providers.Message, 0, len(history)+2)

	// Combine all system instructions into one message at position 0 to avoid errors in strict chat templates (e.g. llama.cpp)
	var sysParts []string

	sysParts = append(sysParts, "You are Picobot, a helpful assistant.")

	// Load workspace bootstrap files
	bootstrapFiles := []string{"SOUL.md", "AGENTS.md", "USER.md", "TOOLS.md"}
	for _, name := range bootstrapFiles {
		p := filepath.Join(cb.workspace, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue // file may not exist yet, skip silently
		}
		content := strings.TrimSpace(string(data))
		if content != "" {
			sysParts = append(sysParts, fmt.Sprintf("## %s\n\n%s", name, content))
		}
	}

	// Channel context and tool availability
	sysParts = append(sysParts, fmt.Sprintf(
		"You are operating on channel=%q chatID=%q. You have full access to all registered tools regardless of the channel. Always use your tools when the user asks you to perform actions (file operations, shell commands, web fetches, etc.).",
		channel, chatID))

	// Memory tool instruction
	sysParts = append(sysParts, "If you decide something should be remembered, call the tool 'write_memory' with JSON arguments: {\"target\": \"today\"|\"long\", \"content\": \"...\", \"append\": true|false}. Use a tool call rather than plain chat text when writing memory.")

	// Structured Planning and Context Management instructions
	sysParts = append(sysParts, `## STRUCTURED PLANNING & GOAL TRACKING
1. For any multi-step, complex, or open-ended request, you MUST start by creating a 'PLAN.md' file in the workspace.
2. The 'PLAN.md' file must contain:
   - **Objective**: Clear description of the final goal.
   - **Current Status**: Active step being executed.
   - **Steps**: List of sequential tasks to perform (e.g., Fetch data, Parse/Extract, Create HTML layout, Generate PDF, Send PDF). Mark each task as [ ] Pending, [/] In Progress, or [x] Completed.
   - **Findings**: Extracted data, links, or notes discovered along the way.
3. You MUST update this 'PLAN.md' file at each stage of your execution to track your progress.
4. To stay within token and context window limits:
   - If a webpage or command output is expected to be large (e.g., > 10KB), save the raw output directly to a file (e.g., 'downloaded_page.html') instead of displaying it in the chat.
   - Write short Go or Python scripts to parse the local file, extract the exact data needed, and print only the final clean output.
   - Keep chat history clean and concise. Focus your LLM outputs on updating the plan and invoking the next tool.`)

	// Skills context
	loadedSkills, err := cb.skillsLoader.LoadAll()
	if err != nil {
		log.Printf("error loading skills: %v", err)
	}
	if len(loadedSkills) > 0 {
		var sb strings.Builder
		sb.WriteString("Available Skills:\n")
		for _, skill := range loadedSkills {
			fmt.Fprintf(&sb, "\n## %s\n%s\n\n%s\n", skill.Name, skill.Description, skill.Content)
		}
		sysParts = append(sysParts, sb.String())
	}

	// File-based memory context (long-term + today's notes)
	if memoryContext != "" {
		sysParts = append(sysParts, "Memory:\n"+memoryContext)
	}

	// Top-K ranked memories
	selected := memories
	if cb.ranker != nil && len(memories) > 0 {
		selected = cb.ranker.Rank(currentMessage, memories, cb.topK)
	}
	if len(selected) > 0 {
		var sb strings.Builder
		sb.WriteString("Relevant memories:\n")
		for _, m := range selected {
			fmt.Fprintf(&sb, "- %s (%s)\n", m.Text, m.Kind)
		}
		sysParts = append(sysParts, sb.String())
	}

	// Emit the single consolidated system message
	msgs = append(msgs, providers.Message{Role: "system", Content: strings.Join(sysParts, "\n\n")})

	// Replay history, preserving each message's original role (user/assistant).
	// Items are stored in "role: content" format by session.AddMessage.
	for _, h := range history {
		if len(h) == 0 {
			continue
		}
		role := "user"
		content := h
		if idx := strings.Index(h, ": "); idx > 0 {
			r := h[:idx]
			if r == "user" || r == "assistant" {
				role = r
				content = h[idx+2:]
			}
		}
		msgs = append(msgs, providers.Message{Role: role, Content: content})
	}

	// Current user message
	msgs = append(msgs, providers.Message{Role: "user", Content: currentMessage})
	return msgs
}
