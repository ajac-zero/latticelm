package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.ResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Check if error is due to request size limit
		if err.Error() == "http: request body too large" {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Enforce per-request token limits
	if s.tokenLimits.MaxOutputTokens > 0 && req.MaxOutputTokens != nil && *req.MaxOutputTokens > s.tokenLimits.MaxOutputTokens {
		WriteJSONError(w, s.logger, fmt.Sprintf(
			"max_output_tokens %d exceeds configured limit of %d",
			*req.MaxOutputTokens, s.tokenLimits.MaxOutputTokens,
		), http.StatusBadRequest)
		return
	}

	// Normalize input to internal messages
	inputMsgs := req.NormalizeInput()

	// Extract caller principal (nil when auth is disabled).
	principal := auth.PrincipalFromContext(r.Context())

	// Build full message history from previous conversation
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
			http.Error(w, "error retrieving conversation", http.StatusInternalServerError)
			return
		}
		if conv == nil {
			s.logger.WarnContext(r.Context(), "conversation not found",
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("conversation_id", *req.PreviousResponseID),
			)
			http.Error(w, "conversation not found", http.StatusNotFound)
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
				http.Error(w, "conversation not found", http.StatusNotFound)
				return
			}
		}

		historyMsgs = conv.Messages
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

	provider, err := s.resolveProvider(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
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
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from path: /v1/responses/{id}
	id := strings.TrimPrefix(r.URL.Path, "/v1/responses/")
	if id == "" {
		http.Error(w, "response id is required", http.StatusBadRequest)
		return
	}

	conv, err := s.convs.Get(r.Context(), id)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "failed to look up conversation for deletion",
			slog.String("request_id", logger.FromContext(r.Context())),
			slog.String("response_id", id),
			slog.String("error", err.Error()),
		)
		http.Error(w, "error looking up conversation", http.StatusInternalServerError)
		return
	}
	if conv == nil {
		http.Error(w, "conversation not found", http.StatusNotFound)
		return
	}

	if err := s.convs.Delete(r.Context(), id); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to delete conversation",
			slog.String("request_id", logger.FromContext(r.Context())),
			slog.String("response_id", id),
			slog.String("error", err.Error()),
		)
		http.Error(w, "error deleting conversation", http.StatusInternalServerError)
		return
	}

	s.logger.InfoContext(r.Context(), "conversation deleted",
		slog.String("request_id", logger.FromContext(r.Context())),
		slog.String("response_id", id),
	)

	w.WriteHeader(http.StatusNoContent)
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

		// Check if error is from circuit breaker
		if isCircuitBreakerError(err) {
			http.Error(w, "service temporarily unavailable - circuit breaker open", http.StatusServiceUnavailable)
		} else {
			http.Error(w, "provider error", http.StatusBadGateway)
		}
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
		if _, err := s.convs.Create(r.Context(), responseID, result.Model, allMsgs, ownerFromPrincipal(auth.PrincipalFromContext(r.Context()))); err != nil {
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
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	responseID := generateID("resp_")
	itemID := generateID("msg_")
	seq := 0
	outputIdx := 0
	contentIdx := 0

	// Build initial response snapshot (in_progress, no output yet)
	initialResp := s.buildResponse(origReq, &api.ProviderResult{
		Model: origReq.Model,
	}, provider.Name(), responseID)
	initialResp.Status = "in_progress"
	initialResp.CompletedAt = nil
	initialResp.Output = []api.OutputItem{}
	initialResp.Usage = nil

	// response.created
	s.sendSSE(w, flusher, &seq, "response.created", &api.StreamEvent{
		Type:     "response.created",
		Response: initialResp,
	})

	// response.in_progress
	s.sendSSE(w, flusher, &seq, "response.in_progress", &api.StreamEvent{
		Type:     "response.in_progress",
		Response: initialResp,
	})

	// response.output_item.added
	inProgressItem := &api.OutputItem{
		ID:      itemID,
		Type:    "message",
		Status:  "in_progress",
		Role:    "assistant",
		Content: []api.ContentPart{},
	}
	s.sendSSE(w, flusher, &seq, "response.output_item.added", &api.StreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: &outputIdx,
		Item:        inProgressItem,
	})

	// response.content_part.added
	emptyPart := &api.ContentPart{
		Type:        "output_text",
		Text:        "",
		Annotations: []api.Annotation{},
	}
	s.sendSSE(w, flusher, &seq, "response.content_part.added", &api.StreamEvent{
		Type:         "response.content_part.added",
		ItemID:       itemID,
		OutputIndex:  &outputIdx,
		ContentIndex: &contentIdx,
		Part:         emptyPart,
	})

	// Start provider stream
	deltaChan, errChan := provider.GenerateStream(r.Context(), providerMsgs, resolvedReq)

	var fullText string
	var streamErr error
	var providerModel string
	var streamUsage *api.Usage

	// Track tool calls being built
	type toolCallBuilder struct {
		itemID    string
		id        string
		name      string
		arguments string
	}
	toolCallsInProgress := make(map[int]*toolCallBuilder)
	nextOutputIdx := 0
	textItemAdded := false

