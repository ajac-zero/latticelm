package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/yourusername/go-llm-gateway/internal/api"
	"github.com/yourusername/go-llm-gateway/internal/conversation"
	"github.com/yourusername/go-llm-gateway/internal/providers"
)

// GatewayServer hosts the Open Responses API for the gateway.
type GatewayServer struct {
	registry *providers.Registry
	convs    *conversation.Store
	logger   *log.Logger
}

// New creates a GatewayServer bound to the provider registry.
func New(registry *providers.Registry, convs *conversation.Store, logger *log.Logger) *GatewayServer {
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

	// Build full message history
	messages := s.buildMessageHistory(&req)
	if messages == nil {
		http.Error(w, "conversation not found", http.StatusNotFound)
		return
	}

	// Update request with full history for provider
	fullReq := req
	fullReq.Input = messages

	provider, err := s.resolveProvider(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Resolve provider_model_id (e.g., Azure deployment name) before sending to provider
	fullReq.Model = s.registry.ResolveModelID(req.Model)

	// Handle streaming vs non-streaming
	if req.Stream {
		s.handleStreamingResponse(w, r, provider, &fullReq, &req)
	} else {
		s.handleSyncResponse(w, r, provider, &fullReq, &req)
	}
}

func (s *GatewayServer) buildMessageHistory(req *api.ResponseRequest) []api.Message {
	// If no previous_response_id, use input as-is
	if req.PreviousResponseID == "" {
		return req.Input
	}

	// Load previous conversation
	conv, ok := s.convs.Get(req.PreviousResponseID)
	if !ok {
		return nil
	}

	// Append new input to conversation history
	messages := make([]api.Message, len(conv.Messages))
	copy(messages, conv.Messages)
	messages = append(messages, req.Input...)

	return messages
}

func (s *GatewayServer) handleSyncResponse(w http.ResponseWriter, r *http.Request, provider providers.Provider, fullReq *api.ResponseRequest, origReq *api.ResponseRequest) {
	resp, err := provider.Generate(r.Context(), fullReq)
	if err != nil {
		s.logger.Printf("provider %s error: %v", provider.Name(), err)
		http.Error(w, "provider error", http.StatusBadGateway)
		return
	}

	// Store conversation - use previous_response_id if continuing, otherwise use new ID
	conversationID := origReq.PreviousResponseID
	if conversationID == "" {
		conversationID = resp.ID
	}
	
	messages := append(fullReq.Input, resp.Output...)
	s.convs.Create(conversationID, resp.Model, messages)
	
	// Return the conversation ID (not the provider's response ID)
	resp.ID = conversationID

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *GatewayServer) handleStreamingResponse(w http.ResponseWriter, r *http.Request, provider providers.Provider, fullReq *api.ResponseRequest, origReq *api.ResponseRequest) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	chunkChan, errChan := provider.GenerateStream(r.Context(), fullReq)
	
	var responseID string
	var fullText string

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				return
			}
			
			// Capture response ID
			if chunk.ID != "" && responseID == "" {
				responseID = chunk.ID
			}
			
			// Override chunk ID with conversation ID
			if origReq.PreviousResponseID != "" {
				chunk.ID = origReq.PreviousResponseID
			} else if responseID != "" {
				chunk.ID = responseID
			}
			
			// Accumulate text from deltas
			if chunk.Delta != nil && len(chunk.Delta.Content) > 0 {
				for _, block := range chunk.Delta.Content {
					fullText += block.Text
				}
			}
			
			data, err := json.Marshal(chunk)
			if err != nil {
				s.logger.Printf("failed to marshal chunk: %v", err)
				continue
			}
			
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			
			if chunk.Done {
				// Store conversation with a single consolidated assistant message
				s.storeStreamConversation(fullReq, origReq, responseID, fullText)
				return
			}
			
		case err := <-errChan:
			if err != nil {
				s.logger.Printf("stream error: %v", err)
				errData, _ := json.Marshal(map[string]string{"error": err.Error()})
				fmt.Fprintf(w, "data: %s\n\n", errData)
				flusher.Flush()
			}
			// Store whatever we accumulated before the error
			s.storeStreamConversation(fullReq, origReq, responseID, fullText)
			return
			
		case <-r.Context().Done():
			s.logger.Printf("client disconnected")
			return
		}
	}
}

func (s *GatewayServer) storeStreamConversation(fullReq *api.ResponseRequest, origReq *api.ResponseRequest, responseID string, fullText string) {
	if responseID == "" || fullText == "" {
		return
	}

	assistantMsg := api.Message{
		Role: "assistant",
		Content: []api.ContentBlock{
			{Type: "output_text", Text: fullText},
		},
	}
	messages := append(fullReq.Input, assistantMsg)

	conversationID := origReq.PreviousResponseID
	if conversationID == "" {
		conversationID = responseID
	}

	s.convs.Create(conversationID, fullReq.Model, messages)
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
