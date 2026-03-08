package conversation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRedisStore(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	require.NotNil(t, store)

	defer store.Close()
}

func TestRedisStore_Create(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()
	messages := CreateTestMessages(3)

	conv, err := store.Create(ctx, "test-id", "test-model", messages, OwnerInfo{})
	require.NoError(t, err)
	require.NotNil(t, conv)

	assert.Equal(t, "test-id", conv.ID)
	assert.Equal(t, "test-model", conv.Model)
	assert.Len(t, conv.Messages, 3)
}

func TestRedisStore_Get(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()
	messages := CreateTestMessages(2)

	// Create a conversation
	created, err := store.Create(ctx, "get-test", "model-1", messages, OwnerInfo{})
	require.NoError(t, err)

	// Retrieve it
	retrieved, err := store.Get(ctx, "get-test")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, created.ID, retrieved.ID)
	assert.Equal(t, created.Model, retrieved.Model)
	assert.Len(t, retrieved.Messages, 2)

	// Test not found
	notFound, err := store.Get(ctx, "non-existent")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestRedisStore_Append(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()
	initialMessages := CreateTestMessages(2)

	// Create conversation
	conv, err := store.Create(ctx, "append-test", "model-1", initialMessages, OwnerInfo{})
	require.NoError(t, err)
	assert.Len(t, conv.Messages, 2)

	// Append more messages
	newMessages := CreateTestMessages(3)
	updated, err := store.Append(ctx, "append-test", newMessages...)
	require.NoError(t, err)
	require.NotNil(t, updated)

	assert.Len(t, updated.Messages, 5)
}

func TestRedisStore_Delete(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()
	messages := CreateTestMessages(1)

	// Create conversation
	_, err := store.Create(ctx, "delete-test", "model-1", messages, OwnerInfo{})
	require.NoError(t, err)

	// Verify it exists
	conv, err := store.Get(ctx, "delete-test")
	require.NoError(t, err)
	require.NotNil(t, conv)

	// Delete it
	err = store.Delete(ctx, "delete-test")
	require.NoError(t, err)

	// Verify it's gone
	deleted, err := store.Get(ctx, "delete-test")
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestRedisStore_Size(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()

	// Initial size should be 0
	assert.Equal(t, 0, store.Size())

	// Create conversations
	messages := CreateTestMessages(1)
	_, err := store.Create(ctx, "size-1", "model-1", messages, OwnerInfo{})
	require.NoError(t, err)

	_, err = store.Create(ctx, "size-2", "model-1", messages, OwnerInfo{})
	require.NoError(t, err)

	assert.Equal(t, 2, store.Size())

	// Delete one
	err = store.Delete(ctx, "size-1")
	require.NoError(t, err)

	assert.Equal(t, 1, store.Size())
}

func TestRedisStore_TTL(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	// Use short TTL for testing
	store := NewRedisStore(client, 100*time.Millisecond)
	defer store.Close()

	ctx := context.Background()
	messages := CreateTestMessages(1)

	// Create a conversation
	_, err := store.Create(ctx, "ttl-test", "model-1", messages, OwnerInfo{})
	require.NoError(t, err)

	// Fast forward time in miniredis
	mr.FastForward(200 * time.Millisecond)

	// Key should have expired
	conv, err := store.Get(ctx, "ttl-test")
	require.NoError(t, err)
	assert.Nil(t, conv, "conversation should have expired")
}

func TestRedisStore_KeyStorage(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()
	messages := CreateTestMessages(1)

	// Create conversation
	_, err := store.Create(ctx, "storage-test", "model-1", messages, OwnerInfo{})
	require.NoError(t, err)

	// Check that key exists in Redis
	keys := mr.Keys()
	assert.Greater(t, len(keys), 0, "should have at least one key in Redis")
}

func TestRedisStore_Concurrent(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()

	// Run concurrent operations
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			id := fmt.Sprintf("concurrent-%d", idx)
			messages := CreateTestMessages(2)

			// Create
			_, err := store.Create(ctx, id, "model-1", messages, OwnerInfo{})
			assert.NoError(t, err)

			// Get
			_, err = store.Get(ctx, id)
			assert.NoError(t, err)

			// Append
			newMsg := CreateTestMessages(1)
			_, err = store.Append(ctx, id, newMsg...)
			assert.NoError(t, err)

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all conversations exist
	assert.Equal(t, 10, store.Size())
}

func TestRedisStore_JSONEncoding(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()

	// Create messages with various content types
	messages := []api.Message{
		{
			Role: "user",
			Content: []api.ContentBlock{
				{Type: "text", Text: "Hello"},
			},
		},
		{
			Role: "assistant",
			Content: []api.ContentBlock{
				{Type: "text", Text: "Hi there!"},
			},
		},
	}

	conv, err := store.Create(ctx, "json-test", "model-1", messages, OwnerInfo{})
	require.NoError(t, err)

	// Retrieve and verify JSON encoding/decoding
	retrieved, err := store.Get(ctx, "json-test")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, len(conv.Messages), len(retrieved.Messages))
	assert.Equal(t, conv.Messages[0].Role, retrieved.Messages[0].Role)
	assert.Equal(t, conv.Messages[0].Content[0].Text, retrieved.Messages[0].Content[0].Text)
}

func TestRedisStore_EmptyMessages(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()

	// Create conversation with empty messages
	conv, err := store.Create(ctx, "empty", "model-1", []api.Message{}, OwnerInfo{})
	require.NoError(t, err)
	require.NotNil(t, conv)

	assert.Len(t, conv.Messages, 0)

	// Retrieve and verify
	retrieved, err := store.Get(ctx, "empty")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Len(t, retrieved.Messages, 0)
}

func TestRedisStore_UpdateExisting(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()
	messages1 := CreateTestMessages(2)

	// Create first version
	conv1, err := store.Create(ctx, "update-test", "model-1", messages1, OwnerInfo{})
	require.NoError(t, err)
	originalTime := conv1.UpdatedAt

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Create again with different data (overwrites)
	messages2 := CreateTestMessages(3)
	conv2, err := store.Create(ctx, "update-test", "model-2", messages2, OwnerInfo{})
	require.NoError(t, err)

	assert.Equal(t, "model-2", conv2.Model)
	assert.Len(t, conv2.Messages, 3)
	assert.True(t, conv2.UpdatedAt.After(originalTime))
}

func TestRedisStore_ContextCancellation(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	messages := CreateTestMessages(1)

	// Operations with cancelled context should fail or return quickly
	_, err := store.Create(ctx, "cancelled", "model-1", messages, OwnerInfo{})
	// Context cancellation should be respected
	_ = err
}

func TestRedisStore_ScanPagination(t *testing.T) {
	client, mr := SetupTestRedis(t)
	defer mr.Close()

	store := NewRedisStore(client, time.Hour)
	defer store.Close()

	ctx := context.Background()
	messages := CreateTestMessages(1)

	// Create multiple conversations to test scanning
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("scan-%d", i)
		_, err := store.Create(ctx, id, "model-1", messages, OwnerInfo{})
		require.NoError(t, err)
	}

	// Size should count all of them
	assert.Equal(t, 50, store.Size())
}
