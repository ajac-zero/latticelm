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
	Create(ctx context.Context, id string, model string, messages []api.Message, owner OwnerInfo, request *api.ResponseRequest, replayState *api.ReplayState) (*Conversation, error)
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
	Replay    *api.ReplayState
	Model     string
	Request   *api.ResponseRequest
	OwnerIss  string
	OwnerSub  string
	TenantID  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func copyRequest(req *api.ResponseRequest) *api.ResponseRequest {
	if req == nil {
		return nil
	}
	out := *req
	out.Tools = append([]byte(nil), req.Tools...)
	out.ToolChoice = append([]byte(nil), req.ToolChoice...)
	out.Text = append([]byte(nil), req.Text...)
	out.Reasoning = append([]byte(nil), req.Reasoning...)
	out.StreamOptions = append([]byte(nil), req.StreamOptions...)
	out.Include = append([]string(nil), req.Include...)
	if req.Metadata != nil {
		out.Metadata = make(map[string]string, len(req.Metadata))
		for k, v := range req.Metadata {
			out.Metadata[k] = v
		}
	}
	if req.Input.String != nil {
		v := *req.Input.String
		out.Input.String = &v
	}
	if req.Input.Items != nil {
		out.Input.Items = append([]api.InputItem(nil), req.Input.Items...)
	}
	return &out
}
