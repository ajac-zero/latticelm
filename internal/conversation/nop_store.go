package conversation

import (
	"context"

	"github.com/ajac-zero/latticelm/internal/api"
)

// NopStore is a no-op conversation store that never persists data.
// Used when conversation storage is disabled.
type NopStore struct{}

// NewNopStore creates a no-op conversation store.
func NewNopStore() *NopStore {
	return &NopStore{}
}

func (s *NopStore) Get(ctx context.Context, id string) (*Conversation, error) {
	return nil, nil
}

func (s *NopStore) Create(ctx context.Context, id string, model string, messages []api.Message, owner OwnerInfo) (*Conversation, error) {
	return nil, nil
}

func (s *NopStore) Append(ctx context.Context, id string, messages ...api.Message) (*Conversation, error) {
	return nil, nil
}

func (s *NopStore) Delete(ctx context.Context, id string) error {
	return nil
}

func (s *NopStore) Size() int {
	return 0
}

func (s *NopStore) Close() error {
	return nil
}