loop:
	for {
		select {
		case delta, ok := <-deltaChan:
			if !ok {
				// deltaChan closed — drain errChan to catch any error
				// that was sent before the goroutine exited.
				if err, ok := <-errChan; ok && err != nil {
					streamErr = err
				}
				break loop
			}
			if delta.Model != "" && providerModel == "" {
				providerModel = delta.Model
			}

			// Handle text content
			if delta.Text != "" {
				// Add text item on first text delta
				if !textItemAdded {
					textItemAdded = true
					nextOutputIdx++
				}
				fullText += delta.Text
				s.sendSSE(w, flusher, &seq, "response.output_text.delta", &api.StreamEvent{
					Type:         "response.output_text.delta",
					ItemID:       itemID,
					OutputIndex:  &outputIdx,
					ContentIndex: &contentIdx,
					Delta:        delta.Text,
				})
			}

			// Handle tool call delta
			if delta.ToolCallDelta != nil {
				tc := delta.ToolCallDelta

				// First chunk for this tool call index
				if _, exists := toolCallsInProgress[tc.Index]; !exists {
					toolItemID := generateID("item_")
					toolOutputIdx := nextOutputIdx
					nextOutputIdx++

					// Send response.output_item.added
					s.sendSSE(w, flusher, &seq, "response.output_item.added", &api.StreamEvent{
						Type:        "response.output_item.added",
						OutputIndex: &toolOutputIdx,
						Item: &api.OutputItem{
							ID:     toolItemID,
							Type:   "function_call",
							Status: "in_progress",
							CallID: tc.ID,
							Name:   tc.Name,
						},
					})

					toolCallsInProgress[tc.Index] = &toolCallBuilder{
						itemID:    toolItemID,
						id:        tc.ID,
						name:      tc.Name,
						arguments: "",
					}
				}

				// Send function_call_arguments.delta
				if tc.Arguments != "" {
					builder := toolCallsInProgress[tc.Index]
					builder.arguments += tc.Arguments
					toolOutputIdx := outputIdx + 1 + tc.Index

					s.sendSSE(w, flusher, &seq, "response.function_call_arguments.delta", &api.StreamEvent{
						Type:        "response.function_call_arguments.delta",
						ItemID:      builder.itemID,
						OutputIndex: &toolOutputIdx,
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

	if streamErr != nil {
		s.logger.ErrorContext(r.Context(), "stream error",
			logger.LogAttrsWithTrace(r.Context(),
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("provider", provider.Name()),
				slog.String("model", origReq.Model),
				slog.String("error", streamErr.Error()),
			)...,
		)

		// Determine error type based on circuit breaker state
		errorType := "server_error"
		errorMessage := streamErr.Error()
		if isCircuitBreakerError(streamErr) {
			errorType = "circuit_breaker_open"
			errorMessage = "service temporarily unavailable - circuit breaker open"
		}

		failedResp := s.buildResponse(origReq, &api.ProviderResult{
			Model: origReq.Model,
		}, provider.Name(), responseID)
		failedResp.Status = "failed"
		failedResp.CompletedAt = nil
		failedResp.Output = []api.OutputItem{}
		failedResp.Error = &api.ResponseError{
			Type:    errorType,
			Message: errorMessage,
		}
		s.sendSSE(w, flusher, &seq, "response.failed", &api.StreamEvent{
			Type:     "response.failed",
			Response: failedResp,
		})
		return
	}

	// Send done events for text output if text was added
	if textItemAdded && fullText != "" {
		// response.output_text.done
		s.sendSSE(w, flusher, &seq, "response.output_text.done", &api.StreamEvent{
			Type:         "response.output_text.done",
			ItemID:       itemID,
			OutputIndex:  &outputIdx,
			ContentIndex: &contentIdx,
			Text:         fullText,
		})

		// response.content_part.done
		completedPart := &api.ContentPart{
			Type:        "output_text",
			Text:        fullText,
			Annotations: []api.Annotation{},
		}
		s.sendSSE(w, flusher, &seq, "response.content_part.done", &api.StreamEvent{
			Type:         "response.content_part.done",
			ItemID:       itemID,
			OutputIndex:  &outputIdx,
			ContentIndex: &contentIdx,
			Part:         completedPart,
		})

		// response.output_item.done
		completedItem := &api.OutputItem{
			ID:      itemID,
			Type:    "message",
			Status:  "completed",
			Role:    "assistant",
			Content: []api.ContentPart{*completedPart},
		}
		s.sendSSE(w, flusher, &seq, "response.output_item.done", &api.StreamEvent{
			Type:        "response.output_item.done",
			OutputIndex: &outputIdx,
			Item:        completedItem,
		})
	}

	// Send done events for each tool call
	for idx, builder := range toolCallsInProgress {
		toolOutputIdx := outputIdx + 1 + idx

		s.sendSSE(w, flusher, &seq, "response.function_call_arguments.done", &api.StreamEvent{
			Type:        "response.function_call_arguments.done",
			ItemID:      builder.itemID,
			OutputIndex: &toolOutputIdx,
			Arguments:   builder.arguments,
		})

		s.sendSSE(w, flusher, &seq, "response.output_item.done", &api.StreamEvent{
			Type:        "response.output_item.done",
			OutputIndex: &toolOutputIdx,
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

	// Build final completed response
	model := origReq.Model
	if providerModel != "" {
		model = providerModel
	}

	// Collect tool calls for result
	var toolCalls []api.ToolCall
	for _, builder := range toolCallsInProgress {
		toolCalls = append(toolCalls, api.ToolCall{
			ID:        builder.id,
			Name:      builder.name,
			Arguments: builder.arguments,
		})
	}

	finalResult := &api.ProviderResult{
		Model:     model,
		Text:      fullText,
		ToolCalls: toolCalls,
	}
	if streamUsage != nil {
		finalResult.Usage = *streamUsage
	}
	completedResp := s.buildResponse(origReq, finalResult, provider.Name(), responseID)

	// Update item IDs to match what we sent during streaming
	if textItemAdded && len(completedResp.Output) > 0 {
		completedResp.Output[0].ID = itemID
	}
	for idx, builder := range toolCallsInProgress {
		// Find the corresponding output item
		for i := range completedResp.Output {
			if completedResp.Output[i].Type == "function_call" && completedResp.Output[i].CallID == builder.id {
				completedResp.Output[i].ID = builder.itemID
				break
			}
		}
		_ = idx // unused
	}

	// response.completed
	s.sendSSE(w, flusher, &seq, "response.completed", &api.StreamEvent{
		Type:     "response.completed",
		Response: completedResp,
	})

	// Store conversation only when storage policy allows
	if s.shouldStore(origReq) && (fullText != "" || len(toolCalls) > 0) {
		assistantMsg := api.Message{
			Role:      "assistant",
			Content:   []api.ContentBlock{{Type: "output_text", Text: fullText}},
			ToolCalls: toolCalls,
		}
		allMsgs := append(storeMsgs, assistantMsg)
		if _, err := s.convs.Create(r.Context(), responseID, model, allMsgs, ownerFromPrincipal(auth.PrincipalFromContext(r.Context()))); err != nil {
			s.logger.ErrorContext(r.Context(), "failed to store conversation",
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("response_id", responseID),
				slog.String("error", err.Error()),
			)
		}
	}

	// Record token usage for distributed quota tracking
	if completedResp.Usage != nil {
		ratelimit.RecordUsageFromContext(r.Context(), completedResp.Usage.InputTokens, completedResp.Usage.OutputTokens)

		// Record token usage for analytics
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

	if fullText != "" || len(toolCalls) > 0 {
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
