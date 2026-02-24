package tools

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ExecTool runs shell commands with a timeout.
// For safety:
// - prefer array form: {"cmd": ["ls", "-la"]}
// - string form (shell) is disallowed to avoid shell injection
// - safe allowlist enabled by default (opt out with PICOBOT_EXEC_ALLOW_UNSAFE=1)
// - blacklist dangerous program names (rm, sudo, dd, mkfs, shutdown, reboot)
// - arguments containing absolute paths, ~ or .. are rejected
// - optional allowedDir enforces a working directory

type ExecTool struct {
	timeout    time.Duration
	allowedDir string
}

func NewExecTool(timeoutSecs int) *ExecTool {
	return &ExecTool{timeout: time.Duration(timeoutSecs) * time.Second}
}

// NewExecToolWithWorkspace creates an ExecTool restricted to the provided workspace directory.
func NewExecToolWithWorkspace(timeoutSecs int, allowedDir string) *ExecTool {
	return &ExecTool{timeout: time.Duration(timeoutSecs) * time.Second, allowedDir: allowedDir}
}

func (t *ExecTool) Name() string { return "exec" }
func (t *ExecTool) Description() string {
	return "Execute shell commands (array form only, restricted for safety)"
}

func (t *ExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"cmd": map[string]interface{}{
				"type":        "array",
				"description": "Command as array [program, arg1, arg2, ...]. String form is disallowed for security.",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 1,
			},
		},
		"required": []string{"cmd"},
	}
}

var dangerous = map[string]struct{}{
	"rm":       {},
	"sudo":     {},
	"dd":       {},
	"mkfs":     {},
	"shutdown": {},
	"reboot":   {},
}

// Default safe allowlist. Set PICOBOT_EXEC_ALLOW_UNSAFE=1 to bypass this list.
var safeExecAllowlist = map[string]struct{}{
	"cat":    {},
	"date":   {},
	"echo":   {},
	"find":   {},
	"git":    {},
	"grep":   {},
	"head":   {},
	"ls":     {},
	"pwd":    {},
	"rg":     {},
	"sleep":  {},
	"stat":   {},
	"tail":   {},
	"true":   {},
	"false":  {},
	"uname":  {},
	"wc":     {},
	"whoami": {},
}

func isDangerousProg(prog string) bool {
	base := filepath.Base(prog)
	base = strings.ToLower(base)
	_, ok := dangerous[base]
	return ok
}

func isSafeAllowedProg(prog string) bool {
	base := strings.ToLower(filepath.Base(prog))
	_, ok := safeExecAllowlist[base]
	return ok
}

func hasUnsafeArg(s string) bool {
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") || strings.Contains(s, "..") {
		return true
	}
	return false
}

func (t *ExecTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	cmdRaw, ok := args["cmd"]
	if !ok {
		return "", fmt.Errorf("exec: 'cmd' argument required")
	}

	// Disallow shell-string commands for safety
	if _, ok := cmdRaw.(string); ok {
		return "", errors.New("exec: string commands are disallowed; use array form")
	}

	var argv []string
	switch v := cmdRaw.(type) {
	case []interface{}:
		if len(v) == 0 {
			return "", fmt.Errorf("exec: empty cmd array")
		}
		for _, a := range v {
			s, ok := a.(string)
			if !ok {
				return "", fmt.Errorf("exec: cmd array must contain strings only")
			}
			argv = append(argv, s)
		}
	default:
		return "", fmt.Errorf("exec: unsupported cmd type")
	}

	prog := argv[0]
	if !envTrue("PICOBOT_EXEC_ALLOW_UNSAFE") {
		if strings.Contains(prog, "/") || strings.Contains(prog, "\\") {
			return "", fmt.Errorf("exec: program path %q is disallowed; use a program name from safe allowlist", prog)
		}
		if !isSafeAllowedProg(prog) {
			return "", fmt.Errorf("exec: program '%s' is not in safe allowlist; set PICOBOT_EXEC_ALLOW_UNSAFE=1 to override", prog)
		}
	}
	if isDangerousProg(prog) {
		return "", fmt.Errorf("exec: program '%s' is disallowed", prog)
	}
	for _, a := range argv[1:] {
		if hasUnsafeArg(a) {
			return "", fmt.Errorf("exec: argument '%s' looks unsafe", a)
		}
	}

	cctx := ctx
	if t.timeout > 0 {
		var cancel context.CancelFunc
		cctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(cctx, prog, argv[1:]...)
	if t.allowedDir != "" {
		cmd.Dir = t.allowedDir
	}
	b, err := cmd.CombinedOutput()
	if err != nil {
		return string(b), fmt.Errorf("exec error: %w", err)
	}
	// Trim trailing newline for nicer test assertions
	out := string(b)
	out = strings.TrimRight(out, "\n")
	return out, nil
}
