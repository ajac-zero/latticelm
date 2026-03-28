package conversation

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ajac-zero/latticelm/internal/api"
)

func setupPostgresDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	pgCtr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	testcontainers.CleanupContainer(t, pgCtr)
	require.NoError(t, err)

	connStr, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)

	t.Cleanup(func() { db.Close() })

	require.NoError(t, db.PingContext(ctx))

	return db
}

func TestNewSQLStore(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	require.NotNil(t, store)

	defer store.Close()

	// Verify table was created
	var tableName sql.NullString
	err = db.QueryRow(`SELECT to_regclass('public.conversations')`).Scan(&tableName)
	require.NoError(t, err)
	assert.True(t, tableName.Valid)
}

func TestSQLStore_Create(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	messages := CreateTestMessages(3)

	conv, err := store.Create(ctx, "test-id", "test-model", messages, OwnerInfo{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, conv)

	assert.Equal(t, "test-id", conv.ID)
	assert.Equal(t, "test-model", conv.Model)
	assert.Len(t, conv.Messages, 3)
}

func TestSQLStore_Get(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	messages := CreateTestMessages(2)

	// Create a conversation
	created, err := store.Create(ctx, "get-test", "model-1", messages, OwnerInfo{}, nil, nil)
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

func TestSQLStore_Append(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	initialMessages := CreateTestMessages(2)

	// Create conversation
	conv, err := store.Create(ctx, "append-test", "model-1", initialMessages, OwnerInfo{}, nil, nil)
	require.NoError(t, err)
	assert.Len(t, conv.Messages, 2)

	// Append more messages
	newMessages := CreateTestMessages(3)
	updated, err := store.Append(ctx, "append-test", newMessages...)
	require.NoError(t, err)
	require.NotNil(t, updated)

	assert.Len(t, updated.Messages, 5)
}

func TestSQLStore_Delete(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	messages := CreateTestMessages(1)

	// Create conversation
	_, err = store.Create(ctx, "delete-test", "model-1", messages, OwnerInfo{}, nil, nil)
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

func TestSQLStore_Size(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Initial size should be 0
	assert.Equal(t, 0, store.Size())

	// Create conversations
	messages := CreateTestMessages(1)
	_, err = store.Create(ctx, "size-1", "model-1", messages, OwnerInfo{}, nil, nil)
	require.NoError(t, err)

	_, err = store.Create(ctx, "size-2", "model-1", messages, OwnerInfo{}, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, 2, store.Size())

	// Delete one
	err = store.Delete(ctx, "size-1")
	require.NoError(t, err)

	assert.Equal(t, 1, store.Size())
}

func TestSQLStore_Cleanup(t *testing.T) {
	db := setupPostgresDB(t)

	// Use very short TTL for testing
	store, err := NewSQLStore(db, "pgx", 100*time.Millisecond)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	messages := CreateTestMessages(1)

	// Create a conversation
	_, err = store.Create(ctx, "cleanup-test", "model-1", messages, OwnerInfo{}, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, store.Size())

	// Wait for TTL to expire and cleanup to run
	time.Sleep(500 * time.Millisecond)

	// Conversation should be cleaned up
	assert.Equal(t, 0, store.Size())
}

func TestSQLStore_ConcurrentAccess(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Run concurrent operations
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			id := fmt.Sprintf("concurrent-%d", idx)
			messages := CreateTestMessages(2)

			// Create
			_, err := store.Create(ctx, id, "model-1", messages, OwnerInfo{}, nil, nil)
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

func TestSQLStore_ContextCancellation(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	messages := CreateTestMessages(1)

	// Operations with cancelled context should fail quickly.
	_, err = store.Create(ctx, "cancelled", "model-1", messages, OwnerInfo{}, nil, nil)
	assert.Error(t, err)
}

func TestSQLStore_JSONEncoding(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
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

	conv, err := store.Create(ctx, "json-test", "model-1", messages, OwnerInfo{}, nil, nil)
	require.NoError(t, err)

	// Retrieve and verify JSON encoding/decoding
	retrieved, err := store.Get(ctx, "json-test")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, len(conv.Messages), len(retrieved.Messages))
	assert.Equal(t, conv.Messages[0].Role, retrieved.Messages[0].Role)
	assert.Equal(t, conv.Messages[0].Content[0].Text, retrieved.Messages[0].Content[0].Text)
}

func TestSQLStore_EmptyMessages(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create conversation with empty messages
	conv, err := store.Create(ctx, "empty", "model-1", []api.Message{}, OwnerInfo{}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, conv)

	assert.Len(t, conv.Messages, 0)

	// Retrieve and verify
	retrieved, err := store.Get(ctx, "empty")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Len(t, retrieved.Messages, 0)
}

func TestSQLStore_ReplayStateRoundTrip(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	messages := []api.Message{{
		Role:    "assistant",
		Content: []api.ContentBlock{{Type: "output_text", Text: "portable"}},
	}}
	replay := &api.ReplayState{
		Provider:           "anthropic",
		ProviderResponseID: "anth_resp_123",
		Items: []api.ReplayItem{{
			ID:             "msg_123",
			OutputItemType: "message",
			MessageIndex:   0,
			Message: &api.Message{
				Role: "assistant",
				Content: []api.ContentBlock{
					{Type: "anthropic_thinking", Text: "chain", Signature: "sig_123"},
					{Type: "output_text", Text: "portable"},
				},
			},
		}},
	}

	_, err = store.Create(ctx, "replay-test", "claude-test", messages, OwnerInfo{}, nil, replay)
	require.NoError(t, err)

	retrieved, err := store.Get(ctx, "replay-test")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.NotNil(t, retrieved.Replay)
	require.Len(t, retrieved.Replay.Items, 1)
	assert.Equal(t, replay.Provider, retrieved.Replay.Provider)
	assert.Equal(t, replay.ProviderResponseID, retrieved.Replay.ProviderResponseID)
	assert.Equal(t, "msg_123", retrieved.Replay.Items[0].ID)
	require.NotNil(t, retrieved.Replay.Items[0].Message)
	assert.Equal(t, "anthropic_thinking", retrieved.Replay.Items[0].Message.Content[0].Type)
	assert.Equal(t, "sig_123", retrieved.Replay.Items[0].Message.Content[0].Signature)
}

func TestSQLStore_UpdateExisting(t *testing.T) {
	db := setupPostgresDB(t)

	store, err := NewSQLStore(db, "pgx", time.Hour)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	messages1 := CreateTestMessages(2)

	// Create first version
	conv1, err := store.Create(ctx, "update-test", "model-1", messages1, OwnerInfo{}, nil, nil)
	require.NoError(t, err)
	originalTime := conv1.UpdatedAt

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Create again with different data (upsert)
	messages2 := CreateTestMessages(3)
	conv2, err := store.Create(ctx, "update-test", "model-2", messages2, OwnerInfo{}, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "model-2", conv2.Model)
	assert.Len(t, conv2.Messages, 3)
	assert.True(t, conv2.UpdatedAt.After(originalTime))
}
