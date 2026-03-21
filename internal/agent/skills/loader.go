package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a loaded skill with its metadata and content.
type Skill struct {
	Name        string
	Description string
	Content     string
}

// Loader handles loading skills from the skills directory.
type Loader struct {
	workspacePath string
}

// NewLoader creates a new skill loader.
func NewLoader(workspacePath string) *Loader {
	return &Loader{workspacePath: workspacePath}
}

// LoadAll loads all skills from the skills directory.
func (l *Loader) LoadAll() ([]Skill, error) {
	skillsPath := filepath.Join(l.workspacePath, "skills")
	entries, err := os.ReadDir(skillsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Skill{}, nil
		}
		return nil, err
	}

	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(skillsPath, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); err == nil {
			skill, err := l.loadSkill(skillPath)
			if err != nil {
				// skip invalid skills but log error
				continue
			}
			skills = append(skills, skill)
		}
	}
	return skills, nil
}

// LoadByName loads a specific skill by name.
func (l *Loader) LoadByName(name string) (Skill, error) {
	skillPath := filepath.Join(l.workspacePath, "skills", name, "SKILL.md")
	return l.loadSkill(skillPath)
}

// loadSkill reads and parses a SKILL.md file.
func (l *Loader) loadSkill(skillPath string) (Skill, error) {
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return Skill{}, err
	}

	// Parse frontmatter
	lines := strings.Split(string(content), "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return Skill{}, fmt.Errorf("invalid SKILL.md format: missing frontmatter")
	}

	skill := Skill{}
	contentStartIdx := 0

	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if line == "---" {
			contentStartIdx = i + 1
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
			skill.Name = value
		case "description":
			skill.Description = value
		}
	}

	if skill.Name == "" {
		return Skill{}, fmt.Errorf("missing name in frontmatter")
	}

	// Extract content after frontmatter
	if contentStartIdx < len(lines) {
		skill.Content = strings.TrimSpace(strings.Join(lines[contentStartIdx:], "\n"))
	}

	return skill, nil
}
