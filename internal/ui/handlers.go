package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

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

// SanitizedUsageConfig exposes only the enabled flag for the usage subsystem.
type SanitizedUsageConfig struct {
	Enabled bool `json:"enabled"`
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
	Usage         SanitizedUsageConfig         `json:"usage"`
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
	Enabled   bool     `json:"enabled"`
	Issuer    string   `json:"issuer"`
	Audiences []string `json:"audiences"`
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

	convConfig := s.cfg.Conversations

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
			Enabled:   s.cfg.Auth.Enabled,
			Issuer:    s.cfg.Auth.Issuer,
			Audiences: s.cfg.Auth.Audiences,
		},
		Conversations: convConfig,
		Logging:       s.cfg.Logging,
		RateLimit:     sanitizedRL,
		Observability: sanitizedObs,
		Usage:         SanitizedUsageConfig{Enabled: s.cfg.Usage.Enabled},
	}

	writeSuccess(w, response)
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

// ProviderRequest is the request body for creating or updating a provider.
type ProviderRequest struct {
	Name string `json:"name"`
	config.ProviderEntry
}

// handleProviders handles GET (list) and POST (create) for providers.
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listProviders(w, r)
	case http.MethodPost:
		s.createProvider(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET and POST are allowed")
	}
}

// listProviders returns the list of configured providers.
func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	providerModels := make(map[string][]string)
	models := s.registry.Models()
	for _, m := range models {
		providerModels[m.Provider] = append(providerModels[m.Provider], m.Model)
	}

	var providerMap map[string]config.ProviderEntry
	if s.configStore != nil {
		var err error
		providerMap, err = s.configStore.ListProviders(r.Context())
		if err != nil {
			s.logger.Error("failed to list providers from store", "error", err)
			writeError(w, http.StatusInternalServerError, "store_error", "Failed to list providers")
			return
		}
	} else {
		providerMap = s.cfg.Providers
	}

	var providers []ProviderInfo
	for name, entry := range providerMap {
		providers = append(providers, ProviderInfo{
			Name:   name,
			Type:   entry.Type,
			Models: providerModels[name],
			Status: "active",
		})
	}

	writeSuccess(w, providers)
}

// createProvider upserts a provider via the config store.
func (s *Server) createProvider(w http.ResponseWriter, r *http.Request) {
	if s.configStore == nil {
		writeError(w, http.StatusNotImplemented, "no_config_store", "Config store not configured (DATABASE_URL and ENCRYPTION_KEY required)")
		return
	}

	var req ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON payload")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Field 'name' is required")
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "missing_type", "Field 'type' is required")
		return
	}

	if err := s.configStore.UpsertProvider(r.Context(), req.Name, req.ProviderEntry); err != nil {
		s.logger.Error("failed to upsert provider", "name", req.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "store_error", "Failed to save provider")
		return
	}

	writeJSON(w, http.StatusCreated, APIResponse{Success: true, Data: map[string]string{"name": req.Name}})
}

// handleProviderByName handles GET, PUT, and DELETE for a single provider.
func (s *Server) handleProviderByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Provider name is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		entry, ok := s.cfg.Providers[name]
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "Provider not found")
			return
		}
		writeSuccess(w, SanitizedProvider{
			Type:       entry.Type,
			APIKey:     maskSecret(entry.APIKey),
			Endpoint:   entry.Endpoint,
			APIVersion: entry.APIVersion,
			Project:    entry.Project,
			Location:   entry.Location,
		})

	case http.MethodPut:
		if s.configStore == nil {
			writeError(w, http.StatusNotImplemented, "no_config_store", "Config store not configured (DATABASE_URL and ENCRYPTION_KEY required)")
			return
		}
		var entry config.ProviderEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON payload")
			return
		}
		if entry.Type == "" {
			writeError(w, http.StatusBadRequest, "missing_type", "Field 'type' is required")
			return
		}
		if err := s.configStore.UpsertProvider(r.Context(), name, entry); err != nil {
			s.logger.Error("failed to update provider", "name", name, "error", err)
			writeError(w, http.StatusInternalServerError, "store_error", "Failed to update provider")
			return
		}
		writeSuccess(w, map[string]string{"name": name})

	case http.MethodDelete:
		if s.configStore == nil {
			writeError(w, http.StatusNotImplemented, "no_config_store", "Config store not configured (DATABASE_URL and ENCRYPTION_KEY required)")
			return
		}
		if err := s.configStore.DeleteProvider(r.Context(), name); err != nil {
			s.logger.Error("failed to delete provider", "name", name, "error", err)
			writeError(w, http.StatusInternalServerError, "store_error", "Failed to delete provider")
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET, PUT, and DELETE are allowed")
	}
}

// handleConfigModels handles GET (list) and POST (create) for config models.
func (s *Server) handleConfigModels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.configStore != nil {
			models, err := s.configStore.ListModels(r.Context())
			if err != nil {
				s.logger.Error("failed to list models from store", "error", err)
				writeError(w, http.StatusInternalServerError, "store_error", "Failed to list models")
				return
			}
			writeSuccess(w, models)
			return
		}
		writeSuccess(w, s.cfg.Models)
	case http.MethodPost:
		if s.configStore == nil {
			writeError(w, http.StatusNotImplemented, "no_config_store", "Config store not configured (DATABASE_URL and ENCRYPTION_KEY required)")
			return
		}
		var m config.ModelEntry
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON payload")
			return
		}
		if m.Name == "" {
			writeError(w, http.StatusBadRequest, "missing_name", "Field 'name' is required")
			return
		}
		if m.Provider == "" {
			writeError(w, http.StatusBadRequest, "missing_provider", "Field 'provider' is required")
			return
		}
		if err := s.configStore.UpsertModel(r.Context(), m); err != nil {
			s.logger.Error("failed to upsert model", "name", m.Name, "error", err)
			writeError(w, http.StatusInternalServerError, "store_error", "Failed to save model")
			return
		}
		writeJSON(w, http.StatusCreated, APIResponse{Success: true, Data: m})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET and POST are allowed")
	}
}

// handleConfigModelByName handles GET, PUT, and DELETE for a single config model.
func (s *Server) handleConfigModelByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Model name is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		for _, m := range s.cfg.Models {
			if m.Name == name {
				writeSuccess(w, m)
				return
			}
		}
		writeError(w, http.StatusNotFound, "not_found", "Model not found")

	case http.MethodPut:
		if s.configStore == nil {
			writeError(w, http.StatusNotImplemented, "no_config_store", "Config store not configured (DATABASE_URL and ENCRYPTION_KEY required)")
			return
		}
		var m config.ModelEntry
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON payload")
			return
		}
		m.Name = name
		if m.Provider == "" {
			writeError(w, http.StatusBadRequest, "missing_provider", "Field 'provider' is required")
			return
		}
		if err := s.configStore.UpsertModel(r.Context(), m); err != nil {
			s.logger.Error("failed to update model", "name", name, "error", err)
			writeError(w, http.StatusInternalServerError, "store_error", "Failed to update model")
			return
		}
		writeSuccess(w, m)

	case http.MethodDelete:
		if s.configStore == nil {
			writeError(w, http.StatusNotImplemented, "no_config_store", "Config store not configured (DATABASE_URL and ENCRYPTION_KEY required)")
			return
		}
		if err := s.configStore.DeleteModel(r.Context(), name); err != nil {
			s.logger.Error("failed to delete model", "name", name, "error", err)
			writeError(w, http.StatusInternalServerError, "store_error", "Failed to delete model")
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET, PUT, and DELETE are allowed")
	}
}
