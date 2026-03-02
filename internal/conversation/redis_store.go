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
func (s *RedisStore) Get(id string) (*Conversation, bool) {
	data, err := s.client.Get(s.ctx, s.key(id)).Bytes()
	if err != nil {
		return nil, false
	}

	var conv Conversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, false
	}

	return &conv, true
}

// Create creates a new conversation with the given messages.
func (s *RedisStore) Create(id string, model string, messages []api.Message) *Conversation {
	now := time.Now()
	conv := &Conversation{
		ID:        id,
		Messages:  messages,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, _ := json.Marshal(conv)
	_ = s.client.Set(s.ctx, s.key(id), data, s.ttl).Err()

	return conv
}

// Append adds new messages to an existing conversation.
func (s *RedisStore) Append(id string, messages ...api.Message) (*Conversation, bool) {
	conv, ok := s.Get(id)
	if !ok {
		return nil, false
	}

	conv.Messages = append(conv.Messages, messages...)
	conv.UpdatedAt = time.Now()

	data, _ := json.Marshal(conv)
	_ = s.client.Set(s.ctx, s.key(id), data, s.ttl).Err()

	return conv, true
}

// Delete removes a conversation from Redis.
func (s *RedisStore) Delete(id string) {
	_ = s.client.Del(s.ctx, s.key(id)).Err()
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
