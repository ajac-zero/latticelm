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
