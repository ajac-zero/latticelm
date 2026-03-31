package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/logger"
)

func (s *GatewayServer) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteOpenResponsesError(w, s.logger, "method not allowed", "invalid_request", http.StatusMethodNotAllowed, nil, nil)
		return
	}

	models := s.registry.Models()
	var data []api.ModelInfo
	for _, m := range models {
		data = append(data, api.ModelInfo{
			ID:       m.Model,
			Provider: m.Provider,
		})
	}

	resp := api.ModelsResponse{
		Object: "list",
		Data:   data,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to encode models response",
			logger.LogAttrsWithTrace(r.Context(),
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("error", err.Error()),
			)...,
		)
	}
}

func (s *GatewayServer) handleResponseByID(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path: supports both /v1/responses/{id} and /api/v1/responses/{id}
	const responsesSuffix = "/responses/"
	idx := strings.LastIndex(r.URL.Path, responsesSuffix)
	id := ""
	if idx >= 0 {
		id = r.URL.Path[idx+len(responsesSuffix):]
	}
	if id == "" {
		WriteOpenResponsesError(w, s.logger, "response id is required", "invalid_request", http.StatusBadRequest, nil, nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetResponse(w, r, id)
	case http.MethodDelete:
		s.handleDeleteResponse(w, r, id)
	default:
		WriteOpenResponsesError(w, s.logger, "method not allowed", "invalid_request", http.StatusMethodNotAllowed, nil, nil)
	}
}

func (s *GatewayServer) handleGetResponse(w http.ResponseWriter, r *http.Request, id string) {
	conv, err := s.convs.Get(r.Context(), id)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to look up conversation for retrieval",
			slog.String("request_id", logger.FromContext(r.Context())),
			slog.String("response_id", id),
			slog.String("error", err.Error()),
		)
		WriteOpenResponsesError(w, s.logger, "error looking up conversation", "server_error", http.StatusInternalServerError, nil, nil)
		return
	}
	if conv == nil {
		WriteOpenResponsesError(w, s.logger, "conversation not found", "not_found", http.StatusNotFound, nil, nil)
		return
	}

	principal := auth.PrincipalFromContext(r.Context())
	if !s.checkOwnership(w, r, conv, principal, id) {
		return
	}

	resp := s.buildResponseFromConversation(conv)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to encode response",
			slog.String("request_id", logger.FromContext(r.Context())),
			slog.String("response_id", id),
			slog.String("error", err.Error()),
		)
	}
}

func (s *GatewayServer) handleDeleteResponse(w http.ResponseWriter, r *http.Request, id string) {
	conv, err := s.convs.Get(r.Context(), id)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to look up conversation for deletion",
			slog.String("request_id", logger.FromContext(r.Context())),
			slog.String("response_id", id),
			slog.String("error", err.Error()),
		)
		WriteOpenResponsesError(w, s.logger, "error looking up conversation", "server_error", http.StatusInternalServerError, nil, nil)
		return
	}
	if conv == nil {
		WriteOpenResponsesError(w, s.logger, "conversation not found", "not_found", http.StatusNotFound, nil, nil)
		return
	}

	if err := s.convs.Delete(r.Context(), id); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to delete conversation",
			slog.String("request_id", logger.FromContext(r.Context())),
			slog.String("response_id", id),
			slog.String("error", err.Error()),
		)
		WriteOpenResponsesError(w, s.logger, "error deleting conversation", "server_error", http.StatusInternalServerError, nil, nil)
		return
	}

	s.logger.InfoContext(r.Context(), "conversation deleted",
		slog.String("request_id", logger.FromContext(r.Context())),
		slog.String("response_id", id),
	)

	w.WriteHeader(http.StatusNoContent)
}

// checkOwnership returns true if the principal is permitted to access conv.
// If access is denied, it writes a 404 to w and returns false.
func (s *GatewayServer) checkOwnership(w http.ResponseWriter, r *http.Request, conv *conversation.Conversation, principal *auth.Principal, conversationID string) bool {
	if principal == nil || principal.OwnsConversation(conv.OwnerIss, conv.OwnerSub, conv.TenantID) {
		return true
	}
	if principal.HasAdminRole(s.adminConfig) {
		s.logger.WarnContext(r.Context(), "admin override: accessing conversation owned by another user",
			logger.LogAttrsWithTrace(r.Context(),
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("conversation_id", conversationID),
				slog.String("admin_sub", principal.Subject),
				slog.String("admin_iss", principal.Issuer),
				slog.String("owner_sub", conv.OwnerSub),
				slog.String("owner_iss", conv.OwnerIss),
				slog.String("owner_tenant", conv.TenantID),
			)...,
		)
		return true
	}
	// Return 404 to avoid leaking conversation existence.
	s.logger.WarnContext(r.Context(), "conversation ownership check failed",
		slog.String("request_id", logger.FromContext(r.Context())),
		slog.String("conversation_id", conversationID),
		slog.String("caller_sub", principal.Subject),
		slog.String("caller_iss", principal.Issuer),
	)
	WriteOpenResponsesError(w, s.logger, "conversation not found", "not_found", http.StatusNotFound, nil, nil)
	return false
}
