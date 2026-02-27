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
	msgs := make([]providers.Message, 0, len(history)+8)
	// system prompt
	msgs = append(msgs, providers.Message{Role: "system", Content: "You are Picobot, a helpful assistant."})

	// Load workspace bootstrap files (SOUL.md, AGENTS.md, USER.md, TOOLS.md)
	// These define the agent's personality, instructions, and available tools documentation.
	bootstrapFiles := []string{"SOUL.md", "AGENTS.md", "USER.md", "TOOLS.md"}
	for _, name := range bootstrapFiles {
		p := filepath.Join(cb.workspace, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue // file may not exist yet, skip silently
		}
		content := strings.TrimSpace(string(data))
		if content != "" {
			msgs = append(msgs, providers.Message{Role: "system", Content: fmt.Sprintf("## %s\n\n%s", name, content)})
		}
	}

	// Tell the model which channel it is operating in and that tools are always available.
	msgs = append(msgs, providers.Message{Role: "system", Content: fmt.Sprintf(
		"You are operating on channel=%q chatID=%q. You have full access to all registered tools regardless of the channel. Always use your tools when the user asks you to perform actions (file operations, shell commands, web fetches, etc.).",
		channel, chatID)})

	// instruction for memory tool usage
	msgs = append(msgs, providers.Message{Role: "system", Content: "If you decide something should be remembered, call the tool 'write_memory' with JSON arguments: {\"target\": \"today\"|\"long\", \"content\": \"...\", \"append\": true|false}. Use a tool call rather than plain chat text when writing memory."})

	// Load and include skills context
	loadedSkills, err := cb.skillsLoader.LoadAll()
	if err != nil {
		log.Printf("error loading skills: %v", err)
	}
	if len(loadedSkills) > 0 {
		var sb strings.Builder
		sb.WriteString("Available Skills:\n")
		for _, skill := range loadedSkills {
			sb.WriteString(fmt.Sprintf("\n## %s\n%s\n\n%s\n", skill.Name, skill.Description, skill.Content))
		}
		msgs = append(msgs, providers.Message{Role: "system", Content: sb.String()})
	}

	// include file-based memory context (long-term + today's notes) if present
	if memoryContext != "" {
		msgs = append(msgs, providers.Message{Role: "system", Content: "Memory:\n" + memoryContext})
	}

	// select top-K memories using ranker if available
	selected := memories
	if cb.ranker != nil && len(memories) > 0 {
		selected = cb.ranker.Rank(currentMessage, memories, cb.topK)
	}
	if len(selected) > 0 {
		var sb strings.Builder
		sb.WriteString("Relevant memories:\n")
		for _, m := range selected {
			sb.WriteString(fmt.Sprintf("- %s (%s)\n", m.Text, m.Kind))
		}
		msgs = append(msgs, providers.Message{Role: "system", Content: sb.String()})
	}

	// replay history
	for _, h := range history {
		// history items are of the form "role: content"
		if len(h) > 0 {
			msgs = append(msgs, providers.Message{Role: "user", Content: h})
		}
	}

	// current
	msgs = append(msgs, providers.Message{Role: "user", Content: currentMessage})
	return msgs
}
