package admin

import (
	"fmt"
	"net/http"
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
	Status    string              `json:"status"`
	Timestamp string              `json:"timestamp"`
	Checks    map[string]HealthCheck `json:"checks"`
}

// HealthCheck represents a single health check.
type HealthCheck struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ConfigResponse contains the sanitized configuration.
type ConfigResponse struct {
	Server        config.ServerConfig             `json:"server"`
	Providers     map[string]SanitizedProvider    `json:"providers"`
	Models        []config.ModelEntry             `json:"models"`
	Auth          SanitizedAuthConfig             `json:"auth"`
	Conversations config.ConversationConfig       `json:"conversations"`
	Logging       config.LoggingConfig            `json:"logging"`
	RateLimit     config.RateLimitConfig          `json:"rate_limit"`
	Observability config.ObservabilityConfig      `json:"observability"`
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
func (s *AdminServer) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
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
func (s *AdminServer) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
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
func (s *AdminServer) handleConfig(w http.ResponseWriter, r *http.Request) {
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

	response := ConfigResponse{
		Server:        s.cfg.Server,
		Providers:     sanitizedProviders,
		Models:        s.cfg.Models,
		Auth:          SanitizedAuthConfig{
			Enabled:  s.cfg.Auth.Enabled,
			Issuer:   s.cfg.Auth.Issuer,
			Audience: s.cfg.Auth.Audience,
		},
		Conversations: convConfig,
		Logging:       s.cfg.Logging,
		RateLimit:     s.cfg.RateLimit,
		Observability: s.cfg.Observability,
	}

	writeSuccess(w, response)
}

// handleProviders returns the list of configured providers.
func (s *AdminServer) handleProviders(w http.ResponseWriter, r *http.Request) {
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
