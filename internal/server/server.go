package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sony/gobreaker"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/logger"
	"github.com/ajac-zero/latticelm/internal/providers"
	"github.com/ajac-zero/latticelm/internal/ratelimit"
	"github.com/ajac-zero/latticelm/internal/usage"
)

// ProviderRegistry is an interface for provider registries.
type ProviderRegistry interface {
	Get(name string) (providers.Provider, bool)
	Models() []struct{ Provider, Model string }
	ResolveModelID(model string) string
	Default(model string) (providers.Provider, error)
}

// TokenLimits defines per-request token limits enforced by the gateway.
type TokenLimits struct {
	MaxPromptTokens int
	MaxOutputTokens int
}

// GatewayServer hosts the Open Responses API for the gateway.
type GatewayServer struct {
	registry       ProviderRegistry
	convs          conversation.Store
	logger         *slog.Logger
	tokenLimits    TokenLimits
	storeByDefault bool
	adminConfig    auth.AdminConfig
}

type streamMessageBuilder struct {
	itemID       string
	outputIndex  int
	contentIndex int
	text         string
}

type streamToolCallBuilder struct {
	streamIndex int
	outputIndex int
	itemID      string
	id          string
	name        string
	arguments   string
}

// New creates a GatewayServer bound to the provider registry.
func New(registry ProviderRegistry, convs conversation.Store, logger *slog.Logger, opts ...Option) *GatewayServer {
	s := &GatewayServer{
		registry:       registry,
		convs:          convs,
		logger:         logger,
		storeByDefault: false,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Option configures optional GatewayServer behaviour.
type Option func(*GatewayServer)

// WithAdminConfig attaches the admin authorization config used for
// conversation ownership overrides.
func WithAdminConfig(cfg auth.AdminConfig) Option {
	return func(s *GatewayServer) {
		s.adminConfig = cfg
	}
}

// SetStoreByDefault configures whether conversations are stored when the
// client does not explicitly set the "store" field.
func (s *GatewayServer) SetStoreByDefault(v bool) {
	s.storeByDefault = v
}

// shouldStore determines whether the conversation should be persisted based
// on the request's store field and the server-level default policy.
func (s *GatewayServer) shouldStore(req *api.ResponseRequest) bool {
	if req.Store != nil {
		return *req.Store
	}
	return s.storeByDefault
}

// SetTokenLimits configures per-request token limits.
func (s *GatewayServer) SetTokenLimits(limits TokenLimits) {
	s.tokenLimits = limits
}

// isCircuitBreakerError checks if the error is from a circuit breaker.
func isCircuitBreakerError(err error) bool {
	return errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)
}

// RegisterRoutes wires the HTTP handlers onto the provided mux.
func (s *GatewayServer) RegisterRoutes(mux *http.ServeMux) {
	s.RegisterAPIRoutes(mux)
	s.RegisterPublicRoutes(mux)
}

// RegisterAPIRoutes wires the authenticated API handlers onto the provided mux.
func (s *GatewayServer) RegisterAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/responses", s.handleResponses)
	mux.HandleFunc("/v1/responses/", s.handleResponseByID)
	mux.HandleFunc("/v1/models", s.handleModels)
}

// RegisterAdminAPIRoutes registers the core API handlers under /api/v1/ so that
// the embedded admin UI (session-based auth) can reach them without a JWT token.
func (s *GatewayServer) RegisterAdminAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/responses", s.handleResponses)
	mux.HandleFunc("/api/v1/responses/", s.handleResponseByID)
	mux.HandleFunc("/api/v1/models", s.handleModels)
}

