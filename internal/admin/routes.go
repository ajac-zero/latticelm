package admin

import (
	"net/http"
)

// RegisterRoutes wires the admin HTTP handlers onto the provided mux.
func (s *AdminServer) RegisterRoutes(mux *http.ServeMux) {
	// API endpoints
	mux.HandleFunc("/admin/api/v1/system/info", s.handleSystemInfo)
	mux.HandleFunc("/admin/api/v1/system/health", s.handleSystemHealth)
	mux.HandleFunc("/admin/api/v1/config", s.handleConfig)
	mux.HandleFunc("/admin/api/v1/providers", s.handleProviders)

	// Serve frontend SPA
	mux.Handle("/admin/", http.StripPrefix("/admin", s.serveSPA()))
}
