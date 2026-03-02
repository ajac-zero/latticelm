package conversation

import (
	"sync"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
)

// Store defines the interface for conversation storage backends.
type Store interface {
	Get(id string) (*Conversation, error)
	Create(id string, model string, messages []api.Message) (*Conversation, error)
	Append(id string, messages ...api.Message) (*Conversation, error)
	Delete(id string) error
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

// Get retrieves a conversation by ID. Returns a deep copy to prevent data races.
func (s *MemoryStore) Get(id string) (*Conversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conv, ok := s.conversations[id]
	if !ok {
		return nil, nil
	}

	// Return a deep copy to prevent data races
	msgsCopy := make([]api.Message, len(conv.Messages))
	copy(msgsCopy, conv.Messages)

	return &Conversation{
		ID:        conv.ID,
		Messages:  msgsCopy,
		Model:     conv.Model,
		CreatedAt: conv.CreatedAt,
		UpdatedAt: conv.UpdatedAt,
	}, nil
}

// Create creates a new conversation with the given messages.
func (s *MemoryStore) Create(id string, model string, messages []api.Message) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Store a copy to prevent external modifications
	msgsCopy := make([]api.Message, len(messages))
	copy(msgsCopy, messages)

	conv := &Conversation{
		ID:        id,
		Messages:  msgsCopy,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.conversations[id] = conv

	// Return a copy
	return &Conversation{
		ID:        id,
		Messages:  messages,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Append adds new messages to an existing conversation.
func (s *MemoryStore) Append(id string, messages ...api.Message) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[id]
	if !ok {
		return nil, nil
	}

	conv.Messages = append(conv.Messages, messages...)
	conv.UpdatedAt = time.Now()

	// Return a deep copy
	msgsCopy := make([]api.Message, len(conv.Messages))
	copy(msgsCopy, conv.Messages)

	return &Conversation{
		ID:        conv.ID,
		Messages:  msgsCopy,
		Model:     conv.Model,
		CreatedAt: conv.CreatedAt,
		UpdatedAt: conv.UpdatedAt,
	}, nil
}

// Delete removes a conversation from the store.
func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.conversations, id)
	return nil
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
