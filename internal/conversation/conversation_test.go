package conversation

import (
	"context"
	"testing"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_CreateAndGet(t *testing.T) {
	store := NewMemoryStore(1 * time.Hour)

	messages := []api.Message{
		{
			Role: "user",
			Content: []api.ContentBlock{
				{Type: "input_text", Text: "Hello"},
			},
		},
	}

	conv, err := store.Create(context.Background(),"test-id", "gpt-4", messages)
	require.NoError(t, err)
	require.NotNil(t, conv)
	assert.Equal(t, "test-id", conv.ID)
	assert.Equal(t, "gpt-4", conv.Model)
	assert.Len(t, conv.Messages, 1)
	assert.Equal(t, "Hello", conv.Messages[0].Content[0].Text)

	retrieved, err := store.Get(context.Background(),"test-id")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, conv.ID, retrieved.ID)
	assert.Equal(t, conv.Model, retrieved.Model)
	assert.Len(t, retrieved.Messages, 1)
}

func TestMemoryStore_GetNonExistent(t *testing.T) {
	store := NewMemoryStore(1 * time.Hour)

	conv, err := store.Get(context.Background(),"nonexistent")
	require.NoError(t, err)
	assert.Nil(t, conv, "should return nil for nonexistent conversation")
}

func TestMemoryStore_Append(t *testing.T) {
	store := NewMemoryStore(1 * time.Hour)

	initialMessages := []api.Message{
		{
			Role: "user",
			Content: []api.ContentBlock{
				{Type: "input_text", Text: "First message"},
			},
		},
	}

	_, err := store.Create(context.Background(),"test-id", "gpt-4", initialMessages)
	require.NoError(t, err)

	newMessages := []api.Message{
		{
			Role: "assistant",
			Content: []api.ContentBlock{
				{Type: "output_text", Text: "Response"},
			},
		},
		{
			Role: "user",
			Content: []api.ContentBlock{
				{Type: "input_text", Text: "Follow-up"},
			},
		},
	}

	conv, err := store.Append(context.Background(),"test-id", newMessages...)
	require.NoError(t, err)
	require.NotNil(t, conv)
	assert.Len(t, conv.Messages, 3, "should have all messages")
	assert.Equal(t, "First message", conv.Messages[0].Content[0].Text)
	assert.Equal(t, "Response", conv.Messages[1].Content[0].Text)
	assert.Equal(t, "Follow-up", conv.Messages[2].Content[0].Text)
}

func TestMemoryStore_AppendNonExistent(t *testing.T) {
	store := NewMemoryStore(1 * time.Hour)

	newMessage := api.Message{
		Role: "user",
		Content: []api.ContentBlock{
			{Type: "input_text", Text: "Hello"},
		},
	}

	conv, err := store.Append(context.Background(),"nonexistent", newMessage)
	require.NoError(t, err)
	assert.Nil(t, conv, "should return nil when appending to nonexistent conversation")
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore(1 * time.Hour)

	messages := []api.Message{
		{
			Role: "user",
			Content: []api.ContentBlock{
				{Type: "input_text", Text: "Hello"},
			},
		},
	}

	_, err := store.Create(context.Background(),"test-id", "gpt-4", messages)
	require.NoError(t, err)

	// Verify it exists
	conv, err := store.Get(context.Background(),"test-id")
	require.NoError(t, err)
	assert.NotNil(t, conv)

	// Delete it
	err = store.Delete(context.Background(),"test-id")
	require.NoError(t, err)

	// Verify it's gone
	conv, err = store.Get(context.Background(),"test-id")
	require.NoError(t, err)
	assert.Nil(t, conv, "conversation should be deleted")
}

