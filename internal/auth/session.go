package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// OIDCSessionCookieName is the browser cookie that stores the OIDC-backed
// server-side session ID.
const OIDCSessionCookieName = "session"

// SessionData holds information about an authenticated user session.
type SessionData struct {
	UserID       string
	Email        string
	Name         string
	IDToken      string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	IsAdmin      bool
	OwnerIss     string // OIDC issuer for conversation ownership
	OwnerSub     string // OIDC subject for conversation ownership
	TenantID     string // Tenant ID for multi-tenancy
}

// SessionStore manages user sessions.
type SessionStore struct {
	sessions map[string]*SessionData
	mu       sync.RWMutex
	ttl      time.Duration
}

// NewSessionStore creates a new session store with the given TTL.
func NewSessionStore(ttl time.Duration) *SessionStore {
	if ttl == 0 {
		ttl = 24 * time.Hour // default 24 hours
	}
	store := &SessionStore{
		sessions: make(map[string]*SessionData),
		ttl:      ttl,
	}
	go store.cleanup()
	return store
}

// Create generates a new session ID and stores the session data.
func (s *SessionStore) Create(data *SessionData) (string, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return "", err
	}

	data.ExpiresAt = time.Now().Add(s.ttl)

	s.mu.Lock()
	s.sessions[sessionID] = data
	s.mu.Unlock()

	return sessionID, nil
}

// Get retrieves session data by session ID.
func (s *SessionStore) Get(sessionID string) (*SessionData, bool) {
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

// Delete removes a session by ID.
func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

// cleanup periodically removes expired sessions.
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
