package conversation

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9"

	"github.com/ajac-zero/latticelm/internal/api"
)

// SetupTestDB creates an in-memory SQLite database for testing
func SetupTestDB(t *testing.T, driver string) *sql.DB {
	t.Helper()

	var dsn string
	switch driver {
	case "sqlite3":
		// Use in-memory SQLite database
		dsn = ":memory:"
	case "postgres":
		// For postgres tests, use a mock or skip
		t.Skip("PostgreSQL tests require external database")
		return nil
	case "mysql":
		// For mysql tests, use a mock or skip
		t.Skip("MySQL tests require external database")
		return nil
	default:
		t.Fatalf("unsupported driver: %s", driver)
		return nil
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Create the conversations table
	schema := `
		CREATE TABLE IF NOT EXISTS conversations (
			conversation_id TEXT PRIMARY KEY,
			messages TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

// SetupTestRedis creates a miniredis instance for testing
func SetupTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("failed to connect to miniredis: %v", err)
	}

	return client, mr
}

// CreateTestMessages generates test message fixtures
func CreateTestMessages(count int) []api.Message {
	messages := make([]api.Message, count)
	for i := 0; i < count; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = api.Message{
			Role: role,
			Content: []api.ContentBlock{
				{
					Type: "text",
					Text: fmt.Sprintf("Test message %d", i+1),
				},
			},
		}
	}
	return messages
}

// CreateTestConversation creates a test conversation with the given ID and messages
func CreateTestConversation(conversationID string, messageCount int) *Conversation {
	return &Conversation{
		ID:        conversationID,
		Messages:  CreateTestMessages(messageCount),
		Model:     "test-model",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// MockStore is a simple in-memory store for testing
type MockStore struct {
	conversations map[string]*Conversation
	getCalled     bool
	createCalled  bool
	appendCalled  bool
	deleteCalled  bool
	sizeCalled    bool
}

func NewMockStore() *MockStore {
	return &MockStore{
		conversations: make(map[string]*Conversation),
	}
}

func (m *MockStore) Get(ctx context.Context, conversationID string) (*Conversation, error) {
	m.getCalled = true
	conv, ok := m.conversations[conversationID]
	if !ok {
		return nil, fmt.Errorf("conversation not found")
	}
	return conv, nil
}

func (m *MockStore) Create(ctx context.Context, conversationID string, model string, messages []api.Message) (*Conversation, error) {
	m.createCalled = true
	m.conversations[conversationID] = &Conversation{
		ID:        conversationID,
		Model:     model,
		Messages:  messages,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return m.conversations[conversationID], nil
}

func (m *MockStore) Append(ctx context.Context, conversationID string, messages ...api.Message) (*Conversation, error) {
	m.appendCalled = true
	conv, ok := m.conversations[conversationID]
	if !ok {
		return nil, fmt.Errorf("conversation not found")
	}
	conv.Messages = append(conv.Messages, messages...)
	conv.UpdatedAt = time.Now()
	return conv, nil
}

func (m *MockStore) Delete(ctx context.Context, conversationID string) error {
	m.deleteCalled = true
	delete(m.conversations, conversationID)
	return nil
}

func (m *MockStore) Size() int {
	m.sizeCalled = true
	return len(m.conversations)
}

func (m *MockStore) Close() error {
	return nil
}
