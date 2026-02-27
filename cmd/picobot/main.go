package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"path/filepath"
	"strings"

	"log"

	"github.com/local/picobot/internal/agent"
	"github.com/local/picobot/internal/agent/memory"
	"github.com/local/picobot/internal/channels"
	"github.com/local/picobot/internal/chat"
	"github.com/local/picobot/internal/config"
	"github.com/local/picobot/internal/cron"
	"github.com/local/picobot/internal/heartbeat"
	"github.com/local/picobot/internal/providers"
)

const version = "0.1.5"

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "picobot",
		Short: "picobot — lightweight clawbot in Go",
	}

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("🤖 picobot v%s\n", version)
		},
	})

	onboardCmd := &cobra.Command{
		Use:   "onboard",
		Short: "Create default config and workspace",
		Run: func(cmd *cobra.Command, args []string) {
			cfgPath, workspacePath, err := config.Onboard()
			if err != nil {
				fmt.Fprintf(os.Stderr, "onboard failed: %v\n", err)
				return
			}
			fmt.Printf("Wrote config to %s\nInitialized workspace at %s\n", cfgPath, workspacePath)
		},
	}

	rootCmd.AddCommand(onboardCmd)

	// channels command — connect and configure messaging channels interactively.
	channelsCmd := &cobra.Command{
		Use:   "channels",
		Short: "Manage channel connections (Telegram, Discord, WhatsApp)",
	}

	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Interactively connect a channel (Telegram, Discord, or WhatsApp)",
		Run: func(cmd *cobra.Command, args []string) {
			reader := bufio.NewReader(os.Stdin)

			fmt.Println("Which channel would you like to connect?")
			fmt.Println()
			fmt.Println("  1) Telegram")
			fmt.Println("  2) Discord")
			fmt.Println("  3) WhatsApp")
			fmt.Println()
			fmt.Print("Enter 1, 2 or 3: ")

			choice, _ := reader.ReadString('\n')
			choice = strings.TrimSpace(strings.ToLower(choice))

			cfg, err := config.LoadConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
				return
			}
			cfgPath, _, err := config.ResolveDefaultPaths()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to resolve config path: %v\n", err)
				return
			}

			switch choice {
			case "1", "telegram":
				setupTelegramInteractive(reader, cfg, cfgPath)
			case "2", "discord":
				setupDiscordInteractive(reader, cfg, cfgPath)
			case "3", "whatsapp":
				setupWhatsAppInteractive(cfg, cfgPath)
			default:
				fmt.Fprintf(os.Stderr, "invalid choice %q — please enter 1, 2 or 3\n", choice)
			}
		},
	}

	channelsCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(channelsCmd)

	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Run a single-shot agent query (use -m)",
		Run: func(cmd *cobra.Command, args []string) {
			msg, _ := cmd.Flags().GetString("message")
			modelFlag, _ := cmd.Flags().GetString("model")
			if msg == "" {
				fmt.Println("Specify a message with -m \"your message\"")
				return
			}

			hub := chat.NewHub(100)
			cfg, _ := config.LoadConfig()
			var provider providers.LLMProvider
			if cfg.Providers.OpenAI != nil && cfg.Providers.OpenAI.APIKey != "" {
				provider = providers.NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, cfg.Agents.Defaults.RequestTimeoutS)
			} else {
				provider = providers.NewStubProvider()
			}

			// choose model: flag > config default > provider default
			model := modelFlag
			if model == "" && cfg.Agents.Defaults.Model != "" {
				model = cfg.Agents.Defaults.Model
			}
			if model == "" {
				model = provider.GetDefaultModel()
			}

			maxIter := cfg.Agents.Defaults.MaxToolIterations
			if maxIter <= 0 {
				maxIter = 100
			}
			ag := agent.NewAgentLoop(hub, provider, model, maxIter, cfg.Agents.Defaults.Workspace, nil)

			resp, err := ag.ProcessDirect(msg, 60*time.Second)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
				return
			}
			fmt.Fprintln(cmd.OutOrStdout(), resp)
		},
	}
	agentCmd.Flags().StringP("message", "m", "", "Message to send to the agent")
	agentCmd.Flags().StringP("model", "M", "", "Model to use (overrides config/provider default)")
	rootCmd.AddCommand(agentCmd)

	gatewayCmd := &cobra.Command{
		Use:   "gateway",
		Short: "Start long-running gateway (agent, telegram, heartbeat)",
		Run: func(cmd *cobra.Command, args []string) {
			hub := chat.NewHub(200)
			cfg, _ := config.LoadConfig()
			provider := providers.NewProviderFromConfig(cfg)

			// choose model: flag > config > provider default
			modelFlag, _ := cmd.Flags().GetString("model")
			model := modelFlag
			if model == "" && cfg.Agents.Defaults.Model != "" {
				model = cfg.Agents.Defaults.Model
			}
			if model == "" {
				model = provider.GetDefaultModel()
			}

			// create scheduler with fire callback that routes back through the agent loop, so the LLM can process the reminder and respond naturally to the user.
			scheduler := cron.NewScheduler(func(job cron.Job) {
				log.Printf("cron fired: %s — %s", job.Name, job.Message)
				hub.In <- chat.Inbound{
					Channel:  job.Channel,
					SenderID: "cron",
					ChatID:   job.ChatID,
					Content:  fmt.Sprintf("[Scheduled reminder fired] %s — Please relay this to the user in a friendly way.", job.Message),
				}
			})

			maxIter := cfg.Agents.Defaults.MaxToolIterations
			if maxIter <= 0 {
				maxIter = 100
			}
			ag := agent.NewAgentLoop(hub, provider, model, maxIter, cfg.Agents.Defaults.Workspace, scheduler)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// start agent loop
			go ag.Run(ctx)

			// start cron scheduler
			go scheduler.Start(ctx.Done())

			// start heartbeat
			hbInterval := time.Duration(cfg.Agents.Defaults.HeartbeatIntervalS) * time.Second
			if hbInterval <= 0 {
				hbInterval = 60 * time.Second
			}
			heartbeat.StartHeartbeat(ctx, cfg.Agents.Defaults.Workspace, hbInterval, hub)

			// start telegram if enabled
			if cfg.Channels.Telegram.Enabled {
				if err := channels.StartTelegram(ctx, hub, cfg.Channels.Telegram.Token, cfg.Channels.Telegram.AllowFrom); err != nil {
					fmt.Fprintf(os.Stderr, "failed to start telegram: %v\n", err)
				}
			}

			// start discord if enabled
			if cfg.Channels.Discord.Enabled {
				if err := channels.StartDiscord(ctx, hub, cfg.Channels.Discord.Token, cfg.Channels.Discord.AllowFrom); err != nil {
					fmt.Fprintf(os.Stderr, "failed to start discord: %v\n", err)
				}
			}

			// start whatsapp if enabled
			if cfg.Channels.WhatsApp.Enabled {
				dbPath := cfg.Channels.WhatsApp.DBPath
				if dbPath == "" {
					dbPath = "~/.picobot/whatsapp.db"
				}
				// Expand home directory
				if strings.HasPrefix(dbPath, "~/") {
					home, _ := os.UserHomeDir()
					dbPath = filepath.Join(home, dbPath[2:])
				}
				if err := channels.StartWhatsApp(ctx, hub, dbPath, cfg.Channels.WhatsApp.AllowFrom); err != nil {
					fmt.Fprintf(os.Stderr, "failed to start whatsapp: %v\n", err)
				}
			}

			// start hub router after all channels have subscribed.
			// This routes outbound messages from hub.Out to each channel's
			// dedicated queue, preventing competing reads when multiple channels
			// are active simultaneously.
			hub.StartRouter(ctx)

			// wait for signal
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			fmt.Println("shutting down gateway")
			cancel()
		},
	}
	gatewayCmd.Flags().StringP("model", "M", "", "Model to use (overrides config/provider default)")
	rootCmd.AddCommand(gatewayCmd)

	// memory subcommands: read, append, write, recent
	memoryCmd := &cobra.Command{
		Use:   "memory",
		Short: "Inspect or modify workspace memory files",
	}

	readCmd := &cobra.Command{
		Use:   "read [today|long]",
		Short: "Read memory (today or long-term)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			target := args[0]
			cfg, _ := config.LoadConfig()
			ws := cfg.Agents.Defaults.Workspace
			if ws == "" {
				ws = "~/.picobot/workspace"
			}
			home, _ := os.UserHomeDir()
			if strings.HasPrefix(ws, "~/") {
				ws = filepath.Join(home, ws[2:])
			}
			mem := memory.NewMemoryStoreWithWorkspace(ws, 100)
			switch target {
			case "today":
				out, _ := mem.ReadToday()
				fmt.Fprintln(cmd.OutOrStdout(), out)
			case "long":
				out, _ := mem.ReadLongTerm()
				fmt.Fprintln(cmd.OutOrStdout(), out)
			default:
				fmt.Fprintln(cmd.ErrOrStderr(), "unknown target: "+target)
			}
		},
	}

	appendCmd := &cobra.Command{
		Use:   "append [today|long] -c <content>",
		Short: "Append content to today's note or long-term memory",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			target := args[0]
			content, _ := cmd.Flags().GetString("content")
			if content == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "-c content required")
				return
			}
			cfg, _ := config.LoadConfig()
			ws := cfg.Agents.Defaults.Workspace
			if ws == "" {
				ws = "~/.picobot/workspace"
			}
			home, _ := os.UserHomeDir()
			if strings.HasPrefix(ws, "~/") {
				ws = filepath.Join(home, ws[2:])
			}
			mem := memory.NewMemoryStoreWithWorkspace(ws, 100)
			switch target {
			case "today":
				if err := mem.AppendToday(content); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "append failed:", err)
					return
				}
				fmt.Fprintln(cmd.OutOrStdout(), "appended to today")
			case "long":
				lt, err := mem.ReadLongTerm()
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "append long failed:", err)
					return
				}
				if err := mem.WriteLongTerm(lt + "\n" + content); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "append long failed:", err)
					return
				}
				fmt.Fprintln(cmd.OutOrStdout(), "appended to long-term memory")
			default:
				fmt.Fprintln(cmd.ErrOrStderr(), "unknown target:", target)
			}
		},
	}
	appendCmd.Flags().StringP("content", "c", "", "Content to append")

	writeCmd := &cobra.Command{
		Use:   "write long -c <content>",
		Short: "Write (overwrite) long-term MEMORY.md",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if args[0] != "long" {
				fmt.Fprintln(os.Stderr, "write currently only supports 'long'")
				return
			}
			content, _ := cmd.Flags().GetString("content")
			if content == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "-c content required")
				return
			}
			cfg, _ := config.LoadConfig()
			ws := cfg.Agents.Defaults.Workspace
			if ws == "" {
				ws = "~/.picobot/workspace"
			}
			home, _ := os.UserHomeDir()
			if strings.HasPrefix(ws, "~/") {
				ws = filepath.Join(home, ws[2:])
			}
			mem := memory.NewMemoryStoreWithWorkspace(ws, 100)
			if err := mem.WriteLongTerm(content); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "write failed:", err)
				return
			}
			fmt.Fprintln(cmd.OutOrStdout(), "wrote long-term memory")
		},
	}
	writeCmd.Flags().StringP("content", "c", "", "Content to write")

	recentCmd := &cobra.Command{
		Use:   "recent -days N",
		Short: "Show recent N days' notes",
		Run: func(cmd *cobra.Command, args []string) {
			days, _ := cmd.Flags().GetInt("days")
			cfg, _ := config.LoadConfig()
			ws := cfg.Agents.Defaults.Workspace
			if ws == "" {
				ws = "~/.picobot/workspace"
			}
			home, _ := os.UserHomeDir()
			if strings.HasPrefix(ws, "~/") {
				ws = filepath.Join(home, ws[2:])
			}
			mem := memory.NewMemoryStoreWithWorkspace(ws, 100)
			out, _ := mem.GetRecentMemories(days)
			fmt.Fprintln(cmd.OutOrStdout(), out)
		},
	}
	recentCmd.Flags().IntP("days", "d", 1, "Number of days to include")

	memoryCmd.AddCommand(readCmd)
	memoryCmd.AddCommand(appendCmd)
	memoryCmd.AddCommand(writeCmd)
	memoryCmd.AddCommand(recentCmd)

	// rank subcommand: rank recent memories by relevance to a query
	rankCmd := &cobra.Command{
		Use:   "rank -q <query>",
		Short: "Rank recent memories relative to a query",
		Run: func(cmd *cobra.Command, args []string) {
			q, _ := cmd.Flags().GetString("query")
			if q == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "-q query required")
				return
			}
			top, _ := cmd.Flags().GetInt("top")
			verbose, _ := cmd.Flags().GetBool("verbose")
			cfg, _ := config.LoadConfig()
			ws := cfg.Agents.Defaults.Workspace
			if ws == "" {
				ws = "~/.picobot/workspace"
			}
			home, _ := os.UserHomeDir()
			if strings.HasPrefix(ws, "~/") {
				ws = filepath.Join(home, ws[2:])
			}
			mem := memory.NewMemoryStoreWithWorkspace(ws, 100)
			// Build memory items from today's file (split into lines) and long-term memory
			items := make([]memory.MemoryItem, 0)
			if td, err := mem.ReadToday(); err == nil && td != "" {
				for _, line := range strings.Split(td, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					// strip leading timestamp [2026-02-07...] if present
					if idx := strings.Index(line, "] "); idx != -1 && strings.HasPrefix(line, "[") {
						line = strings.TrimSpace(line[idx+2:])
					}
					items = append(items, memory.MemoryItem{Kind: "today", Text: line})
				}
			}
			if lt, err := mem.ReadLongTerm(); err == nil && lt != "" {
				for _, line := range strings.Split(lt, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					items = append(items, memory.MemoryItem{Kind: "long", Text: line})
				}
			}
			provider := providers.NewProviderFromConfig(cfg)
			var logger *log.Logger
			if verbose {
				logger = log.New(cmd.OutOrStdout(), "ranker: ", 0)
			}
			ranker := memory.NewLLMRankerWithLogger(provider, provider.GetDefaultModel(), logger)
			res := ranker.Rank(q, items, top)
			for i, m := range res {
				fmt.Fprintf(cmd.OutOrStdout(), "%d: %s (%s)\n", i+1, m.Text, m.Kind)
			}
		},
	}
	rankCmd.Flags().StringP("query", "q", "", "Query to rank memories against")
	rankCmd.Flags().IntP("top", "k", 5, "Number of top memories to show")
	rankCmd.Flags().BoolP("verbose", "v", false, "Enable verbose diagnostic logging (to stdout)")
	memoryCmd.AddCommand(rankCmd)

	rootCmd.AddCommand(memoryCmd)
	return rootCmd
}

