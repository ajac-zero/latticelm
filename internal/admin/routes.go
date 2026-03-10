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

// RegisterPublicRoutes wires unauthenticated admin handlers onto the provided mux.
// These endpoints must remain accessible without a token so the frontend can
// determine gateway configuration before the user has authenticated.
func (s *AdminServer) RegisterPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/config", s.handleUIConfig)
}

// RegisterPublicRoutes wires the auth endpoints that do not require a session token.
// These must be registered on the root mux with higher specificity than /admin/ so
// they bypass the JWT middleware that protects the rest of the admin surface.
func (s *AdminServer) RegisterPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin/api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("/admin/api/v1/auth/logout", s.handleLogout)
}
