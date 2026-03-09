package admin

import (
	"net/http"
)

// RegisterRoutes wires the admin HTTP handlers onto the provided mux.
func (s *AdminServer) RegisterRoutes(mux *http.ServeMux) {
	// API endpoints
	mux.HandleFunc("/api/v1/system/info", s.handleSystemInfo)
	mux.HandleFunc("/api/v1/system/health", s.handleSystemHealth)
	mux.HandleFunc("/api/v1/config", s.handleConfig)
	mux.HandleFunc("/api/v1/providers", s.handleProviders)

	// Lightweight config endpoint for UI (no auth required)
	mux.HandleFunc("/api/config", s.handleUIConfig)

	// Serve frontend SPA
	mux.Handle("/", s.serveSPA())
}
