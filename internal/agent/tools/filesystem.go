package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// FilesystemTool provides read/write/list operations within the filesystem.
// All operations are sandboxed to the workspace directory using os.Root (Go 1.24+),
// which provides kernel-enforced path containment via openat() syscalls.
// This prevents symlink escapes, TOCTOU races, and path traversal attacks.
type FilesystemTool struct {
	root *os.Root
}

// NewFilesystemTool opens an os.Root anchored at workspaceDir.
// The caller should call Close() when done (e.g. via defer).
func NewFilesystemTool(workspaceDir string) (*FilesystemTool, error) {
	absDir, err := filepath.Abs(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("filesystem: resolve workspace path: %w", err)
	}
	root, err := os.OpenRoot(absDir)
	if err != nil {
		return nil, fmt.Errorf("filesystem: open workspace root: %w", err)
	}
	return &FilesystemTool{root: root}, nil
}

// Close releases the underlying os.Root file descriptor.
func (t *FilesystemTool) Close() error {
	return t.root.Close()
}

func (t *FilesystemTool) Name() string        { return "filesystem" }
func (t *FilesystemTool) Description() string { return "Read, write, and list files in the workspace" }

func (t *FilesystemTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The filesystem operation to perform",
				"enum":        []string{"read", "write", "list"},
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file or directory path (relative to workspace)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write (required when action is 'write')",
			},
		},
		"required": []string{"action", "path"},
	}
}

func (t *FilesystemTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	actionRaw, ok := args["action"]
	if !ok {
		return "", fmt.Errorf("filesystem: 'action' is required")
	}
	action, ok := actionRaw.(string)
	if !ok {
		return "", fmt.Errorf("filesystem: 'action' must be a string")
	}
	pathRaw, _ := args["path"]
	pathStr := ""
	if pathRaw != nil {
		switch v := pathRaw.(type) {
		case string:
			pathStr = v
		default:
			return "", fmt.Errorf("filesystem: 'path' must be a string")
		}
	}
	if pathStr == "" {
		pathStr = "."
	}

	switch action {
	case "read":
		b, err := t.root.ReadFile(pathStr)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case "write":
		contentRaw, _ := args["content"]
		content := ""
		switch v := contentRaw.(type) {
		case string:
			content = v
		default:
			return "", fmt.Errorf("filesystem: 'content' must be a string")
		}
		// Create parent directories if needed
		dir := filepath.Dir(pathStr)
		if dir != "." {
			if err := t.root.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
		}
		if err := t.root.WriteFile(pathStr, []byte(content), 0o644); err != nil {
			return "", err
		}
		return "written", nil
	case "list":
		f, err := t.root.Open(pathStr)
		if err != nil {
			return "", err
		}
		defer f.Close()
		entries, err := f.ReadDir(-1)
		if err != nil {
			return "", err
		}
		out := ""
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			out += name + "\n"
		}
		return out, nil
	default:
		return "", fmt.Errorf("filesystem: unknown action %s", action)
	}
}
