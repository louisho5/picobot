# Picobot Internal Architecture

This document provides a detailed overview of the Picobot framework's internal mechanisms, including memory management, task orchestration, and error handling.

## Heartbeat Mechanism
Picobot features a periodic heartbeat check (`internal/heartbeat/service.go`) that runs at a configurable interval (default: 60s). It monitors `HEARTBEAT.md` in the workspace.
- **Polling**: On every tick, it reads the content of `HEARTBEAT.md`.
- **Natural Language Tasks**: If the file is not empty, it pushes the content as a system message into the **Chat Hub**.
- **Agent Processing**: The agent receives these tasks as if they were a user prompt, allowing it to perform scheduled actions (like checking the weather or sending a reminder) using natural language.

## Persistence and Backup
Picobot relies on a "crash-only" design philosophy where all state is persisted to the local filesystem.
- **Sessions**: Chat history is stored as JSON files in `workspace/sessions/`.
- **Memory**: Daily notes and long-term knowledge are stored in `workspace/memory/`.
- **Skills**: Skill definitions are stored in `workspace/skills/`.
- **Resilience**: This file-based persistence acts as a continuous backup. If the process is killed or the server restarts, Picobot reloads its entire state from these files, ensuring no conversation context or learned knowledge is lost.

## Context Window Memory Implementation
Picobot manages the LLM context window through a tiered memory system:
- **Short-Term (Session Window)**: A sliding window of the last 50 messages (defined by `MaxHistorySize` in `internal/session/manager.go`). This provides immediate conversational context.
- **Mid-Term (Daily Notes)**: Append-only notes stored by date (`YYYY-MM-DD.md`). These capture the events and information of the day.
- **Long-Term (Strategic Memory)**: Facts and knowledge stored in `MEMORY.md`.
- **Ranked Recall (RAG)**: To prevent context overflow, Picobot uses an `LLMRanker` to perform semantic search over recent and long-term memories, injecting only the top-K most relevant items into the current prompt.

## Task Management Queue Implementation
The framework uses a central **Chat Hub** (`internal/chat/chat.go`) to orchestrate message flow.
- **Go Channels**: The hub uses buffered Go channels (`In` and `Out`) for asynchronous communication between the agent and various communication channels (e.g., Telegram).
- **Sequential Processing**: The **Agent Loop** (`internal/agent/loop.go`) pulls messages from the `In` channel one by one. This sequential processing ensures that the agent's internal state and memory remain consistent during complex tool-calling sequences.

## Tool Usage and Orchestration
Picobot implements a sophisticated tool-calling orchestration loop:
- **Registry**: All available actions (filesystem, exec, web, memory, skills) are registered in a central registry and shared with the LLM via standard tool definitions.
- **Iterative Loop**: When the LLM requests tool execution, the agent loop executes the tools, appends the results to the conversation history, and re-queries the LLM.
- **Max Iterations**: A `maxToolIterations` limit (default: 100) prevents infinite loops if an LLM gets stuck in a recursive tool-calling pattern.

## Task Restart Implementation
Picobot is designed to be managed by external process managers like **Docker** or **systemd**.
- **External Responsibility**: The framework itself does not handle process-level restarts; it relies on the host environment (e.g., `--restart unless-stopped` in Docker).
- **Polling Recovery**: The Telegram polling mechanism (`internal/channels/telegram.go`) tracks its `offset`. Upon restart, it resumes polling from the last known message ID, ensuring that messages sent while the bot was offline are eventually processed.
- **State Restoration**: All session and memory state is reloaded from disk on startup.

## Dropped Tasks and Backpressure
Picobot distinguishes between inbound and outbound flow to maintain responsiveness:
- **Blocking Inbound (Backpressure)**: Pushing a message to the hub's `In` channel is a blocking operation. If the agent loop is overwhelmed and the buffer is full, the producers (Telegram poller, Cron, Heartbeat) will block. This ensures that no incoming tasks are lost, but instead "pressure" is applied back to the source.
- **Non-Blocking Outbound (Shedding)**: Sending a message to the hub's `Out` channel is non-blocking. If the outbound buffer is full (indicating a slow or disconnected communication channel), the agent **drops** the response to ensure the core loop remains responsive for other tasks. This prevents a single slow client from freezing the entire agent.

## MCP Protocol Feasibility
Picobot's modular tool architecture makes it highly suitable for implementing the **Model Context Protocol (MCP)**.

### Implementation Path
1. **Config Extension**: Add an `mcpServers` map to `Config` in `internal/config/schema.go`.
2. **MCP Tool Wrapper**: Create a new tool type (`internal/agent/tools/mcp.go`) that implements the `Tool` interface. This wrapper would:
    - Establish a connection to the MCP server (SSE or Stdio).
    - Fetch tool definitions from the MCP server during initialization.
    - Proxy `Execute` calls to the remote MCP server.
3. **Dynamic Registration**: Update `NewAgentLoop` in `internal/agent/loop.go` to iterate over the `mcpServers` configuration and register each remote tool into the registry.

### Sample Configuration
```json
{
  "mcpServers": {
    "any_tools": {
      "url": "https://any_tools/mcp",
      "headers": {
        "API_KEY": "YOUR_API_KEY"
      }
    }
  }
}
```
Given Picobot's Go foundation, using a standard MCP Go SDK would allow for a lightweight and robust integration, keeping the binary small while significantly expanding its tool capabilities.
