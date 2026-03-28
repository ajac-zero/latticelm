package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSessionBackend implements SessionBackend using Redis.
type RedisSessionBackend struct {
	client *redis.Client
}

// NewRedisSessionBackend creates a new Redis-backed session backend.
func NewRedisSessionBackend(client *redis.Client) *RedisSessionBackend {
	return &RedisSessionBackend{client: client}
}

// Create stores session data in Redis with TTL.
func (b *RedisSessionBackend) Create(ctx context.Context, sessionID string, data *SessionData, ttl time.Duration) error {
	// Use Redis key prefix for namespacing
	key := b.sessionKey(sessionID)

	// Serialize session data
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal session data: %w", err)
	}

	// Store with TTL
	if err := b.client.Set(ctx, key, dataBytes, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}

	return nil
}

// Get retrieves session data from Redis.
func (b *RedisSessionBackend) Get(ctx context.Context, sessionID string) (*SessionData, error) {
	key := b.sessionKey(sessionID)

	dataBytes, err := b.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // Not found
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var data SessionData
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		return nil, fmt.Errorf("unmarshal session data: %w", err)
	}

	return &data, nil
}

// Delete removes a session from Redis.
func (b *RedisSessionBackend) Delete(ctx context.Context, sessionID string) error {
	key := b.sessionKey(sessionID)
	if err := b.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

// Close closes the Redis client.
func (b *RedisSessionBackend) Close() error {
	return b.client.Close()
}

func (b *RedisSessionBackend) sessionKey(sessionID string) string {
	return "session:" + sessionID
}
