package conversation

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ajac-zero/latticelm/internal/authctx"
)

// API provides HTTP handlers for conversation management.
type API struct {
	store Store
}

// NewAPI creates a new conversation API handler.
func NewAPI(store Store) *API {
	return &API{store: store}
}

// RegisterRoutes registers conversation API routes.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/conversations", a.HandleListConversations)
	mux.HandleFunc("/api/conversations/", a.HandleConversationByID)
}

// ConversationResponse is the API response for a single conversation.
type ConversationResponse struct {
	ID           string `json:"id"`
	Model        string `json:"model"`
	MessageCount int    `json:"message_count"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// ConversationDetailResponse includes full message history.
type ConversationDetailResponse struct {
	ID        string                   `json:"id"`
	Model     string                   `json:"model"`
	Messages  []MessageResponse        `json:"messages"`
	CreatedAt string                   `json:"created_at"`
	UpdatedAt string                   `json:"updated_at"`
}

// MessageResponse is a simplified message representation.
type MessageResponse struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at,omitempty"`
}

// ListConversationsResponse is the API response for listing conversations.
type ListConversationsResponse struct {
	Conversations []*ConversationResponse `json:"conversations"`
	Total         int                    `json:"total"`
	Page          int                    `json:"page"`
	Limit         int                    `json:"limit"`
}

// HandleListConversations lists conversations with pagination and filtering.
// Non-admin users can only see their own conversations.
// Admins can see all conversations and apply additional filters.
func (a *API) HandleListConversations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	model := r.URL.Query().Get("model")
	search := r.URL.Query().Get("search")

	opts := ListOptions{
		Page:   page,
		Limit:  limit,
		Model:  model,
		Search: search,
	}

	// Apply ownership filter for non-admin users
	isAdmin := a.isAdmin(r)
	if !isAdmin {
		ownerIss, ownerSub := a.getOwner(r)
		if ownerIss == "" || ownerSub == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"error": "Not authenticated",
			})
			return
		}
		opts.OwnerIss = ownerIss
		opts.OwnerSub = ownerSub
	}

	// Admin can filter by tenant
	if isAdmin {
		opts.TenantID = r.URL.Query().Get("tenant_id")
	}

	result, err := a.store.List(r.Context(), opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to list conversations",
		})
		return
	}

	// Convert to response format
	conversations := make([]*ConversationResponse, len(result.Conversations))
	for i, conv := range result.Conversations {
		conversations[i] = &ConversationResponse{
			ID:           conv.ID,
			Model:        conv.Model,
			MessageCount: len(conv.Messages),
			CreatedAt:    conv.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    conv.UpdatedAt.Format(time.RFC3339),
		}
	}

	writeJSON(w, http.StatusOK, ListConversationsResponse{
		Conversations: conversations,
		Total:         result.Total,
		Page:          opts.Page,
		Limit:         opts.Limit,
	})
}

// HandleConversationByID handles GET and DELETE for a specific conversation.
func (a *API) HandleConversationByID(w http.ResponseWriter, r *http.Request) {
	// Extract conversation ID from path: /api/conversations/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/conversations/")
	convID := strings.TrimSpace(path)

	if convID == "" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.HandleGetConversation(w, r, convID)
	case http.MethodDelete:
		a.HandleDeleteConversation(w, r, convID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleGetConversation retrieves a single conversation by ID.
// Users can only access their own conversations. Admins can access any conversation.
func (a *API) HandleGetConversation(w http.ResponseWriter, r *http.Request, convID string) {
	conv, err := a.store.Get(r.Context(), convID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to retrieve conversation",
		})
		return
	}
	if conv == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": "Conversation not found",
		})
		return
	}

	// Check ownership
	if !a.canAccess(w, r, conv) {
		return
	}

	// Convert messages to response format
	messages := make([]MessageResponse, len(conv.Messages))
	for i, msg := range conv.Messages {
		content := ""
		if len(msg.Content) > 0 {
			content = msg.Content[0].Text
		}
		messages[i] = MessageResponse{
			Role:    msg.Role,
			Content: content,
		}
	}

	writeJSON(w, http.StatusOK, ConversationDetailResponse{
		ID:        conv.ID,
		Model:     conv.Model,
		Messages:  messages,
		CreatedAt: conv.CreatedAt.Format(time.RFC3339),
		UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
	})
}

// HandleDeleteConversation deletes a conversation by ID.
// Users can only delete their own conversations. Admins can delete any conversation.
func (a *API) HandleDeleteConversation(w http.ResponseWriter, r *http.Request, convID string) {
	conv, err := a.store.Get(r.Context(), convID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to retrieve conversation",
		})
		return
	}
	if conv == nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": "Conversation not found",
		})
		return
	}

	// Check ownership
	if !a.canAccess(w, r, conv) {
		return
	}

	if err := a.store.Delete(r.Context(), convID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to delete conversation",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Conversation deleted successfully",
		"id":      convID,
	})
}

// isAdmin checks if the current user is an admin.
func (a *API) isAdmin(r *http.Request) bool {
	isAdmin := r.Context().Value(authctx.IsAdminKey)
	if isAdmin != nil {
		if admin, ok := isAdmin.(bool); ok && admin {
			return true
		}
	}
	return false
}

// getOwner extracts the owner identity from the request context.
// The OIDC session middleware sets OwnerIssKey and OwnerSubKey before this is called.
func (a *API) getOwner(r *http.Request) (ownerIss, ownerSub string) {
	if v := r.Context().Value(authctx.OwnerIssKey); v != nil {
		ownerIss, _ = v.(string)
	}
	if v := r.Context().Value(authctx.OwnerSubKey); v != nil {
		ownerSub, _ = v.(string)
	}
	return
}

// canAccess checks if the current user can access the conversation.
// Returns false and writes an error response if access is denied.
func (a *API) canAccess(w http.ResponseWriter, r *http.Request, conv *Conversation) bool {
	// Admins can access any conversation
	if a.isAdmin(r) {
		return true
	}

	// Get owner identity from context
	// The session middleware should set these values
	ownerIssVal := r.Context().Value(authctx.OwnerIssKey)
	ownerSubVal := r.Context().Value(authctx.OwnerSubKey)

	if ownerIssVal == nil || ownerSubVal == nil {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"error": "Access denied",
		})
		return false
	}

	ownerIss, ok1 := ownerIssVal.(string)
	ownerSub, ok2 := ownerSubVal.(string)

	if !ok1 || !ok2 {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Invalid owner context",
		})
		return false
	}

	// Check ownership
	if conv.OwnerIss != ownerIss || conv.OwnerSub != ownerSub {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": "Conversation not found",
		})
		return false
	}

	return true
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
