package chat

import (
	"context"
	"log"
	"sync"
	"time"
)

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

// Hub provides simple buffered channels for inbound/outbound messages.
//
// When only one channel (e.g. Telegram) is active, goroutines may read from
// Out directly. When multiple channels are active, call Subscribe for each
// channel and then StartRouter so that outbound messages are dispatched to the
// correct handler without competing reads.
type Hub struct {
	In  chan Inbound
	Out chan Outbound

	subMu sync.RWMutex
	subs  map[string]chan Outbound
}

// NewHub constructs a new Hub with the given buffer size.
func NewHub(buffer int) *Hub {
	return &Hub{
		In:   make(chan Inbound, buffer),
		Out:  make(chan Outbound, buffer),
		subs: make(map[string]chan Outbound),
	}
}

// Subscribe registers a named outbound queue and returns a receive-only channel
// that will receive every Outbound message whose Channel field matches name.
// Register all subscribers before calling StartRouter.
func (h *Hub) Subscribe(name string) <-chan Outbound {
	ch := make(chan Outbound, cap(h.Out))
	h.subMu.Lock()
	h.subs[name] = ch
	h.subMu.Unlock()
	return ch
}

// StartRouter reads from Out and dispatches each message to the registered
// subscriber for its channel. Messages for unregistered channels are dropped
// with a warning. This must be called after all subscribers are registered.
func (h *Hub) StartRouter(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case out, ok := <-h.Out:
				if !ok {
					return
				}
				h.subMu.RLock()
				ch, exists := h.subs[out.Channel]
				h.subMu.RUnlock()
				if exists {
					select {
					case ch <- out:
					case <-ctx.Done():
						return
					}
				} else {
					log.Printf("hub: no subscriber for channel %q, dropping outbound message", out.Channel)
				}
			}
		}
	}()
}

// Close closes the channels.
func (h *Hub) Close() {
	close(h.In)
	close(h.Out)
}

var (
	approvalsMu sync.Mutex
	approvals   = make(map[string]chan string)
)

// RegisterApproval registers a pending approval channel for a chat session.
func RegisterApproval(key string, ch chan string) {
	approvalsMu.Lock()
	approvals[key] = ch
	approvalsMu.Unlock()
}

// UnregisterApproval removes a pending approval registration.
func UnregisterApproval(key string) {
	approvalsMu.Lock()
	delete(approvals, key)
	approvalsMu.Unlock()
}

// TriggerApproval forwards user input to a waiting approval channel if one exists.
func TriggerApproval(key string, response string) bool {
	approvalsMu.Lock()
	ch, exists := approvals[key]
	approvalsMu.Unlock()
	if exists {
		select {
		case ch <- response:
			return true
		default:
			return false
		}
	}
	return false
}

type contextKey string
const (
	ChannelKey contextKey = "channel"
	ChatIDKey  contextKey = "chatID"
)

// WithContext returns a new context with channel and chatID values.
func WithContext(ctx context.Context, channel, chatID string) context.Context {
	ctx = context.WithValue(ctx, ChannelKey, channel)
	return context.WithValue(ctx, ChatIDKey, chatID)
}

// FromContext extracts the channel and chatID values from a context.
func FromContext(ctx context.Context) (string, string) {
	ch, _ := ctx.Value(ChannelKey).(string)
	id, _ := ctx.Value(ChatIDKey).(string)
	return ch, id
}
