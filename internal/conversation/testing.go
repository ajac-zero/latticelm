package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
)

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

func (m *MockStore) Create(ctx context.Context, conversationID string, model string, messages []api.Message, owner OwnerInfo, request *api.ResponseRequest) (*Conversation, error) {
	m.createCalled = true
	m.conversations[conversationID] = &Conversation{
		ID:        conversationID,
		Model:     model,
		Messages:  messages,
		Request:   copyRequest(request),
		OwnerIss:  owner.OwnerIss,
		OwnerSub:  owner.OwnerSub,
		TenantID:  owner.TenantID,
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

func (m *MockStore) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	// Set defaults
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.Limit < 1 {
		opts.Limit = 20
	}

	// Filter conversations
	var filtered []*Conversation
	for _, conv := range m.conversations {
		// Apply filters
		if opts.OwnerIss != "" && conv.OwnerIss != opts.OwnerIss {
			continue
		}
		if opts.OwnerSub != "" && conv.OwnerSub != opts.OwnerSub {
			continue
		}
		if opts.TenantID != "" && conv.TenantID != opts.TenantID {
			continue
		}
		if opts.Model != "" && conv.Model != opts.Model {
			continue
		}
		if opts.Search != "" && !strings.Contains(conv.ID, opts.Search) {
			continue
		}
		filtered = append(filtered, conv)
	}

	// Paginate
	total := len(filtered)
	start := (opts.Page - 1) * opts.Limit
	end := start + opts.Limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	return &ListResult{
		Conversations: filtered[start:end],
		Total:         total,
	}, nil
}

func (m *MockStore) Close() error {
	return nil
}
