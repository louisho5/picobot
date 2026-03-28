package config

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/local/picobot/embeds"
)

// DefaultConfig returns a minimal default Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Agents: AgentsConfig{Defaults: AgentDefaults{
			Workspace:                   "~/.picobot/workspace",
			Model:                       "stub-model",
			MaxTokens:                   8192,
			Temperature:                 0.7,
			MaxToolIterations:           100,
			HeartbeatIntervalS:          60,
			RequestTimeoutS:             60,
			EnableToolActivityIndicator: boolPtr(true),
		}},
		Channels: ChannelsConfig{
			Telegram: TelegramConfig{Enabled: false, Token: "", AllowFrom: []string{}},
			Discord:  DiscordConfig{Enabled: false, Token: "", AllowFrom: []string{}},
			Slack:    SlackConfig{Enabled: false, AppToken: "", BotToken: "", AllowUsers: []string{}, AllowChannels: []string{}},
			WhatsApp: WhatsAppConfig{Enabled: false, DBPath: "", AllowFrom: []string{}},
		},
		MCPServers: map[string]MCPServerConfig{},
		Providers: ProvidersConfig{
			OpenAI: &ProviderConfig{APIKey: "sk-or-v1-REPLACE_ME", APIBase: "https://openrouter.ai/api/v1"},
		},
	}
}

// boolPtr returns a pointer to the given bool value.
func boolPtr(b bool) *bool { return &b }

// SaveConfig writes the config to the given path (creating parent dirs).
func SaveConfig(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o640)
}

// InitializeWorkspace creates the workspace dir and bootstrap files.
func InitializeWorkspace(basePath string) error {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		"SOUL.md": `# Soul

I am picobot 🤖, a personal AI assistant.

## Personality

- Helpful and friendly
- Concise and to the point
- Curious and eager to learn

## Values

- Accuracy over speed
- User privacy and safety
- Transparency in actions

## Communication Style

- Be clear and direct
- Explain reasoning when helpful
- Ask clarifying questions when needed
`,

		"AGENTS.md": `# Agent Instructions

You are a helpful AI assistant. Be concise, accurate, and friendly.

## Guidelines

- Always explain what you're doing before taking actions
- Ask for clarification when the request is ambiguous
- Use tools to help accomplish tasks
- Remember important information using the write_memory tool
- Use read_memory to check existing memory before writing, to avoid duplicates
- Use edit_memory to update or correct specific facts already stored
- Use list_memory to see all available memory files
- Use delete_memory to clean up outdated daily notes

## File Creation

When the user asks you to create files, code, projects, or any deliverable:

1. Always create them inside the workspace directory
2. Create a project folder with the naming convention: project-YYYYMMDD-HHMMSS-TASKNAME
   - YYYYMMDD-HHMMSS is the current date and time
   - TASKNAME is a short lowercase slug describing the task (e.g. landing-page, python-scraper, budget-tracker)
3. Create all files inside that project folder
4. Use the filesystem tool with action "write" for each file
5. After creating all files, list the project folder to confirm

Example: if the user says "create a landing page for my coffee shop", create:
  project-20260208-143000-coffee-landing/
    index.html
    style.css
    script.js

Never create files directly in the workspace root. Always use a project folder.

## Memory

- Use the write_memory tool with target "today" for daily notes
- Use the write_memory tool with target "long" for long-term information
- Use read_memory to check what is already stored before writing new entries
- Use edit_memory to update or correct individual facts without rewriting the whole file
- Use list_memory to see all available memory files
- Use delete_memory to clean up outdated daily notes
- Do NOT just say you'll remember something — actually call write_memory
- NEVER write heartbeat results, health checks, or periodic status logs to memory — these are ephemeral and must be discarded after each run
- Memory is for durable user knowledge only: facts, preferences, project notes, decisions

## Skills

- You can create new skills with the create_skill tool
- Skills are reusable knowledge/procedures stored in skills/
- List available skills with list_skills before creating duplicates

## Safety

- Never execute dangerous commands (rm -rf, format, dd, shutdown)
- Ask for confirmation before destructive file operations
- Do not expose API keys or credentials in responses
`,

		"USER.md": `# User Profile

Information about the user to help personalize interactions.

## Basic Information

- **Name**: (your name)
- **Timezone**: (your timezone, e.g., UTC+8)
- **Language**: (preferred language)

## Preferences

### Communication Style

- [ ] Casual
- [x] Professional
- [ ] Technical

### Response Length

- [x] Brief and concise
- [ ] Adaptive based on question
- [ ] Detailed explanations

### Technical Level

- [ ] Beginner
- [x] Intermediate
- [ ] Expert

## Work Context

- **Primary Role**: (your role, e.g., developer, researcher)
- **Main Projects**: (what you're working on)
- **Tools You Use**: (IDEs, languages, frameworks)

## Topics of Interest

- (add your interests here)
`,

		"TOOLS.md": `# Available Tools

This document describes the tools available to picobot.

## File Operations

### filesystem
Read, write, and list files in the workspace.
- action: "read", "write", "list"
- path: file or directory path (relative to workspace)
- content: (for "write" action) the content to write

Examples:
- Read: {"action": "read", "path": "data.csv"}
- Write: {"action": "write", "path": "data.csv", "content": "Name\nBen\nKen\n"}
- List: {"action": "list", "path": "."}

## Shell Execution

### exec
Execute a shell command and return output.
- command: the shell command to run
- Commands have a timeout (default 60s)
- Dangerous commands are blocked

## Web Access

### web
Fetch and extract content from a URL.
- url: the URL to fetch
- Useful for checking websites, APIs, documentation

### web_search
Search the web using DuckDuckGo (no API key required).
- query: the search terms
- Returns an instant answer, abstract summary, and/or related result links
- Use this to find relevant URLs, then use the web tool to fetch the full page if needed

## Messaging

### message
Send a message to the current channel/chat.
- content: the message text

## Memory

### write_memory
Persist information to memory files. Never store redundant information like heartbeat logs.
- target: "today" (daily notes) or "long" (long-term memory)
- content: what to remember
- append: true to add, false to replace

### list_memory
List all memory files (daily notes and long-term MEMORY.md).
- No arguments needed

### read_memory
Read the contents of a specific memory file.
- target: "today", "long", or a date "YYYY-MM-DD"

### edit_memory
Find and replace text within a memory file.
- target: "today", "long", or "YYYY-MM-DD"
- old_text: exact text to find
- new_text: replacement text (omit or empty string to delete the matched text)

### delete_memory
Delete a daily memory file. Cannot delete long-term memory (MEMORY.md).
- target: date in "YYYY-MM-DD" format

## Skill Management

### create_skill
Create a new skill in the skills/ directory.
- name: skill name (used as folder name)
- description: brief description
- content: the skill's markdown content

### list_skills
List all available skills. No arguments needed.

### read_skill
Read a specific skill's content.
- name: the skill name to read

### delete_skill
Delete a skill from skills/.
- name: the skill name to delete

## Background Tasks

### spawn
Spawn a background subagent process.

### cron
Schedule or manage cron jobs.
`,

		"HEARTBEAT.md": `# Heartbeat

This file is checked periodically (every 60 seconds). Add tasks here that should run on a schedule.

## IMPORTANT RULES FOR HEARTBEAT PROCESSING

- After reviewing this file, take actions ONLY if there are explicit tasks listed below
- If there are no tasks (or all tasks are complete), do NOTHING — do not send any message, do not call write_memory or any memory tool
- NEVER log "heartbeat check complete", "system status: healthy", or any status message to memory files — these clutter memory with useless noise
- Heartbeat results are ephemeral: process, act if needed, then stop silently

## Periodic Tasks

<!-- Add tasks below. The agent will process them on each heartbeat check. -->
<!-- Example:
- Check server status at https://example.com/health
- Summarize unread messages
-->
`,
	}
	for name, content := range files {
		p := filepath.Join(basePath, name)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				return err
			}
		}
	}
	// memory dir
	mem := filepath.Join(basePath, "memory")
	if err := os.MkdirAll(mem, 0o755); err != nil {
		return err
	}
	mm := filepath.Join(mem, "MEMORY.md")
	if _, err := os.Stat(mm); os.IsNotExist(err) {
		if err := os.WriteFile(mm, []byte("# Long-term Memory\n\nImportant facts and information to remember across sessions.\n"), 0o644); err != nil {
			return err
		}
	}

	// skills dir — extract embedded sample skills
	skillsDir := filepath.Join(basePath, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return err
	}
	if err := extractEmbeddedSkills(skillsDir); err != nil {
		return err
	}

	return nil
}