// RegisterPublicRoutes wires the unauthenticated probe handlers onto the provided mux.
func (s *GatewayServer) RegisterPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
}

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

		// Enforce ownership / tenant isolation when auth is enabled.
		if principal != nil && !principal.OwnsConversation(conv.OwnerIss, conv.OwnerSub, conv.TenantID) {
			if principal.HasAdminRole(s.adminConfig) {
				s.logger.WarnContext(r.Context(), "admin override: accessing conversation owned by another user",
					slog.String("request_id", logger.FromContext(r.Context())),
					slog.String("conversation_id", *req.PreviousResponseID),
					slog.String("admin_sub", principal.Subject),
					slog.String("admin_iss", principal.Issuer),
					slog.String("owner_sub", conv.OwnerSub),
					slog.String("owner_iss", conv.OwnerIss),
					slog.String("owner_tenant", conv.TenantID),
				)
			} else {
				// Return 404 to avoid leaking conversation existence.
				s.logger.WarnContext(r.Context(), "conversation ownership check failed",
					slog.String("request_id", logger.FromContext(r.Context())),
					slog.String("conversation_id", *req.PreviousResponseID),
					slog.String("caller_sub", principal.Subject),
					slog.String("caller_iss", principal.Issuer),
				)
				WriteOpenResponsesError(w, s.logger, "conversation not found", "not_found", http.StatusNotFound, nil, nil)
				return
			}
		}

		historyMsgs = conv.Messages
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

	inputMsgs := req.NormalizeInput()

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

	provider, err := s.resolveProvider(&req)
	if err != nil {
		WriteOpenResponsesError(w, s.logger, err.Error(), "invalid_request", http.StatusBadRequest, nil, nil)
		return
	}

	// Resolve provider_model_id (e.g., Azure deployment name)
	resolvedReq := req
	resolvedReq.Model = s.registry.ResolveModelID(req.Model)

	if req.Stream {
		s.handleStreamingResponse(w, r, provider, providerMsgs, &resolvedReq, &req, storeMsgs)
	} else {
		s.handleSyncResponse(w, r, provider, providerMsgs, &resolvedReq, &req, storeMsgs)
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

	// Enforce ownership / tenant isolation when auth is enabled.
	principal := auth.PrincipalFromContext(r.Context())
	if principal != nil && !principal.OwnsConversation(conv.OwnerIss, conv.OwnerSub, conv.TenantID) {
		if principal.HasAdminRole(s.adminConfig) {
			s.logger.WarnContext(r.Context(), "admin override: accessing conversation owned by another user",
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("conversation_id", id),
				slog.String("admin_sub", principal.Subject),
				slog.String("admin_iss", principal.Issuer),
				slog.String("owner_sub", conv.OwnerSub),
				slog.String("owner_iss", conv.OwnerIss),
				slog.String("owner_tenant", conv.TenantID),
			)
		} else {
			// Return 404 to avoid leaking conversation existence.
			s.logger.WarnContext(r.Context(), "conversation ownership check failed",
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("conversation_id", id),
				slog.String("caller_sub", principal.Subject),
				slog.String("caller_iss", principal.Issuer),
			)
			WriteOpenResponsesError(w, s.logger, "conversation not found", "not_found", http.StatusNotFound, nil, nil)
			return
		}
	}

	// Build spec-compliant response from stored conversation
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

func (s *GatewayServer) buildResponseFromConversation(conv *conversation.Conversation) *api.Response {
	// Build output items from the last assistant message
	var outputItems []api.OutputItem
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		msg := conv.Messages[i]
		if msg.Role == "assistant" {
			outputItems = buildOutputItemsFromMessage(msg)
			break
		}
	}

	// Use stored request parameters with defaults
	req := conv.Request
	tools := json.RawMessage(`[]`)
	toolChoice := json.RawMessage(`"auto"`)
	text := json.RawMessage(`{"format":{"type":"text"}}`)
	truncation := "disabled"
	temperature := 1.0
	topP := 1.0
	presencePenalty := 0.0
	frequencyPenalty := 0.0
	topLogprobs := 0
	parallelToolCalls := true
	serviceTier := "default"
	var reasoning json.RawMessage

	if req != nil {
		if req.Tools != nil {
			tools = req.Tools
		}
		if req.ToolChoice != nil {
			toolChoice = req.ToolChoice
		}
		if req.Text != nil {
			text = req.Text
		}
		if req.Truncation != nil {
			truncation = *req.Truncation
		}
		if req.Temperature != nil {
			temperature = *req.Temperature
		}
		if req.TopP != nil {
			topP = *req.TopP
		}
		if req.PresencePenalty != nil {
			presencePenalty = *req.PresencePenalty
		}
		if req.FrequencyPenalty != nil {
			frequencyPenalty = *req.FrequencyPenalty
		}
		if req.TopLogprobs != nil {
			topLogprobs = *req.TopLogprobs
		}
		if req.ParallelToolCalls != nil {
			parallelToolCalls = *req.ParallelToolCalls
		}
		if req.ServiceTier != nil {
			serviceTier = *req.ServiceTier
		}
		if req.Reasoning != nil {
			reasoning = req.Reasoning
		}
	}

	metadata := map[string]string{}
	if req != nil && req.Metadata != nil {
		metadata = req.Metadata
	}

	return &api.Response{
		ID:                conv.ID,
		Object:            "response",
		CreatedAt:         conv.CreatedAt.Unix(),
		CompletedAt:       ptrInt64(conv.UpdatedAt.Unix()),
		Status:            "completed",
		Model:             conv.Model,
		PreviousResponseID: previousResponseIDFromRequest(req),
		Instructions:      instructionsFromRequest(req),
		Output:            outputItems,
		Tools:             tools,
		ToolChoice:        toolChoice,
		Truncation:        truncation,
		ParallelToolCalls: parallelToolCalls,
		Text:              text,
		TopP:              topP,
		PresencePenalty:   presencePenalty,
		FrequencyPenalty:  frequencyPenalty,
		TopLogprobs:       topLogprobs,
		Temperature:       temperature,
		Reasoning:         reasoning,
		Usage:             nil, // Usage not stored in conversation
		MaxOutputTokens:   maxOutputTokensFromRequest(req),
		MaxToolCalls:      maxToolCallsFromRequest(req),
		Store:             true, // If we retrieved it, it was stored
		Background:        false,
		ServiceTier:       serviceTier,
		Metadata:          metadata,
	}
}

