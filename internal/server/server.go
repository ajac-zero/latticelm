package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/providers"
)

// GatewayServer hosts the Open Responses API for the gateway.
type GatewayServer struct {
	registry *providers.Registry
	convs    conversation.Store
	logger   *log.Logger
}

// New creates a GatewayServer bound to the provider registry.
func New(registry *providers.Registry, convs conversation.Store, logger *log.Logger) *GatewayServer {
	return &GatewayServer{
		registry: registry,
		convs:    convs,
		logger:   logger,
	}
}

// RegisterRoutes wires the HTTP handlers onto the provided mux.
func (s *GatewayServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/responses", s.handleResponses)
	mux.HandleFunc("/v1/models", s.handleModels)
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
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *GatewayServer) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req api.ResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Normalize input to internal messages
	inputMsgs := req.NormalizeInput()

	// Build full message history from previous conversation
	var historyMsgs []api.Message
	if req.PreviousResponseID != nil && *req.PreviousResponseID != "" {
		conv, ok := s.convs.Get(*req.PreviousResponseID)
		if !ok {
			http.Error(w, "conversation not found", http.StatusNotFound)
			return
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

func (s *GatewayServer) handleSyncResponse(w http.ResponseWriter, r *http.Request, provider providers.Provider, providerMsgs []api.Message, resolvedReq *api.ResponseRequest, origReq *api.ResponseRequest, storeMsgs []api.Message) {
	result, err := provider.Generate(r.Context(), providerMsgs, resolvedReq)
	if err != nil {
		s.logger.Printf("provider %s error: %v", provider.Name(), err)
		http.Error(w, "provider error", http.StatusBadGateway)
		return
	}

	responseID := generateID("resp_")

	// Build assistant message for conversation store
	assistantMsg := api.Message{
		Role:    "assistant",
		Content: []api.ContentBlock{{Type: "output_text", Text: result.Text}},
	}
	allMsgs := append(storeMsgs, assistantMsg)
	s.convs.Create(responseID, result.Model, allMsgs)

	// Build spec-compliant response
	resp := s.buildResponse(origReq, result, provider.Name(), responseID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *GatewayServer) handleStreamingResponse(w http.ResponseWriter, r *http.Request, provider providers.Provider, providerMsgs []api.Message, resolvedReq *api.ResponseRequest, origReq *api.ResponseRequest, storeMsgs []api.Message) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

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

			if delta.Done {
				break loop
			}
		case err := <-errChan:
			if err != nil {
				streamErr = err
			}
			break loop
		case <-r.Context().Done():
			s.logger.Printf("client disconnected")
			return
		}
	}

	if streamErr != nil {
		s.logger.Printf("stream error: %v", streamErr)
		failedResp := s.buildResponse(origReq, &api.ProviderResult{
			Model: origReq.Model,
		}, provider.Name(), responseID)
		failedResp.Status = "failed"
		failedResp.CompletedAt = nil
		failedResp.Output = []api.OutputItem{}
		failedResp.Error = &api.ResponseError{
			Type:    "server_error",
			Message: streamErr.Error(),
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

	// Store conversation
	if fullText != "" {
		assistantMsg := api.Message{
			Role:    "assistant",
			Content: []api.ContentBlock{{Type: "output_text", Text: fullText}},
		}
		allMsgs := append(storeMsgs, assistantMsg)
		s.convs.Create(responseID, model, allMsgs)
	}
}

func (s *GatewayServer) sendSSE(w http.ResponseWriter, flusher http.Flusher, seq *int, eventType string, event *api.StreamEvent) {
	event.SequenceNumber = *seq
	*seq++
	data, err := json.Marshal(event)
	if err != nil {
		s.logger.Printf("failed to marshal SSE event: %v", err)
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
	store := true
	if req.Store != nil {
		store = *req.Store
	}
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

	var usage *api.Usage
	if result.Text != "" {
		usage = &result.Usage
	}

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

func generateID(prefix string) string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	return prefix + id[:24]
}
