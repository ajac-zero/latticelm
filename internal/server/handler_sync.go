package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/logger"
	"github.com/ajac-zero/latticelm/internal/providers"
	"github.com/ajac-zero/latticelm/internal/ratelimit"
	"github.com/ajac-zero/latticelm/internal/usage"
)

func (s *GatewayServer) handleSyncResponse(w http.ResponseWriter, r *http.Request, provider providers.Provider, providerMsgs []api.Message, resolvedReq *api.ResponseRequest, origReq *api.ResponseRequest, storeMsgs []api.Message) {
	result, err := provider.Generate(r.Context(), providerMsgs, resolvedReq)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "provider generation failed",
			logger.LogAttrsWithTrace(r.Context(),
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("provider", provider.Name()),
				slog.String("model", resolvedReq.Model),
				slog.String("error", err.Error()),
			)...,
		)

		message := "provider error"
		errorType := "model_error"
		if isCircuitBreakerError(err) {
			message = "service temporarily unavailable - circuit breaker open"
			errorType = "server_error"
		}
		WriteOpenResponsesError(w, s.logger, message, errorType, http.StatusInternalServerError, nil, nil)
		return
	}

	if respErr := validateToolCalls(origReq, result.ToolCalls); respErr != nil {
		WriteOpenResponsesError(w, s.logger, respErr.Message, respErr.Type, http.StatusInternalServerError, respErr.Code, respErr.Param)
		return
	}

	responseID := generateID("resp_")
	outputItems := buildOutputItems(result)
	resp := s.buildResponseWithOutput(origReq, result, provider.Name(), responseID, outputItems)

	// Persist conversation only when storage policy allows
	if s.shouldStore(origReq) {
		s.persistConversation(r.Context(), responseID, result.Model, provider.Name(), storeMsgs, result, outputItems, origReq, auth.PrincipalFromContext(r.Context()))
	}

	// Record token usage for distributed quota tracking
	ratelimit.RecordUsageFromContext(r.Context(), result.Usage.InputTokens, result.Usage.OutputTokens)

	// Record token usage for analytics
	principal := auth.PrincipalFromContext(r.Context())
	usage.RecordFromContext(r.Context(), usage.UsageEvent{
		TenantID:     tenantFromPrincipal(principal),
		UserSub:      subFromPrincipal(principal),
		Provider:     provider.Name(),
		Model:        result.Model,
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
		ResponseID:   responseID,
		Stream:       false,
	})

	s.logger.InfoContext(r.Context(), "response generated",
		logger.LogAttrsWithTrace(r.Context(),
			slog.String("request_id", logger.FromContext(r.Context())),
			slog.String("provider", provider.Name()),
			slog.String("model", result.Model),
			slog.String("response_id", responseID),
			slog.Int("input_tokens", result.Usage.InputTokens),
			slog.Int("output_tokens", result.Usage.OutputTokens),
			slog.Bool("has_tool_calls", len(result.ToolCalls) > 0),
			slog.Bool("stored", s.shouldStore(origReq)),
		)...,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to encode response",
			logger.LogAttrsWithTrace(r.Context(),
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("response_id", responseID),
				slog.String("error", err.Error()),
			)...,
		)
	}
}