func buildOutputItemsFromMessage(msg api.Message) []api.OutputItem {
	var items []api.OutputItem

	// Add message item with content
	if len(msg.Content) > 0 {
		contentParts := make([]api.ContentPart, len(msg.Content))
		for i, block := range msg.Content {
			contentParts[i] = api.ContentPart{
				Type:        block.Type,
				Text:        block.Text,
				Annotations: []api.Annotation{},
			}
		}
		items = append(items, api.OutputItem{
			ID:      generateID("msg_"),
			Type:    "message",
			Status:  "completed",
			Role:    "assistant",
			Content: contentParts,
		})
	}

	// Add function_call items
	for _, tc := range msg.ToolCalls {
		items = append(items, api.OutputItem{
			ID:        generateID("item_"),
			Type:      "function_call",
			Status:    "completed",
			CallID:    tc.ID,
			Name:      tc.Name,
			Arguments: tc.Arguments,
		})
	}

	return items
}

func ptrInt64(v int64) *int64 {
	return &v
}

func previousResponseIDFromRequest(req *api.ResponseRequest) *string {
	if req == nil || req.PreviousResponseID == nil || *req.PreviousResponseID == "" {
		return nil
	}
	return req.PreviousResponseID
}

func instructionsFromRequest(req *api.ResponseRequest) *string {
	if req == nil || req.Instructions == nil || *req.Instructions == "" {
		return nil
	}
	return req.Instructions
}

func maxOutputTokensFromRequest(req *api.ResponseRequest) *int {
	if req == nil {
		return nil
	}
	return req.MaxOutputTokens
}

func maxToolCallsFromRequest(req *api.ResponseRequest) *int {
	if req == nil {
		return nil
	}
	return req.MaxToolCalls
}

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

	// Persist conversation only when storage policy allows
	if s.shouldStore(origReq) {
		assistantMsg := api.Message{
			Role:      "assistant",
			Content:   []api.ContentBlock{{Type: "output_text", Text: result.Text}},
			ToolCalls: result.ToolCalls,
		}
		allMsgs := append(storeMsgs, assistantMsg)
		if _, err := s.convs.Create(r.Context(), responseID, result.Model, allMsgs, ownerFromPrincipal(auth.PrincipalFromContext(r.Context())), origReq); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to store conversation",
				logger.LogAttrsWithTrace(r.Context(),
					slog.String("request_id", logger.FromContext(r.Context())),
					slog.String("response_id", responseID),
					slog.String("error", err.Error()),
				)...,
			)
			// Don't fail the response if storage fails
		}
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

	// Build spec-compliant response
	resp := s.buildResponse(origReq, result, provider.Name(), responseID)

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

