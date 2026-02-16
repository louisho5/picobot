package chat

import "time"

// Inbound represents an incoming message to the agent.
type Inbound struct {
	Channel   string
	SenderID  string
	ChatID    string
	Content   string
	Timestamp time.Time
	Media     []string
	Metadata  map[string]interface{}
}

// Outbound represents a message produced by the agent.
type Outbound struct {
	Channel  string
	ChatID   string
	Content  string
	ReplyTo  string
	Media    []string
	Metadata map[string]interface{}
}

// Signal represents a generic notification from the agent to channels.
// Channels register to receive signals they're interested in.
type Signal struct {
	Type     string                 // e.g., "processing_started", "processing_done", "error"
	Channel  string                 // Target channel type ("telegram", "email", etc.)
	ChatID   string                 // Specific chat/conversation ID
	Metadata map[string]interface{} // Extensible payload for future use
}

// Hub provides simple buffered channels for inbound/outbound messages.
type Hub struct {
	In      chan Inbound
	Out     chan Outbound
	Signal  chan Signal  // Broadcast channel for signals
}

// NewHub constructs a new Hub with the given buffer size.
func NewHub(buffer int) *Hub {
	return &Hub{
		In:     make(chan Inbound, buffer),
		Out:    make(chan Outbound, buffer),
		Signal: make(chan Signal, buffer),
	}
}

// Close closes the channels.
func (h *Hub) Close() {
	close(h.In)
	close(h.Out)
	close(h.Signal)
}
