package conversation

import (
	"context"
	"testing"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNopStore_Get(t *testing.T) {
	store := NewNopStore()
	conv, err := store.Get(context.Background(), "any-id")
	require.NoError(t, err)
	assert.Nil(t, conv)
}

func TestNopStore_Create(t *testing.T) {
	store := NewNopStore()
	msgs := []api.Message{{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}}}
	conv, err := store.Create(context.Background(), "id", "model", msgs, OwnerInfo{}, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, conv)
}

func TestNopStore_Append(t *testing.T) {
	store := NewNopStore()
	msg := api.Message{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}}
	conv, err := store.Append(context.Background(), "id", msg)
	require.NoError(t, err)
	assert.Nil(t, conv)
}

func TestNopStore_Delete(t *testing.T) {
	store := NewNopStore()
	err := store.Delete(context.Background(), "id")
	require.NoError(t, err)
}

func TestNopStore_Size(t *testing.T) {
	store := NewNopStore()
	assert.Equal(t, 0, store.Size())
}

func TestNopStore_Close(t *testing.T) {
	store := NewNopStore()
	err := store.Close()
	require.NoError(t, err)
}