func (s *GatewayServer) handleStreamingResponse(w http.ResponseWriter, r *http.Request, provider providers.Provider, providerMsgs []api.Message, resolvedReq *api.ResponseRequest, origReq *api.ResponseRequest, storeMsgs []api.Message) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteOpenResponsesError(w, s.logger, "streaming not supported", "server_error", http.StatusInternalServerError, nil, nil)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	responseID := generateID("resp_")
	seq := 0
	nextOutputIdx := 0

	initialResp := s.buildResponse(origReq, &api.ProviderResult{
		Model: origReq.Model,
	}, provider.Name(), responseID)
	initialResp.Status = "in_progress"
	initialResp.CompletedAt = nil
	initialResp.Output = []api.OutputItem{}
	initialResp.Usage = nil

	s.sendSSE(w, flusher, &seq, "response.in_progress", &api.StreamEvent{
		Type:     "response.in_progress",
		Response: initialResp,
	})

	var assistantMsg *streamMessageBuilder
	toolCallsInProgress := make(map[int]*streamToolCallBuilder)
	var streamErr error
	var streamRespErr *api.ResponseError
	var providerModel string
	var streamUsage *api.Usage

	ensureMessageStarted := func() {
		if assistantMsg != nil {
			return
		}
		assistantMsg = &streamMessageBuilder{
			itemID:       generateID("msg_"),
			outputIndex:  nextOutputIdx,
			contentIndex: 0,
		}
		nextOutputIdx++
		s.sendSSE(w, flusher, &seq, "response.output_item.added", &api.StreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: &assistantMsg.outputIndex,
			Item: &api.OutputItem{
				ID:      assistantMsg.itemID,
				Type:    "message",
				Status:  "in_progress",
				Role:    "assistant",
				Content: []api.ContentPart{},
			},
		})
		s.sendSSE(w, flusher, &seq, "response.content_part.added", &api.StreamEvent{
			Type:         "response.content_part.added",
			ItemID:       assistantMsg.itemID,
			OutputIndex:  &assistantMsg.outputIndex,
			ContentIndex: &assistantMsg.contentIndex,
			Part: &api.ContentPart{
				Type:        "output_text",
				Text:        "",
				Annotations: []api.Annotation{},
			},
		})
	}

	deltaChan, errChan := provider.GenerateStream(r.Context(), providerMsgs, resolvedReq)

