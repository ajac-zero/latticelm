package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/config"
)

// SystemInfoResponse contains system information.
type SystemInfoResponse struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GitCommit string `json:"git_commit"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
	Uptime    string `json:"uptime"`
}

// HealthCheckResponse contains health check results.
type HealthCheckResponse struct {
	Status    string                 `json:"status"`
	Timestamp string                 `json:"timestamp"`
	Checks    map[string]HealthCheck `json:"checks"`
}

// HealthCheck represents a single health check.
type HealthCheck struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ConfigResponse contains the sanitized configuration.
type ConfigResponse struct {
	Server        config.ServerConfig          `json:"server"`
	Providers     map[string]SanitizedProvider `json:"providers"`
	Models        []config.ModelEntry          `json:"models"`
	Auth          SanitizedAuthConfig          `json:"auth"`
	Conversations config.ConversationConfig    `json:"conversations"`
	Logging       config.LoggingConfig         `json:"logging"`
	RateLimit     SanitizedRateLimitConfig     `json:"rate_limit"`
	Observability SanitizedObservabilityConfig `json:"observability"`
}

// SanitizedRateLimitConfig is rate limit config with connection secrets masked.
type SanitizedRateLimitConfig struct {
	Enabled               bool     `json:"enabled"`
	RedisURL              string   `json:"redis_url,omitempty"`
	TrustedProxyCIDRs     []string `json:"trusted_proxy_cidrs,omitempty"`
	RequestsPerSecond     float64  `json:"requests_per_second"`
	Burst                 int      `json:"burst"`
	MaxPromptTokens       int      `json:"max_prompt_tokens"`
	MaxOutputTokens       int      `json:"max_output_tokens"`
	MaxConcurrentRequests int      `json:"max_concurrent_requests"`
	DailyTokenQuota       int64    `json:"daily_token_quota"`
}

// SanitizedObservabilityConfig is observability config with secrets masked.
type SanitizedObservabilityConfig struct {
	Enabled bool                   `json:"enabled"`
	Metrics config.MetricsConfig   `json:"metrics"`
	Tracing SanitizedTracingConfig `json:"tracing"`
}

// SanitizedTracingConfig is tracing config with secrets masked.
type SanitizedTracingConfig struct {
	Enabled     bool                    `json:"enabled"`
	ServiceName string                  `json:"service_name"`
	Sampler     config.SamplerConfig    `json:"sampler"`
	Exporter    SanitizedExporterConfig `json:"exporter"`
}

// SanitizedExporterConfig is exporter config with auth header values masked.
type SanitizedExporterConfig struct {
	Type     string            `json:"type"`
	Endpoint string            `json:"endpoint"`
	Insecure bool              `json:"insecure"`
	Headers  map[string]string `json:"headers,omitempty"`
}

// SanitizedProvider is a provider entry with secrets masked.
type SanitizedProvider struct {
	Type       string `json:"type"`
	APIKey     string `json:"api_key"`
	Endpoint   string `json:"endpoint,omitempty"`
	APIVersion string `json:"api_version,omitempty"`
	Project    string `json:"project,omitempty"`
	Location   string `json:"location,omitempty"`
}

// SanitizedAuthConfig is auth config with secrets masked.
type SanitizedAuthConfig struct {
	Enabled  bool   `json:"enabled"`
	Issuer   string `json:"issuer"`
	Audience string `json:"audience"`
}

// ProviderInfo contains provider information.
type ProviderInfo struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Models []string `json:"models"`
	Status string   `json:"status"`
}

// handleSystemInfo returns system information.
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	uptime := time.Since(s.startTime)

	info := SystemInfoResponse{
		Version:   s.buildInfo.Version,
		BuildTime: s.buildInfo.BuildTime,
		GitCommit: s.buildInfo.GitCommit,
		GoVersion: s.buildInfo.GoVersion,
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		Uptime:    formatDuration(uptime),
	}

	writeSuccess(w, info)
}

// handleSystemHealth returns health check results.
func (s *Server) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	checks := make(map[string]HealthCheck)
	overallStatus := "healthy"

	// Server check
	checks["server"] = HealthCheck{
		Status:  "healthy",
		Message: "Server is running",
	}

	// Provider check
	models := s.registry.Models()
	if len(models) > 0 {
		checks["providers"] = HealthCheck{
			Status:  "healthy",
			Message: "Providers configured",
		}
	} else {
		checks["providers"] = HealthCheck{
			Status:  "unhealthy",
			Message: "No providers configured",
		}
		overallStatus = "unhealthy"
	}

	// Conversation store check
	checks["conversation_store"] = HealthCheck{
		Status:  "healthy",
		Message: "Store accessible",
	}

	response := HealthCheckResponse{
		Status:    overallStatus,
		Timestamp: time.Now().Format(time.RFC3339),
		Checks:    checks,
	}

	writeSuccess(w, response)
}

// handleConfig returns the sanitized configuration.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	// Sanitize providers
	sanitizedProviders := make(map[string]SanitizedProvider)
	for name, provider := range s.cfg.Providers {
		sanitizedProviders[name] = SanitizedProvider{
			Type:       provider.Type,
			APIKey:     maskSecret(provider.APIKey),
			Endpoint:   provider.Endpoint,
			APIVersion: provider.APIVersion,
			Project:    provider.Project,
			Location:   provider.Location,
		}
	}

	// Sanitize DSN in conversations config
	convConfig := s.cfg.Conversations
	if convConfig.DSN != "" {
		convConfig.DSN = maskSecret(convConfig.DSN)
	}

	// Sanitize rate limit config (mask Redis credentials)
	sanitizedRL := SanitizedRateLimitConfig{
		Enabled:               s.cfg.RateLimit.Enabled,
		RedisURL:              maskURL(s.cfg.RateLimit.RedisURL),
		TrustedProxyCIDRs:     s.cfg.RateLimit.TrustedProxyCIDRs,
		RequestsPerSecond:     s.cfg.RateLimit.RequestsPerSecond,
		Burst:                 s.cfg.RateLimit.Burst,
		MaxPromptTokens:       s.cfg.RateLimit.MaxPromptTokens,
		MaxOutputTokens:       s.cfg.RateLimit.MaxOutputTokens,
		MaxConcurrentRequests: s.cfg.RateLimit.MaxConcurrentRequests,
		DailyTokenQuota:       s.cfg.RateLimit.DailyTokenQuota,
	}

	// Sanitize observability config (mask exporter auth headers)
	sanitizedObs := SanitizedObservabilityConfig{
		Enabled: s.cfg.Observability.Enabled,
		Metrics: s.cfg.Observability.Metrics,
		Tracing: SanitizedTracingConfig{
			Enabled:     s.cfg.Observability.Tracing.Enabled,
			ServiceName: s.cfg.Observability.Tracing.ServiceName,
			Sampler:     s.cfg.Observability.Tracing.Sampler,
			Exporter: SanitizedExporterConfig{
				Type:     s.cfg.Observability.Tracing.Exporter.Type,
				Endpoint: s.cfg.Observability.Tracing.Exporter.Endpoint,
				Insecure: s.cfg.Observability.Tracing.Exporter.Insecure,
				Headers:  maskHeaderValues(s.cfg.Observability.Tracing.Exporter.Headers),
			},
		},
	}

	response := ConfigResponse{
		Server:    s.cfg.Server,
		Providers: sanitizedProviders,
		Models:    s.cfg.Models,
		Auth: SanitizedAuthConfig{
			Enabled:  s.cfg.Auth.Enabled,
			Issuer:   s.cfg.Auth.Issuer,
			Audience: s.cfg.Auth.Audience,
		},
		Conversations: convConfig,
		Logging:       s.cfg.Logging,
		RateLimit:     sanitizedRL,
		Observability: sanitizedObs,
	}

	writeSuccess(w, response)
}

// UIConfigResponse contains the minimal config needed by the frontend before authentication.
type UIConfigResponse struct {
	AuthEnabled bool `json:"auth_enabled"`
	OIDCEnabled bool `json:"oidc_enabled"`
}

// handleUIConfig returns the minimal configuration needed by the UI before authentication.
// This endpoint is intentionally unprotected so the frontend can determine whether auth is required.
func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	// OIDC is enabled when auth is enabled and client_id is configured
	oidcEnabled := s.cfg.Auth.Enabled && s.cfg.Auth.ClientID != ""

	writeSuccess(w, UIConfigResponse{
		AuthEnabled: s.cfg.Auth.Enabled,
		OIDCEnabled: oidcEnabled,
	})
}

// handleProviders returns the list of configured providers.
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is allowed")
		return
	}

	// Build provider info map
	providerModels := make(map[string][]string)
	models := s.registry.Models()
	for _, m := range models {
		providerModels[m.Provider] = append(providerModels[m.Provider], m.Model)
	}

	// Build provider list
	var providers []ProviderInfo
	for name, entry := range s.cfg.Providers {
		providers = append(providers, ProviderInfo{
			Name:   name,
			Type:   entry.Type,
			Models: providerModels[name],
			Status: "active",
		})
	}

	writeSuccess(w, providers)
}

type loginRequest struct {
	Token string `json:"token"`
}

// handleLogin accepts a JWT token, validates it, and sets an HttpOnly session cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Token is required")
		return
	}

	// Validate token when auth is enabled.
	if s.authMiddleware != nil {
		if _, err := s.authMiddleware.Validate(req.Token); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid or expired token")
			return
		}
	}

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    req.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})

	writeSuccess(w, map[string]string{"message": "logged in"})
}

// handleLogout clears the session cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	writeSuccess(w, map[string]string{"message": "logged out"})
}

// maskSecret masks a secret string for display.
func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 8 {
		return "********"
	}
	// Show first 4 and last 4 characters
	return secret[:4] + "..." + secret[len(secret)-4:]
}

// formatDuration formats a duration in a human-readable format.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	var parts []string
	if h > 0 {
		parts = append(parts, formatPart(int(h), "hour"))
	}
	if m > 0 {
		parts = append(parts, formatPart(int(m), "minute"))
	}
	if s > 0 || len(parts) == 0 {
		parts = append(parts, formatPart(int(s), "second"))
	}

	return strings.Join(parts, " ")
}

func formatPart(value int, unit string) string {
	if value == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", value, unit)
}

// maskURL masks any password embedded in a URL (e.g. redis://:pass@host/db).
func maskURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "****"
	}
	if u.User == nil {
		return rawURL
	}
	_, hasPassword := u.User.Password()
	if !hasPassword {
		return rawURL
	}
	u.User = url.UserPassword(u.User.Username(), "****")
	// url.String() percent-encodes the asterisks; replace them back so the
	// output stays human-readable.
	return strings.ReplaceAll(u.String(), "%2A%2A%2A%2A", "****")
}

// maskHeaderValues replaces every header value with "****" to hide auth tokens.
func maskHeaderValues(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	masked := make(map[string]string, len(headers))
	for k := range headers {
		masked[k] = "****"
	}
	return masked
}
