package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// SkillMetadata holds metadata parsed from SKILL.md frontmatter.
type SkillMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SkillManager provides tools for managing skills in the workspace.
// All file operations are sandboxed via os.Root (Go 1.24+).
type SkillManager struct {
	root *os.Root // rooted at the workspace directory
}

// NewSkillManager creates a new skill manager backed by an os.Root.
func NewSkillManager(root *os.Root) *SkillManager {
	return &SkillManager{root: root}
}

// ListSkills returns a list of all skills in the skills directory.
func (sm *SkillManager) ListSkills() ([]SkillMetadata, error) {
	f, err := sm.root.Open("skills")
	if err != nil {
		if os.IsNotExist(err) {
			return []SkillMetadata{}, nil
		}
		return nil, err
	}
	defer f.Close()

	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}

	var skills []SkillMetadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := "skills/" + entry.Name() + "/SKILL.md"
		if _, err := sm.root.Stat(skillFile); err == nil {
			meta, err := sm.parseSkillMetadata(skillFile)
			if err != nil {
				// skip invalid skills
				continue
			}
			skills = append(skills, meta)
		}
	}
	return skills, nil
}

// GetSkill reads a skill's content by name.
func (sm *SkillManager) GetSkill(name string) (string, error) {
	content, err := sm.root.ReadFile("skills/" + name + "/SKILL.md")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// CreateSkill creates a new skill with the given name and content.
// Path traversal is prevented by os.Root at the kernel level.
func (sm *SkillManager) CreateSkill(name, description, content string) error {
	if name == "" {
		return fmt.Errorf("skill name is required")
	}
	name = strings.TrimSpace(name)

	skillDir := "skills/" + name
	if err := sm.root.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}

	// Create SKILL.md with frontmatter
	frontmatter := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n", name, description)
	fullContent := frontmatter + content

	return sm.root.WriteFile(skillDir+"/SKILL.md", []byte(fullContent), 0o644)
}

// DeleteSkill removes a skill directory.
func (sm *SkillManager) DeleteSkill(name string) error {
	return sm.root.RemoveAll("skills/" + name)
}

// parseSkillMetadata extracts metadata from SKILL.md frontmatter.
func (sm *SkillManager) parseSkillMetadata(skillPath string) (SkillMetadata, error) {
	content, err := sm.root.ReadFile(skillPath)
	if err != nil {
		return SkillMetadata{}, err
	}

	// parse YAML frontmatter (simple parser for name and description)
	lines := strings.Split(string(content), "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return SkillMetadata{}, fmt.Errorf("invalid frontmatter")
	}

	meta := SkillMetadata{}
	inFrontmatter := true
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if line == "---" {
			inFrontmatter = false
			break
		}
		if !inFrontmatter {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "name":
			meta.Name = value
		case "description":
			meta.Description = value
		}
	}

	if meta.Name == "" {
		return SkillMetadata{}, fmt.Errorf("missing name in frontmatter")
	}
	return meta, nil
}

// ============================================================================
// Tool Implementations
// ============================================================================

// CreateSkillTool allows the agent to create new skills.
type CreateSkillTool struct {
	manager *SkillManager
}

func NewCreateSkillTool(manager *SkillManager) *CreateSkillTool {
	return &CreateSkillTool{manager: manager}
}

func (t *CreateSkillTool) Name() string { return "create_skill" }

func (t *CreateSkillTool) Description() string {
	return "Create a new skill in the skills directory with markdown content"
}

func (t *CreateSkillTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The skill name (alphanumeric, no special chars)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Brief description of what the skill does",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The markdown content for the skill (instructions, examples, etc.)",
			},
		},
		"required": []string{"name", "description", "content"},
	}
}

func (t *CreateSkillTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	name, ok := args["name"].(string)
	if !ok {
		return "", fmt.Errorf("name (string) is required")
	}
	description, ok := args["description"].(string)
	if !ok {
		return "", fmt.Errorf("description (string) is required")
	}
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content (string) is required")
	}

	if err := t.manager.CreateSkill(name, description, content); err != nil {
		return "", err
	}
	return fmt.Sprintf("Skill '%s' created successfully", name), nil
}

// ListSkillsTool lists all available skills.
type ListSkillsTool struct {
	manager *SkillManager
}

func NewListSkillsTool(manager *SkillManager) *ListSkillsTool {
	return &ListSkillsTool{manager: manager}
}

func (t *ListSkillsTool) Name() string { return "list_skills" }

func (t *ListSkillsTool) Description() string {
	return "List all available skills with their names and descriptions"
}

func (t *ListSkillsTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *ListSkillsTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	skills, err := t.manager.ListSkills()
	if err != nil {
		return "", err
	}
	if len(skills) == 0 {
		return "No skills found", nil
	}
	result, err := json.MarshalIndent(skills, "", "  ")
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// ReadSkillTool reads a skill's content.
type ReadSkillTool struct {
	manager *SkillManager
}

func NewReadSkillTool(manager *SkillManager) *ReadSkillTool {
	return &ReadSkillTool{manager: manager}
}

func (t *ReadSkillTool) Name() string { return "read_skill" }

func (t *ReadSkillTool) Description() string {
	return "Read the full content of a skill by name"
}

func (t *ReadSkillTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The name of the skill to read",
			},
		},
		"required": []string{"name"},
	}
}

func (t *ReadSkillTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	name, ok := args["name"].(string)
	if !ok {
		return "", fmt.Errorf("name (string) is required")
	}
	return t.manager.GetSkill(name)
}

// DeleteSkillTool deletes a skill.
type DeleteSkillTool struct {
	manager *SkillManager
}

func NewDeleteSkillTool(manager *SkillManager) *DeleteSkillTool {
	return &DeleteSkillTool{manager: manager}
}

func (t *DeleteSkillTool) Name() string { return "delete_skill" }

func (t *DeleteSkillTool) Description() string {
	return "Delete a skill from the skills directory"
}

func (t *DeleteSkillTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The name of the skill to delete",
			},
		},
		"required": []string{"name"},
	}
}

func (t *DeleteSkillTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	name, ok := args["name"].(string)
	if !ok {
		return "", fmt.Errorf("name (string) is required")
	}
	if err := t.manager.DeleteSkill(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Skill '%s' deleted successfully", name), nil
}
