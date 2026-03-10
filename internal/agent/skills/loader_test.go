package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader_LoadAll(t *testing.T) {
	// Create temp workspace
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create test skills
	testSkills := []struct {
		name        string
		description string
		content     string
	}{
		{"weather", "Get weather info", "# Weather\n\nUse curl wttr.in"},
		{"calculator", "Math calculations", "# Calculator\n\nUse bc command"},
	}

	for _, ts := range testSkills {
		skillDir := filepath.Join(skillsDir, ts.name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		skillFile := filepath.Join(skillDir, "skill.md")
		content := "---\nname: " + ts.name + "\ndescription: " + ts.description + "\n---\n\n" + ts.content
		if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Test loading
	loader := NewLoader(tmpDir)
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}

	// Verify content
	for _, skill := range skills {
		if skill.Name == "" {
			t.Error("skill name is empty")
		}
		if skill.Description == "" {
			t.Error("skill description is empty")
		}
		if skill.Content == "" {
			t.Error("skill content is empty")
		}
	}
}

func TestLoader_LoadByName(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills", "test-skill")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillFile := filepath.Join(skillsDir, "skill.md")
	content := "---\nname: test-skill\ndescription: Test skill\n---\n\n# Test\n\nTest content"
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	skill, err := loader.LoadByName("test-skill")
	if err != nil {
		t.Fatalf("LoadByName failed: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got '%s'", skill.Name)
	}
	if skill.Description != "Test skill" {
		t.Errorf("expected description 'Test skill', got '%s'", skill.Description)
	}
	if !strings.Contains(skill.Content, "Test content") {
		t.Errorf("expected content to contain 'Test content', got '%s'", skill.Content)
	}
}
