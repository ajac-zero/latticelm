package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/yourusername/go-llm-gateway/internal/api"
	"github.com/yourusername/go-llm-gateway/internal/providers"
)

// GatewayServer hosts the Open Responses API for the gateway.
type GatewayServer struct {
	registry *providers.Registry
	logger   *log.Logger
}

// New creates a GatewayServer bound to the provider registry.
func New(registry *providers.Registry, logger *log.Logger) *GatewayServer {
	return &GatewayServer{registry: registry, logger: logger}
}

// RegisterRoutes wires the HTTP handlers onto the provided mux.
func (s *GatewayServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/responses", s.handleResponses)
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

	provider, err := s.resolveProvider(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Handle streaming vs non-streaming
	if req.Stream {
		s.handleStreamingResponse(w, r, provider, &req)
	} else {
		s.handleSyncResponse(w, r, provider, &req)
	}
}

func (s *GatewayServer) handleSyncResponse(w http.ResponseWriter, r *http.Request, provider providers.Provider, req *api.ResponseRequest) {
	resp, err := provider.Generate(r.Context(), req)
	if err != nil {
		s.logger.Printf("provider %s error: %v", provider.Name(), err)
		http.Error(w, "provider error", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *GatewayServer) handleStreamingResponse(w http.ResponseWriter, r *http.Request, provider providers.Provider, req *api.ResponseRequest) {
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

	chunkChan, errChan := provider.GenerateStream(r.Context(), req)

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				return
			}
			
			data, err := json.Marshal(chunk)
			if err != nil {
				s.logger.Printf("failed to marshal chunk: %v", err)
				continue
			}
			
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			
			if chunk.Done {
				return
			}
			
		case err := <-errChan:
			if err != nil {
				s.logger.Printf("stream error: %v", err)
				errData, _ := json.Marshal(map[string]string{"error": err.Error()})
				fmt.Fprintf(w, "data: %s\n\n", errData)
				flusher.Flush()
			}
			return
			
		case <-r.Context().Done():
			s.logger.Printf("client disconnected")
			return
		}
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