loop:
	for {
		select {
		case delta, ok := <-deltaChan:
			if !ok {
				if err, ok := <-errChan; ok && err != nil {
					streamErr = err
				}
				break loop
			}
			if delta.Model != "" && providerModel == "" {
				providerModel = delta.Model
			}
			if delta.Text != "" {
				ensureMessageStarted()
				assistantMsg.text += delta.Text
				s.sendSSE(w, flusher, &seq, "response.output_text.delta", &api.StreamEvent{
					Type:         "response.output_text.delta",
					ItemID:       assistantMsg.itemID,
					OutputIndex:  &assistantMsg.outputIndex,
					ContentIndex: &assistantMsg.contentIndex,
					Delta:        delta.Text,
				})
			}
			if delta.ToolCallDelta != nil {
				tc := delta.ToolCallDelta
				builder, exists := toolCallsInProgress[tc.Index]
				if !exists {
					builder = &streamToolCallBuilder{
						streamIndex: tc.Index,
						outputIndex: nextOutputIdx,
						itemID:      generateID("item_"),
						id:          tc.ID,
						name:        tc.Name,
					}
					nextOutputIdx++
					toolCallsInProgress[tc.Index] = builder
					if respErr := validateToolCallName(origReq, builder.name); respErr != nil {
						streamRespErr = respErr
						break loop
					}
					s.sendSSE(w, flusher, &seq, "response.output_item.added", &api.StreamEvent{
						Type:        "response.output_item.added",
						OutputIndex: &builder.outputIndex,
						Item: &api.OutputItem{
							ID:        builder.itemID,
							Type:      "function_call",
							Status:    "in_progress",
							CallID:    builder.id,
							Name:      builder.name,
							Arguments: "",
						},
					})
				}
				if builder.id == "" && tc.ID != "" {
					builder.id = tc.ID
				}
				if tc.Name != "" && builder.name == "" {
					builder.name = tc.Name
					if respErr := validateToolCallName(origReq, builder.name); respErr != nil {
						streamRespErr = respErr
						break loop
					}
				}
				if tc.Arguments != "" {
					builder.arguments += tc.Arguments
					s.sendSSE(w, flusher, &seq, "response.function_call_arguments.delta", &api.StreamEvent{
						Type:        "response.function_call_arguments.delta",
						ItemID:      builder.itemID,
						OutputIndex: &builder.outputIndex,
						Delta:       tc.Arguments,
					})
				}
			}
			if delta.Usage != nil {
				streamUsage = delta.Usage
			}
			if delta.Done {
				break loop
			}
		case err := <-errChan:
			if err != nil {
				streamErr = err
			}
			break loop
		case <-r.Context().Done():
			s.logger.InfoContext(r.Context(), "client disconnected",
				slog.String("request_id", logger.FromContext(r.Context())),
			)
			return
		}
	}

	if streamErr != nil && streamRespErr == nil {
		s.logger.ErrorContext(r.Context(), "stream error",
			logger.LogAttrsWithTrace(r.Context(),
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("provider", provider.Name()),
				slog.String("model", origReq.Model),
				slog.String("error", streamErr.Error()),
			)...,
		)
		streamRespErr = &api.ResponseError{
			Type:    "model_error",
			Message: streamErr.Error(),
		}
		if isCircuitBreakerError(streamErr) {
			streamRespErr.Type = "server_error"
			streamRespErr.Message = "service temporarily unavailable - circuit breaker open"
		}
	}

	toolCalls := collectToolCalls(toolCallsInProgress)
	if streamRespErr == nil {
		streamRespErr = validateToolCalls(origReq, toolCalls)
	}

	if streamRespErr != nil {
		failedResp := s.buildResponse(origReq, &api.ProviderResult{
			Model: origReq.Model,
		}, provider.Name(), responseID)
		failedResp.Status = "failed"
		failedResp.CompletedAt = nil
		failedResp.Output = []api.OutputItem{}
		failedResp.Error = streamRespErr
		s.sendSSE(w, flusher, &seq, "response.failed", &api.StreamEvent{
			Type:     "response.failed",
			Response: failedResp,
		})
		s.sendSSEDone(w, flusher)
		return
	}

	if assistantMsg != nil {
		s.sendSSE(w, flusher, &seq, "response.output_text.done", &api.StreamEvent{
			Type:         "response.output_text.done",
			ItemID:       assistantMsg.itemID,
			OutputIndex:  &assistantMsg.outputIndex,
			ContentIndex: &assistantMsg.contentIndex,
			Text:         assistantMsg.text,
		})
		completedPart := &api.ContentPart{
			Type:        "output_text",
			Text:        assistantMsg.text,
			Annotations: []api.Annotation{},
		}
		s.sendSSE(w, flusher, &seq, "response.content_part.done", &api.StreamEvent{
			Type:         "response.content_part.done",
			ItemID:       assistantMsg.itemID,
			OutputIndex:  &assistantMsg.outputIndex,
			ContentIndex: &assistantMsg.contentIndex,
			Part:         completedPart,
		})
		s.sendSSE(w, flusher, &seq, "response.output_item.done", &api.StreamEvent{
			Type:        "response.output_item.done",
			OutputIndex: &assistantMsg.outputIndex,
			Item: &api.OutputItem{
				ID:      assistantMsg.itemID,
				Type:    "message",
				Status:  "completed",
				Role:    "assistant",
				Content: []api.ContentPart{*completedPart},
			},
		})
	}

	for _, builder := range sortedToolCallBuilders(toolCallsInProgress) {
		s.sendSSE(w, flusher, &seq, "response.function_call_arguments.done", &api.StreamEvent{
			Type:        "response.function_call_arguments.done",
			ItemID:      builder.itemID,
			OutputIndex: &builder.outputIndex,
			Arguments:   builder.arguments,
		})
		s.sendSSE(w, flusher, &seq, "response.output_item.done", &api.StreamEvent{
			Type:        "response.output_item.done",
			OutputIndex: &builder.outputIndex,
			Item: &api.OutputItem{
				ID:        builder.itemID,
				Type:      "function_call",
				Status:    "completed",
				CallID:    builder.id,
				Name:      builder.name,
				Arguments: builder.arguments,
			},
		})
	}

	model := origReq.Model
	if providerModel != "" {
		model = providerModel
	}
	finalResult := &api.ProviderResult{
		Model:     model,
		ToolCalls: toolCalls,
	}
	if assistantMsg != nil {
		finalResult.Text = assistantMsg.text
	}
	if streamUsage != nil {
		finalResult.Usage = *streamUsage
	}
	completedResp := s.buildResponse(origReq, finalResult, provider.Name(), responseID)
	completedResp.Output = buildOrderedStreamOutput(assistantMsg, toolCallsInProgress)

	s.sendSSE(w, flusher, &seq, "response.completed", &api.StreamEvent{
		Type:     "response.completed",
		Response: completedResp,
	})
	s.sendSSEDone(w, flusher)

	if s.shouldStore(origReq) && (finalResult.Text != "" || len(toolCalls) > 0) {
		assistantMsgToStore := api.Message{
			Role:      "assistant",
			Content:   []api.ContentBlock{{Type: "output_text", Text: finalResult.Text}},
			ToolCalls: toolCalls,
		}
		allMsgs := append(storeMsgs, assistantMsgToStore)
		if _, err := s.convs.Create(r.Context(), responseID, model, allMsgs, ownerFromPrincipal(auth.PrincipalFromContext(r.Context())), origReq); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to store conversation",
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("response_id", responseID),
				slog.String("error", err.Error()),
			)
		}
	}

	if completedResp.Usage != nil {
		ratelimit.RecordUsageFromContext(r.Context(), completedResp.Usage.InputTokens, completedResp.Usage.OutputTokens)
		principal := auth.PrincipalFromContext(r.Context())
		usage.RecordFromContext(r.Context(), usage.UsageEvent{
			TenantID:     tenantFromPrincipal(principal),
			UserSub:      subFromPrincipal(principal),
			Provider:     provider.Name(),
			Model:        model,
			InputTokens:  completedResp.Usage.InputTokens,
			OutputTokens: completedResp.Usage.OutputTokens,
			ResponseID:   responseID,
			Stream:       true,
		})
	}

	if finalResult.Text != "" || len(toolCalls) > 0 {
		s.logger.InfoContext(r.Context(), "streaming response completed",
			slog.String("request_id", logger.FromContext(r.Context())),
			slog.String("provider", provider.Name()),
			slog.String("model", model),
			slog.String("response_id", responseID),
			slog.Bool("has_tool_calls", len(toolCalls) > 0),
			slog.Bool("stored", s.shouldStore(origReq)),
		)
	}
}

