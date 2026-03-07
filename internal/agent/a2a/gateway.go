// Package a2a implements an Agent-to-Agent gateway that enables communication
// between AI agents running in different sandboxes via a message-passing system.
package a2a

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Message represents an agent-to-agent message.
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`    // sender sandbox name
	To        string    `json:"to"`      // recipient sandbox name
	Type      string    `json:"type"`    // message type (e.g., "request", "response", "event")
	Payload   string    `json:"payload"` // message content (JSON string)
	CreatedAt time.Time `json:"createdAt"`
}

// HandlerFunc is a callback invoked when a message arrives for a subscribed sandbox.
type HandlerFunc func(msg *Message)

// Gateway manages agent-to-agent communication.
type Gateway struct {
	mu       sync.RWMutex
	inboxes  map[string][]Message // sandbox name -> pending messages
	handlers map[string][]HandlerFunc
}

// New creates a new Gateway.
func New() *Gateway {
	return &Gateway{
		inboxes:  make(map[string][]Message),
		handlers: make(map[string][]HandlerFunc),
	}
}

// Send sends a message from one sandbox to another.
func (g *Gateway) Send(ctx context.Context, msg *Message) error {
	if msg.From == "" {
		return fmt.Errorf("message 'from' field is required")
	}
	if msg.To == "" {
		return fmt.Errorf("message 'to' field is required")
	}
	if msg.Type == "" {
		return fmt.Errorf("message 'type' field is required")
	}

	// Assign ID and timestamp if not set.
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	g.mu.Lock()
	g.inboxes[msg.To] = append(g.inboxes[msg.To], *msg)
	// Copy handlers under lock to invoke outside.
	handlers := make([]HandlerFunc, len(g.handlers[msg.To]))
	copy(handlers, g.handlers[msg.To])
	g.mu.Unlock()

	// Invoke handlers outside the lock.
	for _, h := range handlers {
		h(msg)
	}

	return nil
}

// Receive retrieves pending messages for a sandbox and clears the inbox.
func (g *Gateway) Receive(ctx context.Context, sandboxName string) ([]Message, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	msgs := g.inboxes[sandboxName]
	if len(msgs) == 0 {
		return []Message{}, nil
	}

	// Return a copy and clear.
	result := make([]Message, len(msgs))
	copy(result, msgs)
	delete(g.inboxes, sandboxName)

	return result, nil
}

// Peek retrieves pending messages without clearing them.
func (g *Gateway) Peek(ctx context.Context, sandboxName string) ([]Message, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	msgs := g.inboxes[sandboxName]
	if len(msgs) == 0 {
		return []Message{}, nil
	}

	result := make([]Message, len(msgs))
	copy(result, msgs)
	return result, nil
}

// Subscribe registers a handler for messages sent to a sandbox.
func (g *Gateway) Subscribe(sandboxName string, handler HandlerFunc) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.handlers[sandboxName] = append(g.handlers[sandboxName], handler)
}

// Broadcast sends a message to all sandboxes in the targets list.
func (g *Gateway) Broadcast(ctx context.Context, from string, targets []string, msgType, payload string) error {
	if from == "" {
		return fmt.Errorf("'from' field is required")
	}
	if len(targets) == 0 {
		return fmt.Errorf("at least one target is required")
	}
	if msgType == "" {
		return fmt.Errorf("message 'type' field is required")
	}

	for _, target := range targets {
		msg := &Message{
			From:    from,
			To:      target,
			Type:    msgType,
			Payload: payload,
		}
		if err := g.Send(ctx, msg); err != nil {
			return fmt.Errorf("broadcast to %q: %w", target, err)
		}
	}
	return nil
}
