package agent

import (
	"context"
	"log"
	"os"
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

// AgentLoop is the core processing loop; it holds an LLM provider, tools, sessions and context builder.
type AgentLoop struct {
	hub           *chat.Hub
	provider      providers.LLMProvider
	tools         *tools.Registry
	sessions      *session.SessionManager
	context       *ContextBuilder
	memory        *memory.MemoryStore
	model         string
	maxIterations int
	running       bool
	mcpManager    *mcp.Manager
}

// NewAgentLoop creates a new AgentLoop with the given provider.
func NewAgentLoop(b *chat.Hub, provider providers.LLMProvider, model string, maxIterations int, workspace string, scheduler *cron.Scheduler, mcpCfg config.MCPConfig) *AgentLoop {
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

	fsTool, err := tools.NewFilesystemTool(workspace)
	if err != nil {
		log.Fatalf("failed to create filesystem tool: %v", err)
	}
	reg.Register(fsTool)

	reg.Register(tools.NewExecTool(60))
	reg.Register(tools.NewWebTool())
	reg.Register(tools.NewSpawnTool())
	if scheduler != nil {
		reg.Register(tools.NewCronTool(scheduler))
	}

	sm := session.NewSessionManager(workspace)
	ctx := NewContextBuilder(workspace, memory.NewLLMRanker(provider, model), 5)
	mem := memory.NewMemoryStoreWithWorkspace(workspace, 100)
	// register memory tool (needs store instance)
	reg.Register(tools.NewWriteMemoryTool(mem))

	// register skill management tools (share the same os.Root)
	skillMgr := tools.NewSkillManager(root)
	reg.Register(tools.NewCreateSkillTool(skillMgr))
	reg.Register(tools.NewListSkillsTool(skillMgr))
	reg.Register(tools.NewReadSkillTool(skillMgr))
	reg.Register(tools.NewDeleteSkillTool(skillMgr))

	// Initialize MCP manager and register MCP tools
	var mcpManager *mcp.Manager
	if len(mcpCfg.Servers) > 0 {
		mcpManager = mcp.NewManager()
		if err := mcpManager.InitializeServers(mcpCfg); err != nil {
			log.Printf("Failed to initialize MCP servers: %v", err)
		} else {
			mcpAdapter := tools.NewMCPToolAdapter(mcpManager)
			mcpAdapter.RegisterMCPTools(reg)
		}
	}

	return &AgentLoop{hub: b, provider: provider, tools: reg, sessions: sm, context: ctx, memory: mem, model: model, maxIterations: maxIterations, mcpManager: mcpManager}
}

// Run starts processing inbound messages. This is a blocking call until context is canceled.
func (a *AgentLoop) Run(ctx context.Context) {
	a.running = true
	log.Println("Agent loop started")

	// Ensure MCP connections are cleaned up on exit
	defer func() {
		if a.mcpManager != nil {
			a.mcpManager.Close()
		}
	}()

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
				// save to session as well
				session := a.sessions.GetOrCreate(msg.Channel + ":" + msg.ChatID)
				session.AddMessage("user", msg.Content)
				session.AddMessage("assistant", "OK, I've remembered that.")
				a.sessions.Save(session)
				continue
			}

			// Set tool context (so message tool knows channel+chat)
			if mt := a.tools.Get("message"); mt != nil {
				if mtool, ok := mt.(interface{ SetContext(string, string) }); ok {
					mtool.SetContext(msg.Channel, msg.ChatID)
				}
			}
			if ct := a.tools.Get("cron"); ct != nil {
				if ctool, ok := ct.(interface{ SetContext(string, string) }); ok {
					ctool.SetContext(msg.Channel, msg.ChatID)
				}
			}

			// Build messages from session, long-term memory, and recent memory
			session := a.sessions.GetOrCreate(msg.Channel + ":" + msg.ChatID)
			// get file-backed memory context (long-term + today)
			memCtx, _ := a.memory.GetMemoryContext()
			memories := a.memory.Recent(5)
			messages := a.context.BuildMessages(session.GetHistory(), msg.Content, msg.Channel, msg.ChatID, memCtx, memories)

			iteration := 0
			finalContent := ""
			lastToolResult := ""
			toolDefs := a.tools.Definitions()

			for iteration < a.maxIterations {
				iteration++
				resp, err := a.provider.Chat(ctx, messages, toolDefs, a.model)

				if err != nil {
					log.Printf("provider error: %v", err)
					finalContent = "Sorry, I encountered an error while processing your request."
					break
				}

				if resp.HasToolCalls {
					// append assistant message with tool_calls attached
					messages = append(messages, providers.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
					// Execute each tool call and return results with "tool" role
					for _, tc := range resp.ToolCalls {
						res, err := a.tools.Execute(ctx, tc.Name, tc.Arguments)
						if err != nil {
							res = "(tool error) " + err.Error()
						}
						lastToolResult = res
						messages = append(messages, providers.Message{Role: "tool", Content: res, ToolCallID: tc.ID})
					}
					// loop again - typing indicator continues running
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

			// Save session
			session.AddMessage("user", msg.Content)
			session.AddMessage("assistant", finalContent)
			a.sessions.Save(session)

			out := chat.Outbound{Channel: msg.Channel, ChatID: msg.ChatID, Content: finalContent}
			select {
			case a.hub.Out <- out:
			default:
				log.Println("Outbound channel full, dropping message")
			}
		default:
			// idle tick
			time.Sleep(100 * time.Millisecond)
		}
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
			result, err := a.tools.Execute(ctx, tc.Name, tc.Arguments)
			if err != nil {
				result = "(tool error) " + err.Error()
			}
			lastToolResult = result
			messages = append(messages, providers.Message{Role: "tool", Content: result, ToolCallID: tc.ID})
		}
	}

	return "Max iterations reached without final response", nil
}
