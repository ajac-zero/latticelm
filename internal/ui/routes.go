package ui

import (
	"net/http"

	"github.com/ajac-zero/latticelm/internal/conversation"
)

// RegisterRoutes wires the admin HTTP handlers onto the provided mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// API endpoints
	mux.HandleFunc("/api/v1/system/info", s.handleSystemInfo)
	mux.HandleFunc("/api/v1/system/health", s.handleSystemHealth)
	mux.HandleFunc("/api/v1/config", s.handleConfig)

	// Provider CRUD
	mux.HandleFunc("/api/v1/providers", s.handleProviders)
	mux.HandleFunc("/api/v1/providers/{name}", s.handleProviderByName)

	// Model CRUD
	mux.HandleFunc("/api/v1/config/models", s.handleConfigModels)
	mux.HandleFunc("/api/v1/config/models/{name}", s.handleConfigModelByName)

	// Conversation management
	convAPI := conversation.NewAPI(s.convStore)
	convAPI.RegisterRoutes(mux)

	// Serve frontend SPA
	mux.Handle("/", s.serveSPA())
}


