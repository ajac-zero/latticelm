package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/conversation"
)

// ---------- shouldStore tests ----------

func TestShouldStore(t *testing.T) {
	tests := []struct {
		name           string
		storeByDefault bool
		reqStore       *bool
		want           bool
	}{
		{
			name:           "default no-store, client omits field",
			storeByDefault: false,
			reqStore:       nil,
			want:           false,
		},
		{
			name:           "default no-store, client sets true",
			storeByDefault: false,
			reqStore:       boolPtr(true),
			want:           true,
		},
		{
			name:           "default no-store, client sets false",
			storeByDefault: false,
			reqStore:       boolPtr(false),
			want:           false,
		},
		{
			name:           "default store, client omits field",
			storeByDefault: true,
			reqStore:       nil,
			want:           true,
		},
		{
			name:           "default store, client sets false",
			storeByDefault: true,
			reqStore:       boolPtr(false),
			want:           false,
		},
		{
			name:           "default store, client sets true",
			storeByDefault: true,
			reqStore:       boolPtr(true),
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(newMockRegistry(), newMockConversationStore(), newMockLogger().asLogger())
			s.SetStoreByDefault(tt.storeByDefault)
			req := &api.ResponseRequest{Store: tt.reqStore}
			assert.Equal(t, tt.want, s.shouldStore(req))
		})
	}
}

// ---------- store=false creates no persisted record (sync) ----------

func TestHandleResponses_Sync_StoreFalse_NoPersistence(t *testing.T) {
	registry := newMockRegistry()
	provider := newMockProvider("openai")
	provider.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
		return &api.ProviderResult{
			Model: "gpt-4",
			Text:  "Hello!",
			Usage: api.Usage{InputTokens: 5, OutputTokens: 5, TotalTokens: 10},
		}, nil
	}
	registry.addProvider("openai", provider)
	registry.addModel("gpt-4", "openai")

	store := newMockConversationStore()
	srv := New(registry, store, newMockLogger().asLogger())

	body := `{"model": "gpt-4", "input": "hello", "store": false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleResponses(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, store.Size(), "no conversation should be stored when store=false")

	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Store, "response should echo store=false")
}

// ---------- default no-store with no explicit store field ----------

func TestHandleResponses_Sync_DefaultNoStore(t *testing.T) {
	registry := newMockRegistry()
	provider := newMockProvider("openai")
	registry.addProvider("openai", provider)
	registry.addModel("gpt-4", "openai")

	store := newMockConversationStore()
	srv := New(registry, store, newMockLogger().asLogger())
	// storeByDefault is false by default

	body := `{"model": "gpt-4", "input": "hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleResponses(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, store.Size(), "default no-store should not persist")
}

// ---------- explicit store=true persists ----------

func TestHandleResponses_Sync_StoreTrue_Persists(t *testing.T) {
	registry := newMockRegistry()
	provider := newMockProvider("openai")
	registry.addProvider("openai", provider)
	registry.addModel("gpt-4", "openai")

	store := newMockConversationStore()
	srv := New(registry, store, newMockLogger().asLogger())

	body := `{"model": "gpt-4", "input": "hello", "store": true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleResponses(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, store.Size(), "store=true should persist")

	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Store, "response should echo store=true")
}

// ---------- store_by_default=true stores when client omits ----------

func TestHandleResponses_Sync_StoreByDefaultTrue(t *testing.T) {
	registry := newMockRegistry()
	provider := newMockProvider("openai")
	registry.addProvider("openai", provider)
	registry.addModel("gpt-4", "openai")

	store := newMockConversationStore()
	srv := New(registry, store, newMockLogger().asLogger())
	srv.SetStoreByDefault(true)

	body := `{"model": "gpt-4", "input": "hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleResponses(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, store.Size(), "store_by_default=true should persist when client omits store")
}

// ---------- streaming: store=false creates no record ----------

func TestHandleResponses_Stream_StoreFalse_NoPersistence(t *testing.T) {
	registry := newMockRegistry()
	provider := newMockProvider("openai")
	provider.streamFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
		deltaChan := make(chan *api.ProviderStreamDelta)
		errChan := make(chan error, 1)
		go func() {
			defer close(deltaChan)
			defer close(errChan)
			deltaChan <- &api.ProviderStreamDelta{Model: "gpt-4", Text: "Hello"}
			deltaChan <- &api.ProviderStreamDelta{Done: true}
		}()
		return deltaChan, errChan
	}
	registry.addProvider("openai", provider)
	registry.addModel("gpt-4", "openai")

	store := newMockConversationStore()
	srv := New(registry, store, newMockLogger().asLogger())

	body := `{"model": "gpt-4", "input": "hello", "stream": true, "store": false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := newFlushableRecorder()

	srv.handleResponses(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, store.Size(), "streaming with store=false should not persist")
}

// ---------- DELETE /v1/responses/{id} ----------

func TestHandleResponseByID_Delete_StorePolicy(t *testing.T) {
	store := newMockConversationStore()
	store.setConversation("resp_abc", &conversation.Conversation{
		ID:    "resp_abc",
		Model: "gpt-4",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}},
		},
	})

	srv := New(newMockRegistry(), store, newMockLogger().asLogger())

	req := httptest.NewRequest(http.MethodDelete, "/v1/responses/resp_abc", nil)
	rec := httptest.NewRecorder()

	srv.handleResponseByID(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, 0, store.Size(), "conversation should be deleted")
}

func TestHandleResponseByID_NotFound_StorePolicy(t *testing.T) {
	store := newMockConversationStore()
	srv := New(newMockRegistry(), store, newMockLogger().asLogger())

	req := httptest.NewRequest(http.MethodDelete, "/v1/responses/nonexistent", nil)
	rec := httptest.NewRecorder()

	srv.handleResponseByID(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "conversation not found")
}

func TestHandleResponseByID_EmptyID_StorePolicy(t *testing.T) {
	srv := New(newMockRegistry(), newMockConversationStore(), newMockLogger().asLogger())

	req := httptest.NewRequest(http.MethodDelete, "/v1/responses/", nil)
	rec := httptest.NewRecorder()

	srv.handleResponseByID(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "response id is required")
}
