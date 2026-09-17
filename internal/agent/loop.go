package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/local/picobot/internal/agent/memory"
	"github.com/local/picobot/internal/agent/tools"
	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/config"
	"github.com/local/picobot/internal/cron"
	"github.com/local/picobot/internal/mcp"
	"github.com/local/picobot/internal/providers"
	"github.com/local/picobot/internal/session"
)

var rememberRE = regexp.MustCompile(`(?i)^remember(?:\s+to)?\s+(.+)$`)

// sendChannelNotification delivers a non-blocking status message back to the
// originating channel so the user can see tool progress in real time.
// It is a no-op for system channels (heartbeat, cron) that have no user-facing chat.
func sendChannelNotification(hub *chat.Hub, channel, chatID, content string) {
	if isSystemChannel(channel) {
		return
	}
	out := chat.Outbound{
		Channel:  channel,
		ChatID:   chatID,
		Content:  content,
		Metadata: map[string]interface{}{"is_notification": true},
	}
	select {
	case hub.Out <- out:
	default:
		log.Println("sendChannelNotification: outbound channel full, dropping notification")
	}
}

// isSystemChannel reports whether a channel is a background/system trigger
// (heartbeat, cron) rather than an interactive user-facing channel.
// Messages from system channels are processed statelessly: no session history
// is loaded as context and nothing is written back to disk.  This prevents the
// heartbeat session file from growing unboundedly and keeps each invocation's
// context window small.
func isSystemChannel(channel string) bool {
	switch channel {
	case "heartbeat", "cron":
		return true
	default:
		return false
	}
}

// AgentLoop is the core processing loop; it holds an LLM provider, tools, sessions and context builder.
type AgentLoop struct {
	hub                *chat.Hub
	provider           providers.LLMProvider
	tools              *tools.Registry
	sessions           *session.SessionManager
	context            *ContextBuilder
	memory             *memory.MemoryStore
	model              string
	maxIterations      int
	running            bool
	mcpClients         []*mcp.Client
	enableToolActivity bool
	workspace          string
}

// NewAgentLoop creates a new AgentLoop with the given provider.
func NewAgentLoop(b *chat.Hub, provider providers.LLMProvider, model string, maxIterations int, workspace string, scheduler *cron.Scheduler, mcpServers map[string]config.MCPServerConfig) *AgentLoop {
	if model == "" {
		model = provider.GetDefaultModel()
	}
	if workspace == "" {
		workspace = "."
	}
	reg := tools.NewRegistry()
	// register default tools
	reg.Register(tools.NewMessageTool(b))

	// Open an os.Root anchored at the workspace for kernel-enforced sandboxing.
	root, err := os.OpenRoot(workspace)
	if err != nil {
		log.Fatalf("failed to open workspace root %q: %v", workspace, err)
	}

	fsTool, err := tools.NewFilesystemTool(b, workspace)
	if err != nil {
		log.Fatalf("failed to create filesystem tool: %v", err)
	}
	reg.Register(fsTool)
	reg.Register(tools.NewSendFileTool(b, workspace))

	reg.Register(tools.NewExecToolWithWorkspace(b, 60, workspace))
	reg.Register(tools.NewWebTool())
	reg.Register(tools.NewWebSearchTool())
	reg.Register(tools.NewSpawnTool())
	if scheduler != nil {
		reg.Register(tools.NewCronTool(scheduler))
	}

	sm := session.NewSessionManager(workspace)
	ctx := NewContextBuilder(workspace, memory.NewSimpleRanker(), 5)
	mem := memory.NewMemoryStoreWithWorkspace(workspace, 100)
	// register memory tools (all share the same store instance)
	reg.Register(tools.NewWriteMemoryTool(mem))
	reg.Register(tools.NewListMemoryTool(mem))
	reg.Register(tools.NewReadMemoryTool(mem))
	reg.Register(tools.NewEditMemoryTool(mem))
	reg.Register(tools.NewDeleteMemoryTool(mem))

	// register skill management tools (share the same os.Root)
	skillMgr := tools.NewSkillManager(root)
	reg.Register(tools.NewCreateSkillTool(skillMgr))
	reg.Register(tools.NewListSkillsTool(skillMgr))
	reg.Register(tools.NewReadSkillTool(skillMgr))
	reg.Register(tools.NewDeleteSkillTool(skillMgr))

	// Connect to configured MCP servers and register their tools.
	var mcpClients []*mcp.Client
	for name, cfg := range mcpServers {
		var client *mcp.Client
		var err error
		switch {
		case cfg.Command != "":
			client, err = mcp.NewStdioClient(name, cfg.Command, cfg.Args)
		case cfg.URL != "":
			client, err = mcp.NewHTTPClient(name, cfg.URL, cfg.Headers)
		default:
			log.Printf("MCP server %q: no command or url configured, skipping", name)
			continue
		}
		if err != nil {
			log.Printf("MCP server %q: failed to connect: %v", name, err)
			continue
		}
		mcpClients = append(mcpClients, client)
		for _, tool := range client.Tools() {
			reg.Register(tools.NewMCPTool(client, name, tool))
		}
		log.Printf("MCP server %q: registered %d tools", name, len(client.Tools()))
	}

	return &AgentLoop{hub: b, provider: provider, tools: reg, sessions: sm, context: ctx, memory: mem, model: model, maxIterations: maxIterations, mcpClients: mcpClients, enableToolActivity: true, workspace: workspace}
}

