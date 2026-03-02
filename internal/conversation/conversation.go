package conversation

import (
	"sync"
	"time"

	"github.com/yourusername/go-llm-gateway/internal/api"
)

// Store defines the interface for conversation storage backends.
type Store interface {
	Get(id string) (*Conversation, bool)
	Create(id string, model string, messages []api.Message) *Conversation
	Append(id string, messages ...api.Message) (*Conversation, bool)
	Delete(id string)
	Size() int
}

// MemoryStore manages conversation history in-memory with automatic expiration.
type MemoryStore struct {
	conversations map[string]*Conversation
	mu            sync.RWMutex
	ttl           time.Duration
}

// Conversation holds the message history for a single conversation thread.
type Conversation struct {
	ID        string
	Messages  []api.Message
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewMemoryStore creates an in-memory conversation store with the given TTL.
func NewMemoryStore(ttl time.Duration) *MemoryStore {
	s := &MemoryStore{
		conversations: make(map[string]*Conversation),
		ttl:           ttl,
	}
	
	// Start cleanup goroutine if TTL is set
	if ttl > 0 {
		go s.cleanup()
	}
	
	return s
}

// Get retrieves a conversation by ID.
func (s *MemoryStore) Get(id string) (*Conversation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	conv, ok := s.conversations[id]
	return conv, ok
}

// Create creates a new conversation with the given messages.
func (s *MemoryStore) Create(id string, model string, messages []api.Message) *Conversation {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	now := time.Now()
	conv := &Conversation{
		ID:        id,
		Messages:  messages,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}
	
	s.conversations[id] = conv
	return conv
}

// Append adds new messages to an existing conversation.
func (s *MemoryStore) Append(id string, messages ...api.Message) (*Conversation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	conv, ok := s.conversations[id]
	if !ok {
		return nil, false
	}
	
	conv.Messages = append(conv.Messages, messages...)
	conv.UpdatedAt = time.Now()
	
	return conv, true
}

// Delete removes a conversation from the store.
func (s *MemoryStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	delete(s.conversations, id)
}

// cleanup periodically removes expired conversations.
func (s *MemoryStore) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, conv := range s.conversations {
			if now.Sub(conv.UpdatedAt) > s.ttl {
				delete(s.conversations, id)
			}
		}
		s.mu.Unlock()
	}
}

// Size returns the number of active conversations.
func (s *MemoryStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.conversations)
}