// extractEmbeddedSkills walks the embedded skills FS and writes each file
// to the target directory, skipping files that already exist.
func extractEmbeddedSkills(targetDir string) error {
	return fs.WalkDir(embeds.Skills, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Strip the leading "skills/" prefix to get the relative path
		rel, err := filepath.Rel("skills", path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dest := filepath.Join(targetDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		// Skip if file already exists (don't overwrite user changes)
		if _, err := os.Stat(dest); err == nil {
			return nil
		}
		data, err := embeds.Skills.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
}

// ResolveDefaultPaths returns absolute paths for the config and workspace based on home directory.
func ResolveDefaultPaths() (cfgPath string, workspacePath string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	cfgPath = filepath.Join(home, ".picobot", "config.json")
	workspacePath = filepath.Join(home, ".picobot", "workspace")
	return cfgPath, workspacePath, nil
}

// Onboard writes default config and initializes the workspace at the user's home.
func Onboard() (string, string, error) {
	cfgPath, workspacePath, err := ResolveDefaultPaths()
	if err != nil {
		return "", "", err
	}
	cfg := DefaultConfig()
	// set workspace path in config
	cfg.Agents.Defaults.Workspace = workspacePath
	if err := SaveConfig(cfg, cfgPath); err != nil {
		return "", "", fmt.Errorf("saving config: %w", err)
	}
	if err := InitializeWorkspace(workspacePath); err != nil {
		return "", "", fmt.Errorf("initializing workspace: %w", err)
	}
	return cfgPath, workspacePath, nil
}