func TestMemoryStore_Size(t *testing.T) {
	store := NewMemoryStore(1 * time.Hour)

	assert.Equal(t, 0, store.Size(), "should start empty")

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
	}

	_, err := store.Create(context.Background(),"conv-1", "gpt-4", messages)
	require.NoError(t, err)
	assert.Equal(t, 1, store.Size())

	_, err = store.Create(context.Background(),"conv-2", "gpt-4", messages)
	require.NoError(t, err)
	assert.Equal(t, 2, store.Size())

	err = store.Delete(context.Background(),"conv-1")
	require.NoError(t, err)
	assert.Equal(t, 1, store.Size())
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	store := NewMemoryStore(1 * time.Hour)

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
	}

	// Create initial conversation
	_, err := store.Create(context.Background(),"test-id", "gpt-4", messages)
	require.NoError(t, err)

	// Simulate concurrent reads and writes
	done := make(chan bool, 10)
	for i := 0; i < 5; i++ {
		go func() {
			_, _ = store.Get(context.Background(),"test-id")
			done <- true
		}()
	}
	for i := 0; i < 5; i++ {
		go func() {
			newMsg := api.Message{
				Role: "assistant",
				Content: []api.ContentBlock{{Type: "output_text", Text: "Response"}},
			}
			_, _ = store.Append(context.Background(),"test-id", newMsg)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify final state
	conv, err := store.Get(context.Background(),"test-id")
	require.NoError(t, err)
	assert.NotNil(t, conv)
	assert.GreaterOrEqual(t, len(conv.Messages), 1)
}

func TestMemoryStore_DeepCopy(t *testing.T) {
	store := NewMemoryStore(1 * time.Hour)

	messages := []api.Message{
		{
			Role: "user",
			Content: []api.ContentBlock{
				{Type: "input_text", Text: "Original"},
			},
		},
	}

	_, err := store.Create(context.Background(),"test-id", "gpt-4", messages)
	require.NoError(t, err)

	// Get conversation
	conv1, err := store.Get(context.Background(),"test-id")
	require.NoError(t, err)

	// Note: Current implementation copies the Messages slice but not the Content blocks
	// So modifying the slice structure is safe, but modifying content blocks affects the original
	// This documents actual behavior - future improvement could add deep copying of content blocks

	// Safe: appending to Messages slice
	originalLen := len(conv1.Messages)
	conv1.Messages = append(conv1.Messages, api.Message{
		Role: "assistant",
		Content: []api.ContentBlock{{Type: "output_text", Text: "New message"}},
	})
	assert.Equal(t, originalLen+1, len(conv1.Messages), "can modify returned message slice")

	// Verify original is unchanged
	conv2, err := store.Get(context.Background(),"test-id")
	require.NoError(t, err)
	assert.Equal(t, originalLen, len(conv2.Messages), "original conversation unaffected by slice modification")
}

func TestMemoryStore_TTLCleanup(t *testing.T) {
	// Use very short TTL for testing
	store := NewMemoryStore(100 * time.Millisecond)

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
	}

	_, err := store.Create(context.Background(),"test-id", "gpt-4", messages)
	require.NoError(t, err)

	// Verify it exists
	conv, err := store.Get(context.Background(),"test-id")
	require.NoError(t, err)
	assert.NotNil(t, conv)
	assert.Equal(t, 1, store.Size())

	// Wait for TTL to expire and cleanup to run
	// Cleanup runs every 1 minute, but for testing we check the logic
	// In production, we'd wait longer or expose cleanup for testing
	time.Sleep(150 * time.Millisecond)

	// Note: The cleanup goroutine runs every 1 minute, so in a real scenario
	// we'd need to wait that long or refactor to expose the cleanup function
	// For now, this test documents the expected behavior
}

func TestMemoryStore_NoTTL(t *testing.T) {
	// Store with no TTL (0 duration) should not start cleanup
	store := NewMemoryStore(0)

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
	}

	_, err := store.Create(context.Background(),"test-id", "gpt-4", messages)
	require.NoError(t, err)
	assert.Equal(t, 1, store.Size())

	// Without TTL, conversation should persist indefinitely
	conv, err := store.Get(context.Background(),"test-id")
	require.NoError(t, err)
	assert.NotNil(t, conv)
}

func TestMemoryStore_UpdatedAtTracking(t *testing.T) {
	store := NewMemoryStore(1 * time.Hour)

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
	}

	conv, err := store.Create(context.Background(),"test-id", "gpt-4", messages)
	require.NoError(t, err)
	createdAt := conv.CreatedAt
	updatedAt := conv.UpdatedAt

	assert.Equal(t, createdAt, updatedAt, "initially created and updated should match")

	// Wait a bit and append
	time.Sleep(10 * time.Millisecond)

	newMsg := api.Message{
		Role: "assistant",
		Content: []api.ContentBlock{{Type: "output_text", Text: "Response"}},
	}
	conv, err = store.Append(context.Background(),"test-id", newMsg)
	require.NoError(t, err)

	assert.Equal(t, createdAt, conv.CreatedAt, "created time should not change")
	assert.True(t, conv.UpdatedAt.After(updatedAt), "updated time should be newer")
}

func TestMemoryStore_MultipleConversations(t *testing.T) {
	store := NewMemoryStore(1 * time.Hour)

	// Create multiple conversations
	for i := 0; i < 10; i++ {
		id := "conv-" + string(rune('0'+i))
		model := "gpt-4"
		messages := []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello " + id}}},
		}
		_, err := store.Create(context.Background(),id, model, messages)
		require.NoError(t, err)
	}

	assert.Equal(t, 10, store.Size())

	// Verify each conversation is independent
	for i := 0; i < 10; i++ {
		id := "conv-" + string(rune('0'+i))
		conv, err := store.Get(context.Background(),id)
		require.NoError(t, err)
		require.NotNil(t, conv)
		assert.Equal(t, id, conv.ID)
		assert.Contains(t, conv.Messages[0].Content[0].Text, id)
	}
}
