package conversation

import (
	"context"
	"sync"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
)

// OwnerInfo holds the identity metadata stamped onto every conversation.
type OwnerInfo struct {
	OwnerIss string
	OwnerSub string
	TenantID string
}

// Store defines the interface for conversation storage backends.
type Store interface {
	Get(ctx context.Context, id string) (*Conversation, error)
	Create(ctx context.Context, id string, model string, messages []api.Message, owner OwnerInfo) (*Conversation, error)
	Append(ctx context.Context, id string, messages ...api.Message) (*Conversation, error)
	Delete(ctx context.Context, id string) error
	Size() int
	Close() error
}

// MemoryStore manages conversation history in-memory with automatic expiration.
type MemoryStore struct {
	conversations map[string]*Conversation
	mu            sync.RWMutex
	ttl           time.Duration
	done          chan struct{}
}

// Conversation holds the message history for a single conversation thread.
type Conversation struct {
	ID        string
	Messages  []api.Message
	Model     string
	OwnerIss  string
	OwnerSub  string
	TenantID  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// copyMessages returns a deep copy of a Message slice, duplicating nested Content and ToolCalls slices.
func copyMessages(messages []api.Message) []api.Message {
	out := make([]api.Message, len(messages))
	for i, msg := range messages {
		out[i] = msg
		if msg.Content != nil {
			out[i].Content = make([]api.ContentBlock, len(msg.Content))
			copy(out[i].Content, msg.Content)
		}
		if msg.ToolCalls != nil {
			out[i].ToolCalls = make([]api.ToolCall, len(msg.ToolCalls))
			copy(out[i].ToolCalls, msg.ToolCalls)
		}
	}
	return out
}

// copyConversation returns a deep copy of a Conversation.
func copyConversation(conv *Conversation) *Conversation {
	return &Conversation{
		ID:        conv.ID,
		Messages:  copyMessages(conv.Messages),
		Model:     conv.Model,
		OwnerIss:  conv.OwnerIss,
		OwnerSub:  conv.OwnerSub,
		TenantID:  conv.TenantID,
		CreatedAt: conv.CreatedAt,
		UpdatedAt: conv.UpdatedAt,
	}
}

// NewMemoryStore creates an in-memory conversation store with the given TTL.
func NewMemoryStore(ttl time.Duration) *MemoryStore {
	s := &MemoryStore{
		conversations: make(map[string]*Conversation),
		ttl:           ttl,
		done:          make(chan struct{}),
	}

	// Start cleanup goroutine if TTL is set
	if ttl > 0 {
		go s.cleanup()
	}

	return s
}

// Get retrieves a conversation by ID. Returns a deep copy to prevent data races.
func (s *MemoryStore) Get(ctx context.Context, id string) (*Conversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conv, ok := s.conversations[id]
	if !ok {
		return nil, nil
	}

	return copyConversation(conv), nil
}

// Create creates a new conversation with the given messages.
func (s *MemoryStore) Create(ctx context.Context, id string, model string, messages []api.Message, owner OwnerInfo) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	conv := &Conversation{
		ID:        id,
		Messages:  copyMessages(messages),
		Model:     model,
		OwnerIss:  owner.OwnerIss,
		OwnerSub:  owner.OwnerSub,
		TenantID:  owner.TenantID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.conversations[id] = conv

	return copyConversation(conv), nil
}

// Append adds new messages to an existing conversation.
func (s *MemoryStore) Append(ctx context.Context, id string, messages ...api.Message) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[id]
	if !ok {
		return nil, nil
	}

	conv.Messages = append(conv.Messages, copyMessages(messages)...)
	conv.UpdatedAt = time.Now()

	return copyConversation(conv), nil
}

// Delete removes a conversation from the store.
func (s *MemoryStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.conversations, id)
	return nil
}

// cleanup periodically removes expired conversations.
func (s *MemoryStore) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for id, conv := range s.conversations {
				if now.Sub(conv.UpdatedAt) > s.ttl {
					delete(s.conversations, id)
				}
			}
			s.mu.Unlock()
		case <-s.done:
			return
		}
	}
}

// Size returns the number of active conversations.
func (s *MemoryStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.conversations)
}

// Close stops the cleanup goroutine and releases resources.
func (s *MemoryStore) Close() error {
	close(s.done)
	return nil
}
