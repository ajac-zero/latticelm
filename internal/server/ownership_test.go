package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/conversation"
)

func setupTestServer(adminCfg auth.AdminConfig) (*GatewayServer, *mockConversationStore, *mockProvider) {
	registry := newMockRegistry()
	provider := newMockProvider("openai")
	registry.addProvider("openai", provider)
	registry.addModel("gpt-4", "openai")

	store := newMockConversationStore()
	server := New(registry, store, newMockLogger().asLogger(), WithAdminConfig(adminCfg))
	server.SetStoreByDefault(true)
	return server, store, provider
}

func makeRequest(t *testing.T, server *GatewayServer, body string, principal *auth.Principal) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if principal != nil {
		req = req.WithContext(auth.ContextWithPrincipal(req.Context(), principal))
	}

	rec := httptest.NewRecorder()
	server.handleResponses(rec, req)
	return rec
}

func TestOwnership_SameUserCanContinueOwnConversation(t *testing.T) {
	server, store, _ := setupTestServer(auth.AdminConfig{Enabled: true})

	store.setConversation("prev-1", &conversation.Conversation{
		ID:       "prev-1",
		Model:    "gpt-4",
		OwnerIss: "https://auth.example.com",
		OwnerSub: "user-1",
		TenantID: "tenant-a",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}},
		},
	})

	principal := &auth.Principal{
		Issuer:   "https://auth.example.com",
		Subject:  "user-1",
		TenantID: "tenant-a",
	}

	rec := makeRequest(t, server,
		`{"model":"gpt-4","input":"follow-up","previous_response_id":"prev-1"}`,
		principal,
	)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp api.Response
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "completed", resp.Status)
}

func TestOwnership_CrossUserAccessDenied(t *testing.T) {
	server, store, _ := setupTestServer(auth.AdminConfig{Enabled: true})

	store.setConversation("prev-owned", &conversation.Conversation{
		ID:       "prev-owned",
		Model:    "gpt-4",
		OwnerIss: "https://auth.example.com",
		OwnerSub: "user-1",
		TenantID: "tenant-a",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}},
		},
	})

	// User B trying to access User A's conversation
	principal := &auth.Principal{
		Issuer:   "https://auth.example.com",
		Subject:  "user-2",
		TenantID: "tenant-a",
	}

	rec := makeRequest(t, server,
		`{"model":"gpt-4","input":"steal context","previous_response_id":"prev-owned"}`,
		principal,
	)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "conversation not found")
}

func TestOwnership_CrossTenantAccessDenied(t *testing.T) {
	server, store, _ := setupTestServer(auth.AdminConfig{Enabled: true})

	store.setConversation("prev-tenant", &conversation.Conversation{
		ID:       "prev-tenant",
		Model:    "gpt-4",
		OwnerIss: "https://auth.example.com",
		OwnerSub: "user-1",
		TenantID: "tenant-a",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}},
		},
	})

	// Same user ID but different tenant
	principal := &auth.Principal{
		Issuer:   "https://auth.example.com",
		Subject:  "user-1",
		TenantID: "tenant-b",
	}

	rec := makeRequest(t, server,
		`{"model":"gpt-4","input":"cross-tenant","previous_response_id":"prev-tenant"}`,
		principal,
	)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "conversation not found")
}

