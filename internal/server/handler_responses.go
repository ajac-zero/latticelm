package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/logger"
)

func (s *GatewayServer) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteOpenResponsesError(w, s.logger, "method not allowed", "invalid_request", http.StatusMethodNotAllowed, nil, nil)
		return
	}

	var req api.ResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err.Error() == "http: request body too large" {
			WriteOpenResponsesError(w, s.logger, "request body too large", "invalid_request", http.StatusRequestEntityTooLarge, nil, nil)
			return
		}
		WriteOpenResponsesError(w, s.logger, "invalid JSON payload", "invalid_request", http.StatusBadRequest, nil, nil)
		return
	}

	principal := auth.PrincipalFromContext(r.Context())
	var prevConv *conversation.Conversation
	var historyMsgs []api.Message
	if req.PreviousResponseID != nil && *req.PreviousResponseID != "" {
		conv, err := s.convs.Get(r.Context(), *req.PreviousResponseID)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "failed to retrieve conversation",
				logger.LogAttrsWithTrace(r.Context(),
					slog.String("request_id", logger.FromContext(r.Context())),
					slog.String("conversation_id", *req.PreviousResponseID),
					slog.String("error", err.Error()),
				)...,
			)
			WriteOpenResponsesError(w, s.logger, "error retrieving conversation", "server_error", http.StatusInternalServerError, nil, nil)
			return
		}
		if conv == nil {
			s.logger.WarnContext(r.Context(), "conversation not found",
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("conversation_id", *req.PreviousResponseID),
			)
			WriteOpenResponsesError(w, s.logger, "conversation not found", "not_found", http.StatusNotFound, nil, nil)
			return
		}

		if !s.checkOwnership(w, r, conv, principal, *req.PreviousResponseID) {
			return
		}

		prevConv = conv
		historyMsgs = cloneMessages(conv.Messages)
		req.InheritMissingContext(conv.Request)
	}

	if err := req.Validate(); err != nil {
		WriteOpenResponsesError(w, s.logger, err.Error(), "invalid_request", http.StatusBadRequest, nil, nil)
		return
	}

	if s.tokenLimits.MaxOutputTokens > 0 && req.MaxOutputTokens != nil && *req.MaxOutputTokens > s.tokenLimits.MaxOutputTokens {
		WriteOpenResponsesError(w, s.logger, fmt.Sprintf(
			"max_output_tokens %d exceeds configured limit of %d",
			*req.MaxOutputTokens, s.tokenLimits.MaxOutputTokens,
		), "invalid_request", http.StatusBadRequest, nil, nil)
		return
	}

	inputMsgs, itemRefIDs := req.NormalizeInput()

	provider, err := s.resolveProvider(&req)
	if err != nil {
		WriteOpenResponsesError(w, s.logger, err.Error(), "invalid_request", http.StatusBadRequest, nil, nil)
		return
	}

	if prevConv != nil {
		historyMsgs = applyReplayState(historyMsgs, prevConv.Replay, provider.Name())
		if len(itemRefIDs) > 0 {
			resolvedMsgs := s.resolveItemReferences(itemRefIDs, historyMsgs, prevConv.Replay, provider.Name())
			if len(resolvedMsgs) > 0 {
				historyMsgs = append(resolvedMsgs, historyMsgs...)
			}
		}
	}

	// Combined messages for conversation storage (history + new input, no instructions)
	storeMsgs := make([]api.Message, 0, len(historyMsgs)+len(inputMsgs))
	storeMsgs = append(storeMsgs, historyMsgs...)
	storeMsgs = append(storeMsgs, inputMsgs...)

	// Build provider messages: instructions + history + input
	var providerMsgs []api.Message
	if req.Instructions != nil && *req.Instructions != "" {
		providerMsgs = append(providerMsgs, api.Message{
			Role:    "developer",
			Content: []api.ContentBlock{{Type: "input_text", Text: *req.Instructions}},
		})
	}
	providerMsgs = append(providerMsgs, storeMsgs...)

	// Resolve provider_model_id (e.g., Azure deployment name)
	resolvedReq := req
	resolvedReq.Model = s.registry.ResolveModelID(req.Model)

	if req.Stream {
		s.handleStreamingResponse(w, r, provider, providerMsgs, &resolvedReq, &req, storeMsgs)
	} else {
		s.handleSyncResponse(w, r, provider, providerMsgs, &resolvedReq, &req, storeMsgs)
	}
}
