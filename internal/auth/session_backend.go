package auth

import (
	"context"
	"sync"
	"time"
)

// SessionBackend defines the interface for session storage backends.
// Implementations can be in-memory (single instance) or distributed (Redis, etc.)
type SessionBackend interface {
	// Create stores session data with the given ID and TTL.
	Create(ctx context.Context, sessionID string, data *SessionData, ttl time.Duration) error

	// Get retrieves session data by ID. Returns nil if not found or expired.
	Get(ctx context.Context, sessionID string) (*SessionData, error)

	// Delete removes a session by ID.
	Delete(ctx context.Context, sessionID string) error

	// Close releases any resources used by the backend.
	Close() error
}

// MemorySessionBackend implements SessionBackend using in-memory storage.
type MemorySessionBackend struct {
	mu       sync.Mutex
	sessions map[string]*SessionData
	ttls     map[string]time.Time
}

// NewMemorySessionBackend creates a new in-memory session backend.
func NewMemorySessionBackend() *MemorySessionBackend {
	return &MemorySessionBackend{
		sessions: make(map[string]*SessionData),
		ttls:     make(map[string]time.Time),
	}
}

// Create stores session data in memory.
func (b *MemorySessionBackend) Create(ctx context.Context, sessionID string, data *SessionData, ttl time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	storedData := *data
	b.sessions[sessionID] = &storedData
	b.ttls[sessionID] = time.Now().Add(ttl)
	return nil
}

// Get retrieves session data from memory.
func (b *MemorySessionBackend) Get(ctx context.Context, sessionID string) (*SessionData, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	data, exists := b.sessions[sessionID]
	if !exists {
		return nil, nil
	}

	// Check expiration
	expiresAt, hasExpiry := b.ttls[sessionID]
	if hasExpiry && time.Now().After(expiresAt) {
		delete(b.sessions, sessionID)
		delete(b.ttls, sessionID)
		return nil, nil
	}

	return data, nil
}

// Delete removes a session from memory.
func (b *MemorySessionBackend) Delete(ctx context.Context, sessionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.sessions, sessionID)
	delete(b.ttls, sessionID)
	return nil
}

// Close is a no-op for in-memory backend.
func (b *MemorySessionBackend) Close() error {
	return nil
}
