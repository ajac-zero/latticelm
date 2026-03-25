package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemorySessionBackend(t *testing.T) {
	ctx := context.Background()
	backend := NewMemorySessionBackend()
	defer backend.Close()

	data := &SessionData{
		UserID:  "user-123",
		Email:   "test@example.com",
		Name:    "Test User",
		IsAdmin: true,
	}

	// Test Create
	err := backend.Create(ctx, "session-1", data, time.Hour)
	require.NoError(t, err)

	// Test Get
	retrieved, err := backend.Get(ctx, "session-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "user-123", retrieved.UserID)
	assert.Equal(t, "test@example.com", retrieved.Email)
	assert.True(t, retrieved.IsAdmin)

	// Test Get non-existent
	retrieved, err = backend.Get(ctx, "non-existent")
	require.NoError(t, err)
	assert.Nil(t, retrieved)

	// Test Delete
	err = backend.Delete(ctx, "session-1")
	require.NoError(t, err)

	retrieved, err = backend.Get(ctx, "session-1")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestMemorySessionBackend_Expiration(t *testing.T) {
	ctx := context.Background()
	backend := NewMemorySessionBackend()
	defer backend.Close()

	data := &SessionData{
		UserID: "user-123",
	}

	// Create with short TTL
	err := backend.Create(ctx, "session-1", data, 10*time.Millisecond)
	require.NoError(t, err)

	// Should be retrievable immediately
	retrieved, err := backend.Get(ctx, "session-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Should be expired
	retrieved, err = backend.Get(ctx, "session-1")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestRedisSessionBackend(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	ctx := context.Background()
	backend := NewRedisSessionBackend(client)
	defer backend.Close()

	data := &SessionData{
		UserID:       "user-789",
		Email:        "redis@example.com",
		Name:         "Redis User",
		IsAdmin:      false,
		OwnerIss:     "https://issuer.example.com",
		OwnerSub:     "oidc-sub-123",
		TenantID:     "tenant-456",
		IDToken:      "id-token-value",
		AccessToken:  "access-token-value",
		RefreshToken: "refresh-token-value",
	}

	// Test Create
	err := backend.Create(ctx, "session-redis-1", data, time.Hour)
	require.NoError(t, err)

	// Test Get
	retrieved, err := backend.Get(ctx, "session-redis-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "user-789", retrieved.UserID)
	assert.Equal(t, "redis@example.com", retrieved.Email)
	assert.Equal(t, "Redis User", retrieved.Name)
	assert.False(t, retrieved.IsAdmin)
	assert.Equal(t, "https://issuer.example.com", retrieved.OwnerIss)
	assert.Equal(t, "oidc-sub-123", retrieved.OwnerSub)
	assert.Equal(t, "tenant-456", retrieved.TenantID)
	assert.Equal(t, "id-token-value", retrieved.IDToken)

	// Test Get non-existent
	retrieved, err = backend.Get(ctx, "non-existent")
	require.NoError(t, err)
	assert.Nil(t, retrieved)

	// Test Delete
	err = backend.Delete(ctx, "session-redis-1")
	require.NoError(t, err)

	retrieved, err = backend.Get(ctx, "session-redis-1")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestRedisSessionBackend_Expiration(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	ctx := context.Background()
	backend := NewRedisSessionBackend(client)
	defer backend.Close()

	data := &SessionData{
		UserID: "user-123",
	}

	// Create with short TTL
	err := backend.Create(ctx, "session-expire", data, 100*time.Millisecond)
	require.NoError(t, err)

	// Should be retrievable immediately
	retrieved, err := backend.Get(ctx, "session-expire")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Fast-forward miniredis time
	mr.FastForward(200 * time.Millisecond)

	// Should be expired
	retrieved, err = backend.Get(ctx, "session-expire")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestSessionStore_WithMemoryBackend(t *testing.T) {
	backend := NewMemorySessionBackend()
	store := NewSessionStore(time.Hour, backend)
	defer store.Close()

	data := &SessionData{
		UserID: "user-123",
		Email:  "test@example.com",
	}

	// Test Create
	sessionID, err := store.Create(data)
	require.NoError(t, err)
	assert.NotEmpty(t, sessionID)

	// Test Get
	retrieved, exists := store.Get(sessionID)
	assert.True(t, exists)
	require.NotNil(t, retrieved)
	assert.Equal(t, "user-123", retrieved.UserID)
	assert.Empty(t, retrieved.AccessToken)
	assert.Empty(t, retrieved.RefreshToken)

	// Test Delete
	store.Delete(sessionID)

	retrieved, exists = store.Get(sessionID)
	assert.False(t, exists)
	assert.Nil(t, retrieved)
}

func TestSessionStore_WithRedisBackend(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	backend := NewRedisSessionBackend(client)
	store := NewSessionStore(time.Hour, backend)
	defer store.Close()

	data := &SessionData{
		UserID: "user-redis",
		Email:  "redis@example.com",
	}

	sessionID, err := store.Create(data)
	require.NoError(t, err)
	assert.NotEmpty(t, sessionID)

	retrieved, exists := store.Get(sessionID)
	assert.True(t, exists)
	require.NotNil(t, retrieved)
	assert.Equal(t, "user-redis", retrieved.UserID)

	store.Delete(sessionID)

	retrieved, exists = store.Get(sessionID)
	assert.False(t, exists)
	assert.Nil(t, retrieved)
}

func TestSessionStore_WithNilBackend(t *testing.T) {
	// Nil backend uses in-memory map (backward compatible)
	store := NewSessionStore(time.Hour)

	data := &SessionData{
		UserID: "user-456",
		Email:  "legacy@example.com",
	}

	sessionID, err := store.Create(data)
	require.NoError(t, err)

	retrieved, exists := store.Get(sessionID)
	assert.True(t, exists)
	require.NotNil(t, retrieved)
	assert.Equal(t, "user-456", retrieved.UserID)

	store.Delete(sessionID)

	retrieved, exists = store.Get(sessionID)
	assert.False(t, exists)
}

func TestSessionStore_DeleteWithError(t *testing.T) {
	store := NewSessionStore(time.Hour, failingSessionBackend{deleteErr: errors.New("delete failed")})

	err := store.DeleteWithError("session-1")
	require.EqualError(t, err, "delete failed")
}

func TestMemorySessionBackend_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	backend := NewMemorySessionBackend()
	defer backend.Close()

	var wg sync.WaitGroup
	errCh := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sessionID := fmt.Sprintf("session-%d", i)
			data := &SessionData{UserID: sessionID}
			if err := backend.Create(ctx, sessionID, data, time.Hour); err != nil {
				errCh <- err
				return
			}
			retrieved, err := backend.Get(ctx, sessionID)
			if err != nil {
				errCh <- err
				return
			}
			if retrieved == nil {
				errCh <- errors.New("expected session data")
				return
			}
			if err := backend.Delete(ctx, sessionID); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestSessionStore_Close(t *testing.T) {
	// Test with backend that has Close
	backend := NewMemorySessionBackend()
	store := NewSessionStore(time.Hour, backend)

	err := store.Close()
	assert.NoError(t, err)

	// Test with nil backend (no-op Close)
	store = NewSessionStore(time.Hour)
	err = store.Close()
	assert.NoError(t, err)
}

type failingSessionBackend struct {
	deleteErr error
}

func (f failingSessionBackend) Create(ctx context.Context, sessionID string, data *SessionData, ttl time.Duration) error {
	return nil
}

func (f failingSessionBackend) Get(ctx context.Context, sessionID string) (*SessionData, error) {
	return nil, nil
}

func (f failingSessionBackend) Delete(ctx context.Context, sessionID string) error {
	return f.deleteErr
}

func (f failingSessionBackend) Close() error {
	return nil
}