// SetToolActivityIndicator controls whether the feedback of tool progress
func (a *AgentLoop) SetToolActivityIndicator(enabled bool) {
	a.enableToolActivity = enabled
}

// SetMemoryRanker overrides the default memory ranker.
func (a *AgentLoop) SetMemoryRanker(ranker memory.Ranker) {
	a.context.ranker = ranker
}

// Close shuts down all MCP server connections.
func (a *AgentLoop) Close() {
	for _, c := range a.mcpClients {
		_ = c.Close()
	}
}

// Run starts processing inbound messages. This is a blocking call until context is canceled.
func (a *AgentLoop) Run(ctx context.Context) {
	a.running = true
	log.Println("Agent loop started")

	for a.running {
		select {
		case <-ctx.Done():
			log.Println("Agent loop received shutdown signal")
			a.running = false
			return
		case msg, ok := <-a.hub.In:
			if !ok {
				log.Println("Inbound channel closed, stopping agent loop")
				a.running = false
				return
			}

			log.Printf("Processing message from %s:%s\n", msg.Channel, msg.SenderID)

			// Check if this message resolves a pending tool execution approval (e.g. exec)
			key := msg.Channel + ":" + msg.ChatID
			if chat.TriggerApproval(key, msg.Content) {
				continue
			}

			// Offload the message processing to a concurrent goroutine so that we don't
			// block the inbound queue, allowing approval/cancel messages to be read.
			go a.processMessage(ctx, msg)
		}
	}
}