func (s *GatewayServer) sendSSE(w http.ResponseWriter, flusher http.Flusher, seq *int, eventType string, event *api.StreamEvent) {
	event.SequenceNumber = *seq
	*seq++
	data, err := json.Marshal(event)
	if err != nil {
		s.logger.Error("failed to marshal SSE event",
			slog.String("event_type", eventType),
			slog.String("error", err.Error()),
		)
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
	flusher.Flush()
}

func (s *GatewayServer) sendSSEDone(w http.ResponseWriter, flusher http.Flusher) {
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func validateToolCallName(req *api.ResponseRequest, name string) *api.ResponseError {
	if req == nil {
		return nil
	}
	toolChoice, err := req.ParseToolChoice()
	if err != nil {
		return &api.ResponseError{Type: "invalid_request", Message: err.Error()}
	}
	if toolChoice.Mode == "none" {
		return &api.ResponseError{Type: "model_error", Message: "model emitted a tool call while tool_choice was none"}
	}
	if name == "" {
		return nil
	}
	declared, err := req.DeclaredToolNames()
	if err != nil {
		return &api.ResponseError{Type: "invalid_request", Message: err.Error()}
	}
	if len(declared) == 0 {
		return &api.ResponseError{Type: "model_error", Message: fmt.Sprintf("model emitted undeclared tool %q", name)}
	}
	if _, ok := declared[name]; !ok {
		return &api.ResponseError{Type: "model_error", Message: fmt.Sprintf("model emitted undeclared tool %q", name)}
	}
	switch toolChoice.Mode {
	case "function":
		if name != toolChoice.RequiredToolName {
			return &api.ResponseError{Type: "model_error", Message: fmt.Sprintf("model emitted tool %q but tool_choice required %q", name, toolChoice.RequiredToolName)}
		}
	case "allowed_tools":
		if _, ok := toolChoice.AllowedTools[name]; !ok {
			return &api.ResponseError{Type: "model_error", Message: fmt.Sprintf("model emitted disallowed tool %q", name)}
		}
	}
	return nil
}

func validateToolCalls(req *api.ResponseRequest, toolCalls []api.ToolCall) *api.ResponseError {
	if req == nil {
		return nil
	}
	for _, toolCall := range toolCalls {
		if respErr := validateToolCallName(req, toolCall.Name); respErr != nil {
			return respErr
		}
	}
	toolChoice, err := req.ParseToolChoice()
	if err != nil {
		return &api.ResponseError{Type: "invalid_request", Message: err.Error()}
	}
	if len(toolCalls) == 0 && (toolChoice.Mode == "required" || toolChoice.Mode == "any" || toolChoice.Mode == "function") {
		msg := "model did not emit a required tool call"
		if toolChoice.Mode == "function" {
			msg = fmt.Sprintf("model did not emit required tool %q", toolChoice.RequiredToolName)
		}
		return &api.ResponseError{Type: "model_error", Message: msg}
	}
	return nil
}

func collectToolCalls(builders map[int]*streamToolCallBuilder) []api.ToolCall {
	sorted := sortedToolCallBuilders(builders)
	toolCalls := make([]api.ToolCall, 0, len(sorted))
	for _, builder := range sorted {
		toolCalls = append(toolCalls, api.ToolCall{
			ID:        builder.id,
			Name:      builder.name,
			Arguments: builder.arguments,
		})
	}
	return toolCalls
}

func sortedToolCallBuilders(builders map[int]*streamToolCallBuilder) []*streamToolCallBuilder {
	sorted := make([]*streamToolCallBuilder, 0, len(builders))
	for _, builder := range builders {
		sorted = append(sorted, builder)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].outputIndex < sorted[j].outputIndex
	})
	return sorted
}

