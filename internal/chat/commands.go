package chat

// Command is a chat command handled by the agent loop itself rather than
// sent to the LLM. Channels may advertise these to users (e.g. Telegram's
// command menu).
type Command struct {
	Name        string
	Description string
}

// Commands lists the conversation-management commands.
var Commands = []Command{
	{Name: "new", Description: "Start a new conversation (optionally: /new <title>)"},
	{Name: "chats", Description: "List your conversations"},
	{Name: "switch", Description: "Switch conversation: /switch <number>"},
	{Name: "help", Description: "Show available commands"},
}
