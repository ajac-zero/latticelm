package apikeys

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKey(t *testing.T) {
	plaintext, hash, err := GenerateKey()
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(plaintext, "sk-"), "key should start with sk-")
	assert.Len(t, plaintext, 67, "sk- (3) + 64 hex chars")
	assert.Len(t, hash, 64, "SHA-256 hex digest is 64 chars")
	assert.Equal(t, hash, HashKey(plaintext), "hash must match")
}

func TestHashKey_Deterministic(t *testing.T) {
	key := "sk-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	h1 := HashKey(key)
	h2 := HashKey(key)
	assert.Equal(t, h1, h2)
}

func TestKeyDisplayPrefix(t *testing.T) {
	assert.Equal(t, "sk-a1b2c3d4...", KeyDisplayPrefix("sk-a1b2c3d4e5f6"))
	assert.Equal(t, "short", KeyDisplayPrefix("short"))
}

func TestAPIKey_IsExpired(t *testing.T) {
	k := &APIKey{}
	assert.False(t, k.IsExpired(), "nil expires_at is not expired")
}
