package conversation

import (
	"context"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
)

// OwnerInfo holds the identity metadata stamped onto every conversation.
type OwnerInfo struct {
	OwnerIss string
	OwnerSub string
	TenantID string
}

// ListOptions contains filters and pagination for listing conversations.
type ListOptions struct {
	Page     int    // Page number (1-indexed)
	Limit    int    // Items per page
	OwnerIss string // Filter by owner issuer
	OwnerSub string // Filter by owner subject
	TenantID string // Filter by tenant
	Model    string // Filter by model
	Search   string // Search by ID (partial match)
}

// ListResult contains paginated conversation list results.
type ListResult struct {
	Conversations []*Conversation
	Total         int
}

// Store defines the interface for conversation storage backends.
type Store interface {
	Get(ctx context.Context, id string) (*Conversation, error)
	Create(ctx context.Context, id string, model string, messages []api.Message, owner OwnerInfo) (*Conversation, error)
	Append(ctx context.Context, id string, messages ...api.Message) (*Conversation, error)
	Delete(ctx context.Context, id string) error
	Size() int
	Close() error
	List(ctx context.Context, opts ListOptions) (*ListResult, error)
}

// Conversation holds the message history for a single conversation thread.
type Conversation struct {
	ID        string
	Messages  []api.Message
	Model     string
	OwnerIss  string
	OwnerSub  string
	TenantID  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// copyMessages returns a deep copy of a Message slice, duplicating nested Content and ToolCalls slices.
func copyMessages(messages []api.Message) []api.Message {
	out := make([]api.Message, len(messages))
	for i, msg := range messages {
		out[i] = msg
		if msg.Content != nil {
			out[i].Content = make([]api.ContentBlock, len(msg.Content))
			copy(out[i].Content, msg.Content)
		}
		if msg.ToolCalls != nil {
			out[i].ToolCalls = make([]api.ToolCall, len(msg.ToolCalls))
			copy(out[i].ToolCalls, msg.ToolCalls)
		}
	}
	return out
}

// copyConversation returns a deep copy of a Conversation.
func copyConversation(conv *Conversation) *Conversation {
	return &Conversation{
		ID:        conv.ID,
		Messages:  copyMessages(conv.Messages),
		Model:     conv.Model,
		OwnerIss:  conv.OwnerIss,
		OwnerSub:  conv.OwnerSub,
		TenantID:  conv.TenantID,
		CreatedAt: conv.CreatedAt,
		UpdatedAt: conv.UpdatedAt,
	}
}