func TestOwnership_AdminOverrideAllowed(t *testing.T) {
	server, store, _ := setupTestServer(auth.AdminConfig{
		Enabled:       true,
		AllowedValues: []string{"admin"},
	})

	store.setConversation("prev-admin", &conversation.Conversation{
		ID:       "prev-admin",
		Model:    "gpt-4",
		OwnerIss: "https://auth.example.com",
		OwnerSub: "user-1",
		TenantID: "tenant-a",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}},
		},
	})

	// Admin from a different user/tenant
	principal := &auth.Principal{
		Issuer:   "https://auth.example.com",
		Subject:  "admin-user",
		TenantID: "tenant-admin",
		Roles:    []string{"admin"},
	}

	rec := makeRequest(t, server,
		`{"model":"gpt-4","input":"admin access","previous_response_id":"prev-admin"}`,
		principal,
	)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOwnership_NonAdminCannotOverride(t *testing.T) {
	server, store, _ := setupTestServer(auth.AdminConfig{
		Enabled:       true,
		AllowedValues: []string{"admin"},
	})

	store.setConversation("prev-nonadmin", &conversation.Conversation{
		ID:       "prev-nonadmin",
		Model:    "gpt-4",
		OwnerIss: "https://auth.example.com",
		OwnerSub: "user-1",
		TenantID: "tenant-a",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}},
		},
	})

	principal := &auth.Principal{
		Issuer:   "https://auth.example.com",
		Subject:  "user-3",
		TenantID: "tenant-a",
		Roles:    []string{"user"},
	}

	rec := makeRequest(t, server,
		`{"model":"gpt-4","input":"non-admin attempt","previous_response_id":"prev-nonadmin"}`,
		principal,
	)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "conversation not found")
}

func TestOwnership_NoPrincipalSkipsCheck(t *testing.T) {
	server, store, _ := setupTestServer(auth.AdminConfig{Enabled: false})

	store.setConversation("prev-noauth", &conversation.Conversation{
		ID:    "prev-noauth",
		Model: "gpt-4",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}},
		},
	})

	// No principal (auth disabled)
	rec := makeRequest(t, server,
		`{"model":"gpt-4","input":"no auth","previous_response_id":"prev-noauth"}`,
		nil,
	)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOwnership_NewConversationStampsOwner(t *testing.T) {
	server, store, _ := setupTestServer(auth.AdminConfig{Enabled: true})

	principal := &auth.Principal{
		Issuer:   "https://auth.example.com",
		Subject:  "user-1",
		TenantID: "tenant-a",
	}

	rec := makeRequest(t, server,
		`{"model":"gpt-4","input":"first message"}`,
		principal,
	)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Check that the created conversation has ownership fields
	store.mu.Lock()
	defer store.mu.Unlock()

	require.Equal(t, 1, len(store.conversations))

	for _, conv := range store.conversations {
		assert.Equal(t, "https://auth.example.com", conv.OwnerIss)
		assert.Equal(t, "user-1", conv.OwnerSub)
		assert.Equal(t, "tenant-a", conv.TenantID)
	}
}

func TestOwnership_DifferentIssuerDenied(t *testing.T) {
	server, store, _ := setupTestServer(auth.AdminConfig{Enabled: true})

	store.setConversation("prev-issuer", &conversation.Conversation{
		ID:       "prev-issuer",
		Model:    "gpt-4",
		OwnerIss: "https://auth.example.com",
		OwnerSub: "user-1",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}},
		},
	})

	// Same subject but different issuer
	principal := &auth.Principal{
		Issuer:  "https://other-auth.example.com",
		Subject: "user-1",
	}

	rec := makeRequest(t, server,
		`{"model":"gpt-4","input":"diff issuer","previous_response_id":"prev-issuer"}`,
		principal,
	)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestOwnership_StreamingEnforcesOwnership(t *testing.T) {
	server, store, _ := setupTestServer(auth.AdminConfig{Enabled: true})

	store.setConversation("prev-stream", &conversation.Conversation{
		ID:       "prev-stream",
		Model:    "gpt-4",
		OwnerIss: "https://auth.example.com",
		OwnerSub: "user-1",
		TenantID: "tenant-a",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hi"}}},
		},
	})

	// Different user trying to access via streaming
	principal := &auth.Principal{
		Issuer:   "https://auth.example.com",
		Subject:  "user-2",
		TenantID: "tenant-a",
	}

	rec := makeRequest(t, server,
		`{"model":"gpt-4","input":"steal","stream":true,"previous_response_id":"prev-stream"}`,
		principal,
	)

	// Ownership check happens before streaming starts, so we get a 404
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
