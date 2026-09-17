package tools

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/local/picobot/internal/chat"
)

// ExecTool runs shell commands with a timeout.
// For safety:
// - prefer array form: {"cmd": ["ls", "-la"]}
// - string form (shell) is disallowed to avoid shell injection
// - blacklist dangerous program names (rm, sudo, dd, mkfs, shutdown, reboot)
// - arguments containing absolute paths, ~ or .. are rejected
// - optional allowedDir enforces a working directory

type ExecTool struct {
	hub        *chat.Hub
	timeout    time.Duration
	allowedDir string
	mu         sync.RWMutex
	channel    string
	chatID     string
}

func NewExecTool(b *chat.Hub, timeoutSecs int) *ExecTool {
	return &ExecTool{hub: b, timeout: time.Duration(timeoutSecs) * time.Second}
}

// NewExecToolWithWorkspace creates an ExecTool restricted to the provided workspace directory.
func NewExecToolWithWorkspace(b *chat.Hub, timeoutSecs int, allowedDir string) *ExecTool {
	return &ExecTool{hub: b, timeout: time.Duration(timeoutSecs) * time.Second, allowedDir: allowedDir}
}

func (t *ExecTool) Name() string { return "exec" }
func (t *ExecTool) Description() string {
	if t.allowedDir != "" {
		return fmt.Sprintf("Execute command arrays or raw shell strings in workspace %s", t.allowedDir)
	}
	return "Execute command arrays or raw shell strings"
}

func (t *ExecTool) SetContext(channel, chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.channel = channel
	t.chatID = chatID
}

func (t *ExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"cmd": map[string]interface{}{
				"type":        "array",
				"description": "Command as array [program, arg1, arg2, ...]. Safe command arrays run directly.",
				"items": map[string]interface{}{
					"type": "string",
				},
				"minItems": 1,
			},
			"shell_cmd": map[string]interface{}{
				"type":        "string",
				"description": "A raw shell command string to execute via /bin/sh -c (e.g. 'pip3 install pdfkit && python3 run.py'). ALWAYS requires user approval before execution.",
			},
		},
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

func isDangerousProg(prog string) bool {
	base := filepath.Base(prog)
	base = strings.ToLower(base)
	_, ok := dangerous[base]
	return ok
}

func (t *ExecTool) isSafeArg(s string) bool {
	if strings.Contains(s, "..") {
		return false
	}
	if strings.HasPrefix(s, "~") {
		return false
	}
	if strings.HasPrefix(s, "/") {
		if t.allowedDir != "" {
			cleanAllowed := filepath.Clean(t.allowedDir)
			cleanArg := filepath.Clean(s)
			rel, err := filepath.Rel(cleanAllowed, cleanArg)
			if err == nil && !strings.HasPrefix(rel, "..") {
				return true
			}
		}
		return false
	}
	return true
}

func formatCmd(prog string, argv []string, shellCmd string, isShell bool) string {
	if isShell {
		return shellCmd
	}
	var sb strings.Builder
	sb.WriteString(prog)
	for _, a := range argv[1:] {
		sb.WriteString(" ")
		if strings.Contains(a, " ") || strings.Contains(a, "\n") {
			sb.WriteString(fmt.Sprintf("%q", a))
		} else {
			sb.WriteString(a)
		}
	}
	return sb.String()
}

func (t *ExecTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	cmdRaw, hasCmd := args["cmd"]
	shellCmdRaw, hasShellCmd := args["shell_cmd"]
	if !hasCmd && !hasShellCmd {
		return "", fmt.Errorf("exec: 'cmd' or 'shell_cmd' argument required")
	}

	channel, chatID := chat.FromContext(ctx)
	log.Printf("ExecTool.Execute: fromContext channel=%q chatID=%q", channel, chatID)
	if channel == "" {
		t.mu.RLock()
		channel = t.channel
		t.mu.RUnlock()
	}
	if chatID == "" {
		t.mu.RLock()
		chatID = t.chatID
		t.mu.RUnlock()
	}
	requireApproval := channel != "cli" && channel != "" && chatID != "" && t.hub != nil && channel != "heartbeat" && channel != "cron"

	var isShell bool
	var shellCmdStr string
	var prog string
	var argv []string

	if hasShellCmd {
		sStr, ok := shellCmdRaw.(string)
		if !ok || sStr == "" {
			return "", fmt.Errorf("exec: 'shell_cmd' must be a non-empty string")
		}
		isShell = true
		shellCmdStr = sStr
	} else {
		var argvRaw []interface{}
		switch v := cmdRaw.(type) {
		case []interface{}:
			argvRaw = v
		default:
			return "", fmt.Errorf("exec: unsupported cmd type (must be an array of strings)")
		}

		if len(argvRaw) == 0 {
			return "", fmt.Errorf("exec: empty cmd array")
		}
		for _, a := range argvRaw {
			s, ok := a.(string)
			if !ok {
				return "", fmt.Errorf("exec: cmd array must contain strings only")
			}
			argv = append(argv, s)
		}

		prog = argv[0]
		if !requireApproval {
			if isDangerousProg(prog) {
				return "", fmt.Errorf("exec: program '%s' is disallowed", prog)
			}
			for _, a := range argv[1:] {
				if !t.isSafeArg(a) {
					return "", fmt.Errorf("exec: argument '%s' looks unsafe", a)
				}
			}
		}
	}

	// Request user approval for command execution
	if requireApproval {
		cmdDisplay := formatCmd(prog, argv, shellCmdStr, isShell)
		msgContent := fmt.Sprintf("⚠️ **Approval Required**\nPicobot wants to run the following command:\n\n```bash\n%s\n```\n\nChoose an action below to proceed.", cmdDisplay)
		out := chat.Outbound{
			Channel: channel,
			ChatID:  chatID,
			Content: msgContent,
			Metadata: map[string]interface{}{
				"telegram_reply_markup": `{"inline_keyboard": [[{"text": "Approve ✅", "callback_data": "yes"}, {"text": "Deny ❌", "callback_data": "no"}]]}`,
			},
		}
		select {
		case t.hub.Out <- out:
		default:
			return "", fmt.Errorf("exec: outbound queue full, cannot send approval request")
		}

		// Wait for user response
		approvalChan := make(chan string, 1)
		key := channel + ":" + chatID
		chat.RegisterApproval(key, approvalChan)
		defer chat.UnregisterApproval(key)

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case resp := <-approvalChan:
			respClean := strings.ToLower(strings.TrimSpace(resp))
			if respClean != "yes" && respClean != "approve" && respClean != "y" {
				return "", fmt.Errorf("exec: command denied by user response: %q", resp)
			}
		}
	}

	cctx := ctx
	if t.timeout > 0 {
		var cancel context.CancelFunc
		cctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	var cmd *exec.Cmd
	if isShell {
		cmd = exec.CommandContext(cctx, "/bin/sh", "-c", shellCmdStr)
	} else {
		cmd = exec.CommandContext(cctx, prog, argv[1:]...)
	}

	if t.allowedDir != "" {
		cmd.Dir = t.allowedDir
	}
	b, err := cmd.CombinedOutput()
	if err != nil {
		return string(b), fmt.Errorf("exec error: %w", err)
	}
	out := string(b)
	out = strings.TrimRight(out, "\n")
	return out, nil
}