func (a *AgentLoop) processMessage(ctx context.Context, msg chat.Inbound) {
	// Quick heuristic: if user asks the agent to remember something explicitly,
	// store it in today's note and reply immediately without calling the LLM.
	trimmed := strings.TrimSpace(msg.Content)
	rememberRe := rememberRE
	if matches := rememberRe.FindStringSubmatch(trimmed); len(matches) == 2 {
		note := matches[1]
		if err := a.memory.AppendToday(note); err != nil {
			log.Printf("error appending to memory: %v", err)
		}
		out := chat.Outbound{Channel: msg.Channel, ChatID: msg.ChatID, Content: "OK, I've remembered that."}
		select {
		case a.hub.Out <- out:
		default:
			log.Println("Outbound channel full, dropping message")
		}
		// Only save session for interactive channels, not system triggers.
		if !isSystemChannel(msg.Channel) {
			sess := a.sessions.GetOrCreate(msg.Channel + ":" + msg.ChatID)
			sess.AddMessage("user", msg.Content)
			sess.AddMessage("assistant", "OK, I've remembered that.")
			if err := a.sessions.Save(sess); err != nil {
				log.Printf("error saving session: %v", err)
			}
		}
		return
	}

	// Set tool context (so message tool knows channel+chat)
	if mt := a.tools.Get("message"); mt != nil {
		if mtool, ok := mt.(interface{ SetContext(string, string) }); ok {
			mtool.SetContext(msg.Channel, msg.ChatID)
		}
	}
	if sft := a.tools.Get("send_file"); sft != nil {
		if sftool, ok := sft.(interface{ SetContext(string, string) }); ok {
			sftool.SetContext(msg.Channel, msg.ChatID)
		}
	}
	if et := a.tools.Get("exec"); et != nil {
		if etool, ok := et.(interface{ SetContext(string, string) }); ok {
			etool.SetContext(msg.Channel, msg.ChatID)
		}
	}
	if ct := a.tools.Get("cron"); ct != nil {
		if ctool, ok := ct.(interface{ SetContext(string, string) }); ok {
			ctool.SetContext(msg.Channel, msg.ChatID)
		}
	}

	// Build messages from session, long-term memory, and recent memory.
	// System channels (heartbeat, cron) get a blank ephemeral session so
	// their history never accumulates and bloats the context window.
	var sess *session.Session
	if isSystemChannel(msg.Channel) {
		sess = &session.Session{Key: msg.Channel + ":" + msg.ChatID}
	} else {
		sess = a.sessions.GetOrCreate(msg.Channel + ":" + msg.ChatID)
	}
	// get file-backed memory context (long-term + today)
	memCtx, _ := a.memory.GetMemoryContext()
	memories := a.memory.Recent(5)
	messages := a.context.BuildMessages(sess.GetHistory(), msg.Content, msg.Channel, msg.ChatID, memCtx, memories)

	iteration := 0
	finalContent := ""
	lastToolResult := ""
	toolDefs := a.tools.Definitions()
	for iteration < a.maxIterations {
		iteration++
		messages = pruneOlderToolMessages(messages)
		resp, err := a.provider.Chat(ctx, messages, toolDefs, a.model)
		if err != nil {
			log.Printf("provider error: %v", err)
			finalContent = "Sorry, I encountered an error while processing your request."
			break
		}

		if resp.HasToolCalls {
			// append assistant message with tool_calls attached
			messages = append(messages, providers.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
			// execute each tool call and return results with "tool" role
			for _, tc := range resp.ToolCalls {
				if a.enableToolActivity {
					sendChannelNotification(a.hub, msg.Channel, msg.ChatID,
						formatToolActivity(tc.Name, tc.Arguments))
				}

				start := time.Now()
				toolCtx := chat.WithContext(ctx, msg.Channel, msg.ChatID)

				var res string
				var err error
				if guardErr := a.checkPlanGuard(tc.Name, tc.Arguments); guardErr != nil {
					res = "(tool error) " + guardErr.Error()
					err = guardErr
				} else {
					res, err = a.tools.Execute(toolCtx, tc.Name, tc.Arguments)
				}
				elapsed := time.Since(start).Round(time.Millisecond)

				if err != nil {
					if a.enableToolActivity {
						sendChannelNotification(a.hub, msg.Channel, msg.ChatID,
							fmt.Sprintf("📢 %s failed (%s): %v", tc.Name, elapsed, err))
					}
					if res == "" {
						res = "(tool error) " + err.Error()
					}
				} else {
					if a.enableToolActivity {
						sendChannelNotification(a.hub, msg.Channel, msg.ChatID,
							fmt.Sprintf("📢 %s done (%s)", tc.Name, elapsed))
					}
				}
				lastToolResult = res
				messages = append(messages, providers.Message{Role: "tool", Content: res, ToolCallID: tc.ID})
			}
			// loop again
			continue
		} else {
			finalContent = resp.Content
			break
		}
	}

	if finalContent == "" && lastToolResult != "" {
		finalContent = lastToolResult
	} else if finalContent == "" {
		finalContent = "I've completed processing but have no response to give."
	}

	// Save session for interactive channels only.
	// System channels (heartbeat, cron) are stateless triggers — their
	// history must not be persisted, otherwise the file grows unboundedly.
	if !isSystemChannel(msg.Channel) {
		sess.AddMessage("user", msg.Content)
		sess.AddMessage("assistant", finalContent)
		if err := a.sessions.Save(sess); err != nil {
			log.Printf("error saving session: %v", err)
		}
	}

	out := chat.Outbound{Channel: msg.Channel, ChatID: msg.ChatID, Content: finalContent}
	select {
	case a.hub.Out <- out:
	default:
		log.Println("Outbound channel full, dropping message")
	}
}

// ProcessDirect sends a message directly to the provider and returns the response.
// It supports tool calling - if the model requests tools, they will be executed.
func (a *AgentLoop) ProcessDirect(content string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Set tool context so message/cron tools know the originating channel,
	// matching what Run() does for hub-based messages.
	if mt := a.tools.Get("message"); mt != nil {
		if mtool, ok := mt.(interface{ SetContext(string, string) }); ok {
			mtool.SetContext("cli", "direct")
		}
	}
	if sft := a.tools.Get("send_file"); sft != nil {
		if sftool, ok := sft.(interface{ SetContext(string, string) }); ok {
			sftool.SetContext("cli", "direct")
		}
	}
	if et := a.tools.Get("exec"); et != nil {
		if etool, ok := et.(interface{ SetContext(string, string) }); ok {
			etool.SetContext("cli", "direct")
		}
	}
	if ct := a.tools.Get("cron"); ct != nil {
		if ctool, ok := ct.(interface{ SetContext(string, string) }); ok {
			ctool.SetContext("cli", "direct")
		}
	}

	// Build full context (bootstrap files, skills, memory) just like the main loop
	memCtx, _ := a.memory.GetMemoryContext()
	memories := a.memory.Recent(5)
	messages := a.context.BuildMessages(nil, content, "cli", "direct", memCtx, memories)

	// Support tool calling iterations (similar to main loop)
	var lastToolResult string
	for iteration := 0; iteration < a.maxIterations; iteration++ {
		messages = pruneOlderToolMessages(messages)
		resp, err := a.provider.Chat(ctx, messages, a.tools.Definitions(), a.model)
		if err != nil {
			return "", err
		}

		if !resp.HasToolCalls {
			// No tool calls, return the response (fall back to last tool result if empty)
			if resp.Content != "" {
				return resp.Content, nil
			}
			if lastToolResult != "" {
				return lastToolResult, nil
			}
			return resp.Content, nil
		}

		// Execute tool calls
		messages = append(messages, providers.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			var result string
			var err error
			if guardErr := a.checkPlanGuard(tc.Name, tc.Arguments); guardErr != nil {
				result = "(tool error) " + guardErr.Error()
			} else {
				toolCtx := chat.WithContext(ctx, "cli", "direct")
				result, err = a.tools.Execute(toolCtx, tc.Name, tc.Arguments)
				if err != nil {
					result = "(tool error) " + err.Error()
				}
			}
			lastToolResult = result
			messages = append(messages, providers.Message{Role: "tool", Content: result, ToolCallID: tc.ID})
		}
	}

	return "Max iterations reached without final response", nil
}

// formatToolActivity provides a clean, user-friendly summary of tool execution arguments
// instead of posting large raw JSON payloads (e.g. whole files or long commands).
func formatToolActivity(name string, args map[string]interface{}) string {
	if name == "filesystem" {
		action, _ := args["action"].(string)
		path, _ := args["path"].(string)
		if action != "" && path != "" {
			return fmt.Sprintf("🤖 Running: filesystem %s (%s)", action, path)
		}
	} else if name == "exec" {
		cmd, _ := args["command"].(string)
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		if cmd != "" {
			return fmt.Sprintf("🤖 Running: exec `%s`", cmd)
		}
	} else if name == "web" {
		u, _ := args["url"].(string)
		if u != "" {
			return fmt.Sprintf("🤖 Running: web fetch (%s)", u)
		}
	} else if name == "web_search" {
		q, _ := args["query"].(string)
		if q != "" {
			return fmt.Sprintf("🤖 Running: search `%s`", q)
		}
	} else if name == "write_memory" || name == "read_memory" || name == "edit_memory" {
		target, _ := args["target"].(string)
		if target != "" {
			return fmt.Sprintf("🤖 Running: memory %s (%s)", name, target)
		}
	}

	// Fallback to a truncated JSON representation
	b, _ := json.Marshal(args)
	argStr := string(b)
	if len(argStr) > 80 {
		argStr = argStr[:77] + "..."
	}
	return fmt.Sprintf("🤖 Running: %s %s", name, argStr)
}

// pruneOlderToolMessages identifies tool result messages that were generated
// in previous turns of the active execution loop, and truncates their content
// if it exceeds 1024 characters. This prevents context bloat during long runs.
func pruneOlderToolMessages(msgs []providers.Message) []providers.Message {
	lastAssistantIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			lastAssistantIdx = i
			break
		}
	}

	result := make([]providers.Message, len(msgs))
	for i, m := range msgs {
		if m.Role == "tool" && i < lastAssistantIdx {
			if len(m.Content) > 1024 {
				m.Content = fmt.Sprintf("[Tool result truncated to save context. Original size: %d bytes. Content is saved/available locally if needed.]", len(m.Content))
			}
		}
		result[i] = m
	}
	return result
}

// checkPlanGuard checks if PLAN.md exists in the workspace before allowing modifying/active tool calls.
func (a *AgentLoop) checkPlanGuard(name string, args map[string]interface{}) error {
	planPath := filepath.Join(a.workspace, "PLAN.md")
	if _, err := os.Stat(planPath); err == nil {
		return nil
	}

	// PLAN.md does not exist. Allow only read/list filesystem actions, memory, list/read skill, and message tools.
	if name == "filesystem" {
		action, _ := args["action"].(string)
		path, _ := args["path"].(string)
		if action == "read" || action == "list" {
			return nil
		}
		if action == "write" && strings.Contains(strings.ToLower(path), "plan.md") {
			return nil
		}
		return fmt.Errorf("you must first create a 'PLAN.md' file in the workspace detailing your objective, steps, and status (using the filesystem write tool) before modifying other files")
	}

	if name == "message" || name == "read_memory" || name == "write_memory" || name == "edit_memory" || name == "list_memory" || name == "delete_memory" {
		return nil
	}
	if name == "list_skills" || name == "read_skill" {
		return nil
	}

	return fmt.Errorf("you must first create a 'PLAN.md' file in the workspace detailing your objective, steps, and status (using the filesystem write tool) before calling the '%s' tool", name)
}



