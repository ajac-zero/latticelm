package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

// OIDCSessionCookieName is the browser cookie that stores the OIDC-backed
// server-side session ID.
const OIDCSessionCookieName = "session"

// SessionData holds information about an authenticated user session.
type SessionData struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	IDToken   string    `json:"id_token"`
	ExpiresAt time.Time `json:"expires_at"`
	IsAdmin   bool      `json:"is_admin"`
	OwnerIss  string    `json:"owner_iss"` // OIDC issuer for conversation ownership
	OwnerSub  string    `json:"owner_sub"` // OIDC subject for conversation ownership
	TenantID  string    `json:"tenant_id"` // Tenant ID for multi-tenancy
}

// SessionStore manages user sessions with a pluggable backend.
type SessionStore struct {
	backend SessionBackend
	ttl     time.Duration
	// For backward compatibility: in-memory map when no backend provided
	sessions map[string]*SessionData
	mu       sync.RWMutex
}

// NewSessionStore creates a new session store with the given TTL and optional backend.
// If no backend is provided, uses in-memory storage (single instance only).
func NewSessionStore(ttl time.Duration, backends ...SessionBackend) *SessionStore {
	if ttl == 0 {
		ttl = 24 * time.Hour // default 24 hours
	}
	var backend SessionBackend
	if len(backends) > 0 {
		backend = backends[0]
	}
	store := &SessionStore{
		backend: backend,
		ttl:     ttl,
	}
	if backend == nil {
		store.sessions = make(map[string]*SessionData)
		go store.cleanup()
	}
	return store
}

// Create generates a new session ID and stores the session data.
func (s *SessionStore) Create(data *SessionData) (string, error) {
	if data == nil {
		return "", errors.New("session data is nil")
	}

	sessionID, err := generateSessionID()
	if err != nil {
		return "", err
	}

	data.ExpiresAt = time.Now().Add(s.ttl)
	storedData := *data

	// Use backend if configured
	if s.backend != nil {
		ctx := context.Background()
		if err := s.backend.Create(ctx, sessionID, &storedData, s.ttl); err != nil {
			return "", err
		}
		return sessionID, nil
	}

	// Fallback to in-memory storage
	s.mu.Lock()
	s.sessions[sessionID] = &storedData
	s.mu.Unlock()

	return sessionID, nil
}

// Get retrieves session data by session ID.
func (s *SessionStore) Get(sessionID string) (*SessionData, bool) {
	// Use backend if configured
	if s.backend != nil {
		ctx := context.Background()
		data, err := s.backend.Get(ctx, sessionID)
		if err != nil || data == nil {
			return nil, false
		}
		// Check expiration (backend may handle this, but verify)
		if time.Now().After(data.ExpiresAt) {
			return nil, false
		}
		return data, true
	}

	// Fallback to in-memory storage
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, exists := s.sessions[sessionID]
	if !exists {
		return nil, false
	}

	// Check if session is expired
	if time.Now().After(data.ExpiresAt) {
		return nil, false
	}

	return data, true
}

// TTL returns the configured session lifetime.
func (s *SessionStore) TTL() time.Duration {
	return s.ttl
}

// Delete removes a session by ID.
func (s *SessionStore) Delete(sessionID string) {
	_ = s.DeleteWithError(sessionID)
}

// DeleteWithError removes a session by ID and returns any backend error.
func (s *SessionStore) DeleteWithError(sessionID string) error {
	// Use backend if configured
	if s.backend != nil {
		ctx := context.Background()
		return s.backend.Delete(ctx, sessionID)
	}

	// Fallback to in-memory storage
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	return nil
}

// Close releases resources used by the backend.
func (s *SessionStore) Close() error {
	if s.backend != nil {
		return s.backend.Close()
	}
	return nil
}

// cleanup periodically removes expired sessions (in-memory only).
func (s *SessionStore) cleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for id, data := range s.sessions {
			if now.After(data.ExpiresAt) {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

// generateSessionID creates a cryptographically secure random session ID.
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
