package conversation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/redis/go-redis/v9"
)

// RedisStore manages conversation history in Redis with automatic expiration.
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
	ctx    context.Context
}

// NewRedisStore creates a Redis-backed conversation store.
func NewRedisStore(client *redis.Client, ttl time.Duration) *RedisStore {
	return &RedisStore{
		client: client,
		ttl:    ttl,
		ctx:    context.Background(),
	}
}

// key returns the Redis key for a conversation ID.
func (s *RedisStore) key(id string) string {
	return "conv:" + id
}

// Get retrieves a conversation by ID from Redis.
func (s *RedisStore) Get(id string) (*Conversation, error) {
	data, err := s.client.Get(s.ctx, s.key(id)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var conv Conversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, err
	}

	return &conv, nil
}

// Create creates a new conversation with the given messages.
func (s *RedisStore) Create(id string, model string, messages []api.Message) (*Conversation, error) {
	now := time.Now()
	conv := &Conversation{
		ID:        id,
		Messages:  messages,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(conv)
	if err != nil {
		return nil, err
	}

	if err := s.client.Set(s.ctx, s.key(id), data, s.ttl).Err(); err != nil {
		return nil, err
	}

	return conv, nil
}

// Append adds new messages to an existing conversation.
func (s *RedisStore) Append(id string, messages ...api.Message) (*Conversation, error) {
	conv, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, nil
	}

	conv.Messages = append(conv.Messages, messages...)
	conv.UpdatedAt = time.Now()

	data, err := json.Marshal(conv)
	if err != nil {
		return nil, err
	}

	if err := s.client.Set(s.ctx, s.key(id), data, s.ttl).Err(); err != nil {
		return nil, err
	}

	return conv, nil
}

// Delete removes a conversation from Redis.
func (s *RedisStore) Delete(id string) error {
	return s.client.Del(s.ctx, s.key(id)).Err()
}

// Size returns the number of active conversations in Redis.
func (s *RedisStore) Size() int {
	var count int
	var cursor uint64

	for {
		keys, nextCursor, err := s.client.Scan(s.ctx, cursor, "conv:*", 100).Result()
		if err != nil {
			return 0
		}

		count += len(keys)
		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	return count
}