func main() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// promptLine prints a prompt and returns the trimmed input line.
func promptLine(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// parseAllowFrom splits a comma-separated string into a trimmed slice.
// Returns an empty slice (not nil) if the input is blank.
func parseAllowFrom(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func setupTelegramInteractive(reader *bufio.Reader, cfg config.Config, cfgPath string) {
	fmt.Println()
	fmt.Println("=== Telegram Setup ===")
	fmt.Println()
	fmt.Println("You need a bot token from @BotFather on Telegram:")
	fmt.Println("  1. Message @BotFather on Telegram")
	fmt.Println("  2. Send /newbot and follow the prompts")
	fmt.Println("  3. Copy the token it gives you")
	fmt.Println()

	token := promptLine(reader, "Bot token: ")
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: token cannot be empty")
		return
	}

	fmt.Println()
	fmt.Println("To restrict who can message your bot, enter your Telegram user ID.")
	fmt.Println("Find it by messaging @userinfobot on Telegram.")
	fmt.Println("Leave blank to allow everyone.")
	fmt.Println()

	allowFromStr := promptLine(reader, "Allowed user IDs (comma-separated, blank = everyone): ")
	allowFrom := parseAllowFrom(allowFromStr)

	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.Token = token
	cfg.Channels.Telegram.AllowFrom = allowFrom

	if err := config.SaveConfig(cfg, cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("Telegram configured! Run 'picobot gateway' to start.")
}

func setupDiscordInteractive(reader *bufio.Reader, cfg config.Config, cfgPath string) {
	fmt.Println()
	fmt.Println("=== Discord Setup ===")
	fmt.Println()
	fmt.Println("You need a bot token from the Discord Developer Portal:")
	fmt.Println("  1. Go to https://discord.com/developers/applications")
	fmt.Println("  2. Create an application → Bot → Reset Token")
	fmt.Println("  3. Enable \"Message Content Intent\" under Privileged Gateway Intents")
	fmt.Println("  4. Invite the bot to your server via OAuth2 → URL Generator")
	fmt.Println("  5. Copy the token and paste it below")
	fmt.Println()

	token := promptLine(reader, "Bot token: ")
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: token cannot be empty")
		return
	}

	fmt.Println()
	fmt.Println("To restrict who can message your bot, enter Discord user IDs.")
	fmt.Println("Enable Developer Mode (Settings → Advanced) then right-click your name → Copy User ID.")
	fmt.Println("Leave blank to allow everyone.")
	fmt.Println()

	allowFromStr := promptLine(reader, "Allowed user IDs (comma-separated, blank = everyone): ")
	allowFrom := parseAllowFrom(allowFromStr)

	cfg.Channels.Discord.Enabled = true
	cfg.Channels.Discord.Token = token
	cfg.Channels.Discord.AllowFrom = allowFrom

	if err := config.SaveConfig(cfg, cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "failed to save config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("Discord configured! Run 'picobot gateway' to start.")
}

func setupWhatsAppInteractive(cfg config.Config, cfgPath string) {
	fmt.Println()
	fmt.Println("=== WhatsApp Setup ===")
	fmt.Println()

	dbPath := cfg.Channels.WhatsApp.DBPath
	if dbPath == "" {
		dbPath = "~/.picobot/whatsapp.db"
	}
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(dbPath, "~/") {
		dbPath = filepath.Join(home, dbPath[2:])
	}

	if err := channels.SetupWhatsApp(dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "WhatsApp setup failed: %v\n", err)
		return
	}

	cfg.Channels.WhatsApp.Enabled = true
	cfg.Channels.WhatsApp.DBPath = dbPath
	if saveErr := config.SaveConfig(cfg, cfgPath); saveErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save config: %v\n", saveErr)
	} else {
		fmt.Printf("Config updated: whatsapp enabled, dbPath set to %s\n", dbPath)
	}

	fmt.Println("\nWhatsApp setup complete! Run 'picobot gateway' to start.")
}
