package tools

import (
	"context"
	"os"
	"testing"
)

func openTestRoot(t *testing.T) *os.Root {
	t.Helper()
	tmpDir := t.TempDir()
	root, err := os.OpenRoot(tmpDir)
	if err != nil {
		t.Fatalf("OpenRoot failed: %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Logf("root.Close: %v", err)
		}
	})
	return root
}

func TestSkillManager_CreateSkill(t *testing.T) {
	root := openTestRoot(t)
	mgr := NewSkillManager(root)

	err := mgr.CreateSkill("test-skill", "Test description", "# Test\n\nTest content")
	if err != nil {
		t.Fatalf("CreateSkill failed: %v", err)
	}

	// Verify file was created
	content, err := root.ReadFile("skills/test-skill/SKILL.md")
	if err != nil {
		t.Fatalf("Failed to read created skill: %v", err)
	}

	strContent := string(content)
	if !containsString(strContent, "name: test-skill") {
		t.Error("Skill content missing name in frontmatter")
	}
	if !containsString(strContent, "Test content") {
		t.Error("Skill content missing body content")
	}
}

func TestSkillManager_ListSkills(t *testing.T) {
	root := openTestRoot(t)
	mgr := NewSkillManager(root)

	// Create test skills
	if err := mgr.CreateSkill("skill1", "Description 1", "Content 1"); err != nil {
		t.Fatalf("CreateSkill skill1 failed: %v", err)
	}
	if err := mgr.CreateSkill("skill2", "Description 2", "Content 2"); err != nil {
		t.Fatalf("CreateSkill skill2 failed: %v", err)
	}

	skills, err := mgr.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}

	if len(skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(skills))
	}
}

func TestSkillManager_GetSkill(t *testing.T) {
	root := openTestRoot(t)
	mgr := NewSkillManager(root)

	if err := mgr.CreateSkill("test-skill", "Test description", "# Test\n\nTest content"); err != nil {
		t.Fatalf("CreateSkill failed: %v", err)
	}

	content, err := mgr.GetSkill("test-skill")
	if err != nil {
		t.Fatalf("GetSkill failed: %v", err)
	}

	if !containsString(content, "Test content") {
		t.Error("Skill content not returned correctly")
	}
}

func TestSkillManager_DeleteSkill(t *testing.T) {
	root := openTestRoot(t)
	mgr := NewSkillManager(root)

	if err := mgr.CreateSkill("test-skill", "Test description", "Content"); err != nil {
		t.Fatalf("CreateSkill failed: %v", err)
	}

	err := mgr.DeleteSkill("test-skill")
	if err != nil {
		t.Fatalf("DeleteSkill failed: %v", err)
	}

	// Verify skill is gone
	_, err = root.Stat("skills/test-skill")
	if !os.IsNotExist(err) {
		t.Error("Skill directory still exists after deletion")
	}
}

func TestCreateSkillTool_Execute(t *testing.T) {
	root := openTestRoot(t)
	mgr := NewSkillManager(root)
	tool := NewCreateSkillTool(mgr)

	args := map[string]interface{}{
		"name":        "test-skill",
		"description": "Test description",
		"content":     "# Test Content",
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !containsString(result, "created successfully") {
		t.Errorf("Unexpected result: %s", result)
	}

	// Verify skill was created
	_, err = root.Stat("skills/test-skill/SKILL.md")
	if os.IsNotExist(err) {
		t.Error("Skill file was not created")
	}
}

func TestListSkillsTool_Execute(t *testing.T) {
	root := openTestRoot(t)
	mgr := NewSkillManager(root)
	if err := mgr.CreateSkill("skill1", "Description 1", "Content 1"); err != nil {
		t.Fatalf("CreateSkill skill1 failed: %v", err)
	}
	if err := mgr.CreateSkill("skill2", "Description 2", "Content 2"); err != nil {
		t.Fatalf("CreateSkill skill2 failed: %v", err)
	}

	tool := NewListSkillsTool(mgr)
	result, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !containsString(result, "skill1") || !containsString(result, "skill2") {
		t.Errorf("Expected skills in result, got: %s", result)
	}
}

func TestReadSkillTool_Execute(t *testing.T) {
	root := openTestRoot(t)
	mgr := NewSkillManager(root)
	if err := mgr.CreateSkill("test-skill", "Test description", "# Test Content"); err != nil {
		t.Fatalf("CreateSkill failed: %v", err)
	}

	tool := NewReadSkillTool(mgr)
	args := map[string]interface{}{"name": "test-skill"}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !containsString(result, "Test Content") {
		t.Errorf("Expected skill content in result, got: %s", result)
	}
}

func TestDeleteSkillTool_Execute(t *testing.T) {
	root := openTestRoot(t)
	mgr := NewSkillManager(root)
	if err := mgr.CreateSkill("test-skill", "Test description", "Content"); err != nil {
		t.Fatalf("CreateSkill failed: %v", err)
	}

	tool := NewDeleteSkillTool(mgr)
	args := map[string]interface{}{"name": "test-skill"}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !containsString(result, "deleted successfully") {
		t.Errorf("Unexpected result: %s", result)
	}

	// Verify skill is gone
	_, err = root.Stat("skills/test-skill")
	if !os.IsNotExist(err) {
		t.Error("Skill directory still exists after deletion")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsAt(s, substr))))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
