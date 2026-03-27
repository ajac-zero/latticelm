package apikeys

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

const keyPrefix = "sk-"

// Status represents the state of an API key.
type Status string

const (
	StatusActive  Status = "active"
	StatusRevoked Status = "revoked"
)

// APIKey represents a stored API key record.
type APIKey struct {
	ID         string
	Name       string
	KeyHash    string // SHA-256 hex digest of the full key
	KeyPrefix  string // first 8 chars for display (e.g., "sk-a1b2...")
	UserID     string // FK to users.id
	Status     Status
	ExpiresAt  *time.Time
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// IsExpired returns true when the key has a set expiry in the past.
func (k *APIKey) IsExpired() bool {
	return k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt)
}

// GenerateKey produces a cryptographically random API key and returns both
// the plaintext (to show the user once) and its SHA-256 hash for storage.
func GenerateKey() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	plaintext = keyPrefix + hex.EncodeToString(b)
	hash = HashKey(plaintext)
	return plaintext, hash, nil
}

// HashKey returns the hex-encoded SHA-256 digest of a plaintext key.
func HashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// KeyDisplayPrefix returns a short prefix safe to show in listings.
func KeyDisplayPrefix(plaintext string) string {
	if len(plaintext) > 11 {
		return plaintext[:11] + "..."
	}
	return plaintext
}
