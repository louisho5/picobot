package config

import (
	"os"
	"path/filepath"
)

// WorkspaceFiles defines bootstrap filenames used in the workspace.
// Lowercase names are the defaults; uppercase names are legacy fallbacks.
type WorkspaceFiles struct {
	Soul      string `json:"soul,omitempty"`
	Agents    string `json:"agents,omitempty"`
	User      string `json:"user,omitempty"`
	Tools     string `json:"tools,omitempty"`
	Heartbeat string `json:"heartbeat,omitempty"`
	Memory    string `json:"memory,omitempty"`
	Skill     string `json:"skill,omitempty"`
}

func defaultWorkspaceFiles() WorkspaceFiles {
	return WorkspaceFiles{
		Soul:      "soul.md",
		Agents:    "agents.md",
		User:      "user.md",
		Tools:     "tools.md",
		Heartbeat: "heartbeat.md",
		Memory:    "memory.md",
		Skill:     "skill.md",
	}
}

func legacyWorkspaceFiles() WorkspaceFiles {
	return WorkspaceFiles{
		Soul:      "SOUL.md",
		Agents:    "AGENTS.md",
		User:      "USER.md",
		Tools:     "TOOLS.md",
		Heartbeat: "HEARTBEAT.md",
		Memory:    "MEMORY.md",
		Skill:     "SKILL.md",
	}
}

// DefaultWorkspaceFiles returns the default (lowercase) workspace filenames.
func DefaultWorkspaceFiles() WorkspaceFiles {
	return defaultWorkspaceFiles()
}

// LegacyWorkspaceFiles returns the legacy (uppercase) workspace filenames.
func LegacyWorkspaceFiles() WorkspaceFiles {
	return legacyWorkspaceFiles()
}

func mergeWorkspaceFiles(custom WorkspaceFiles) WorkspaceFiles {
	out := defaultWorkspaceFiles()
	if custom.Soul != "" {
		out.Soul = custom.Soul
	}
	if custom.Agents != "" {
		out.Agents = custom.Agents
	}
	if custom.User != "" {
		out.User = custom.User
	}
	if custom.Tools != "" {
		out.Tools = custom.Tools
	}
	if custom.Heartbeat != "" {
		out.Heartbeat = custom.Heartbeat
	}
	if custom.Memory != "" {
		out.Memory = custom.Memory
	}
	if custom.Skill != "" {
		out.Skill = custom.Skill
	}
	return out
}

// ResolveWorkspaceFiles returns configured workspace filenames with defaults.
func ResolveWorkspaceFiles(cfg Config) WorkspaceFiles {
	return mergeWorkspaceFiles(cfg.Agents.Defaults.Files)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ResolveWorkspaceFilePath returns the first existing file path from the
// preferred configured name and the legacy fallback name.
func ResolveWorkspaceFilePath(basePath, preferred, legacy string) string {
	if preferred != "" {
		p := filepath.Join(basePath, preferred)
		if fileExists(p) {
			return p
		}
	}
	if legacy != "" && legacy != preferred {
		p := filepath.Join(basePath, legacy)
		if fileExists(p) {
			return p
		}
	}
	if preferred != "" {
		return filepath.Join(basePath, preferred)
	}
	return filepath.Join(basePath, legacy)
}
