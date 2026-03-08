package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Backend defines the interface for distributed rate limiting storage.
type Backend interface {
	AllowRequest(ctx context.Context, key string, rate float64, burst int) (bool, error)
	AcquireConcurrent(ctx context.Context, key string, max int) (bool, error)
	ReleaseConcurrent(ctx context.Context, key string) error
	CheckQuota(ctx context.Context, key string, limit int64) (remaining int64, allowed bool, err error)
	RecordUsage(ctx context.Context, key string, tokens int64) error
	Close() error
}

// RedisBackend implements Backend using Redis with Lua scripts for atomicity.
type RedisBackend struct {
	client *redis.Client
}

// NewRedisBackend creates a new Redis-backed rate limiting backend.
func NewRedisBackend(client *redis.Client) *RedisBackend {
	return &RedisBackend{client: client}
}

// tokenBucketScript implements a distributed token bucket via Lua.
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = 1

local data = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(data[1])
local last_refill = tonumber(data[2])

if tokens == nil then
	tokens = capacity
	last_refill = now
end

local elapsed = (now - last_refill) / 1000000
local new_tokens = math.min(capacity, tokens + elapsed * rate)

if new_tokens >= requested then
	new_tokens = new_tokens - requested
	redis.call('HMSET', key, 'tokens', new_tokens, 'last_refill', now)
	redis.call('PEXPIRE', key, math.ceil(capacity / rate * 1000) + 5000)
	return 1
else
	redis.call('HMSET', key, 'tokens', new_tokens, 'last_refill', now)
	redis.call('PEXPIRE', key, math.ceil(capacity / rate * 1000) + 5000)
	return 0
end
`)

// AllowRequest checks if a request is allowed under the token bucket rate limit.
func (b *RedisBackend) AllowRequest(ctx context.Context, key string, rate float64, burst int) (bool, error) {
	nowMicro := time.Now().UnixMicro()
	result, err := tokenBucketScript.Run(ctx, b.client, []string{key}, rate, burst, nowMicro).Int64()
	if err != nil {
		return false, fmt.Errorf("token bucket script: %w", err)
	}
	return result == 1, nil
}

// acquireConcurrentScript atomically checks and increments a concurrent counter.
var acquireConcurrentScript = redis.NewScript(`
local key = KEYS[1]
local max = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])

local current = tonumber(redis.call('GET', key) or '0')
if current >= max then
	return 0
end
redis.call('INCR', key)
redis.call('EXPIRE', key, ttl)
return 1
`)

// AcquireConcurrent tries to acquire a concurrent request slot.
func (b *RedisBackend) AcquireConcurrent(ctx context.Context, key string, max int) (bool, error) {
	ttl := 300 // 5 minute safety TTL
	result, err := acquireConcurrentScript.Run(ctx, b.client, []string{key}, max, ttl).Int64()
	if err != nil {
		return false, fmt.Errorf("acquire concurrent script: %w", err)
	}
	return result == 1, nil
}

// ReleaseConcurrent decrements the concurrent request counter.
func (b *RedisBackend) ReleaseConcurrent(ctx context.Context, key string) error {
	pipe := b.client.Pipeline()
	pipe.Decr(ctx, key)
	// Ensure counter doesn't go below 0 due to race conditions
	pipe.Do(ctx, "WATCH", key)
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		// Fallback: just decrement
		val, decrErr := b.client.Decr(ctx, key).Result()
		if decrErr != nil {
			return fmt.Errorf("release concurrent: %w", decrErr)
		}
		// Correct if it went negative
		if val < 0 {
			b.client.Set(ctx, key, 0, 0)
		}
		return nil
	}
	return nil
}

// CheckQuota checks if the daily token quota has been exceeded.
func (b *RedisBackend) CheckQuota(ctx context.Context, key string, limit int64) (int64, bool, error) {
	used, err := b.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return limit, true, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("check quota: %w", err)
	}
	remaining := limit - used
	if remaining <= 0 {
		return 0, false, nil
	}
	return remaining, true, nil
}

// RecordUsage records token usage against the daily quota.
func (b *RedisBackend) RecordUsage(ctx context.Context, key string, tokens int64) error {
	pipe := b.client.Pipeline()
	pipe.IncrBy(ctx, key, tokens)
	// Set expiry to end of current UTC day + 1 hour buffer
	now := time.Now().UTC()
	endOfDay := time.Date(now.Year(), now.Month(), now.Day()+1, 1, 0, 0, 0, time.UTC)
	pipe.ExpireAt(ctx, key, endOfDay)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("record usage: %w", err)
	}
	return nil
}

// Close closes the Redis connection.
func (b *RedisBackend) Close() error {
	return b.client.Close()
}