func buildOrderedStreamOutput(message *streamMessageBuilder, toolCalls map[int]*streamToolCallBuilder) []api.OutputItem {
	type orderedItem struct {
		index int
		item  api.OutputItem
	}
	items := make([]orderedItem, 0, len(toolCalls)+1)
	if message != nil {
		items = append(items, orderedItem{
			index: message.outputIndex,
			item: api.OutputItem{
				ID:     message.itemID,
				Type:   "message",
				Status: "completed",
				Role:   "assistant",
				Content: []api.ContentPart{{
					Type:        "output_text",
					Text:        message.text,
					Annotations: []api.Annotation{},
				}},
			},
		})
	}
	for _, builder := range toolCalls {
		items = append(items, orderedItem{
			index: builder.outputIndex,
			item: api.OutputItem{
				ID:        builder.itemID,
				Type:      "function_call",
				Status:    "completed",
				CallID:    builder.id,
				Name:      builder.name,
				Arguments: builder.arguments,
			},
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].index < items[j].index
	})
	output := make([]api.OutputItem, 0, len(items))
	for _, item := range items {
		output = append(output, item.item)
	}
	return output
}

func (s *GatewayServer) buildResponse(req *api.ResponseRequest, result *api.ProviderResult, providerName string, responseID string) *api.Response {
	now := time.Now().Unix()

	model := result.Model
	if model == "" {
		model = req.Model
	}

	// Build output items array
	outputItems := []api.OutputItem{}

	// Add message item if there's text
	if result.Text != "" {
		outputItems = append(outputItems, api.OutputItem{
			ID:     generateID("msg_"),
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []api.ContentPart{{
				Type:        "output_text",
				Text:        result.Text,
				Annotations: []api.Annotation{},
			}},
		})
	}

	// Add function_call items
	for _, tc := range result.ToolCalls {
		outputItems = append(outputItems, api.OutputItem{
			ID:        generateID("item_"),
			Type:      "function_call",
			Status:    "completed",
			CallID:    tc.ID,
			Name:      tc.Name,
			Arguments: tc.Arguments,
		})
	}

	// Echo back request params with defaults
	tools := req.Tools
	if tools == nil {
		tools = json.RawMessage(`[]`)
	}
	toolChoice := req.ToolChoice
	if toolChoice == nil {
		toolChoice = json.RawMessage(`"auto"`)
	}
	text := req.Text
	if text == nil {
		text = json.RawMessage(`{"format":{"type":"text"}}`)
	}
	truncation := "disabled"
	if req.Truncation != nil {
		truncation = *req.Truncation
	}
	temperature := 1.0
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	topP := 1.0
	if req.TopP != nil {
		topP = *req.TopP
	}
	presencePenalty := 0.0
	if req.PresencePenalty != nil {
		presencePenalty = *req.PresencePenalty
	}
	frequencyPenalty := 0.0
	if req.FrequencyPenalty != nil {
		frequencyPenalty = *req.FrequencyPenalty
	}
	topLogprobs := 0
	if req.TopLogprobs != nil {
		topLogprobs = *req.TopLogprobs
	}
	parallelToolCalls := true
	if req.ParallelToolCalls != nil {
		parallelToolCalls = *req.ParallelToolCalls
	}
	store := s.shouldStore(req)
	background := false
	if req.Background != nil {
		background = *req.Background
	}
	serviceTier := "default"
	if req.ServiceTier != nil {
		serviceTier = *req.ServiceTier
	}
	var reasoning json.RawMessage
	if req.Reasoning != nil {
		reasoning = req.Reasoning
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]string{}
	}

	usage := &result.Usage

	return &api.Response{
		ID:                 responseID,
		Object:             "response",
		CreatedAt:          now,
		CompletedAt:        &now,
		Status:             "completed",
		IncompleteDetails:  nil,
		Model:              model,
		PreviousResponseID: req.PreviousResponseID,
		Instructions:       req.Instructions,
		Output:             outputItems,
		Error:              nil,
		Tools:              tools,
		ToolChoice:         toolChoice,
		Truncation:         truncation,
		ParallelToolCalls:  parallelToolCalls,
		Text:               text,
		TopP:               topP,
		PresencePenalty:    presencePenalty,
		FrequencyPenalty:   frequencyPenalty,
		TopLogprobs:        topLogprobs,
		Temperature:        temperature,
		Reasoning:          reasoning,
		Usage:              usage,
		MaxOutputTokens:    req.MaxOutputTokens,
		MaxToolCalls:       req.MaxToolCalls,
		Store:              store,
		Background:         background,
		ServiceTier:        serviceTier,
		Metadata:           metadata,
		SafetyIdentifier:   nil,
		PromptCacheKey:     nil,
		Provider:           providerName,
	}
}

func (s *GatewayServer) resolveProvider(req *api.ResponseRequest) (providers.Provider, error) {
	if req.Provider != "" {
		if provider, ok := s.registry.Get(req.Provider); ok {
			return provider, nil
		}
		return nil, fmt.Errorf("provider %s not configured", req.Provider)
	}
	return s.registry.Default(req.Model)
}

func ownerFromPrincipal(p *auth.Principal) conversation.OwnerInfo {
	if p == nil {
		return conversation.OwnerInfo{}
	}
	return conversation.OwnerInfo{
		OwnerIss: p.Issuer,
		OwnerSub: p.Subject,
		TenantID: p.TenantID,
	}
}

func tenantFromPrincipal(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	return p.TenantID
}

func subFromPrincipal(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	return p.Subject
}

func generateID(prefix string) string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	return prefix + id[:24]
}
