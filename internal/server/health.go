package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// HealthStatus represents the health check response.
type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp int64             `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// handleHealth returns a basic health check endpoint.
// This is suitable for Kubernetes liveness probes.
func (s *GatewayServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to encode health response", "error", err.Error())
	}
}

// handleReady returns a readiness check that verifies dependencies.
// This is suitable for Kubernetes readiness probes and load balancer health checks.
func (s *GatewayServer) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	checks := make(map[string]string)
	allHealthy := true

	// Check conversation store connectivity
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Test conversation store by attempting a simple operation
	testID := "health_check_test"
	_, err := s.convs.Get(ctx, testID)
	if err != nil {
		checks["conversation_store"] = "unhealthy: " + err.Error()
		allHealthy = false
	} else {
		checks["conversation_store"] = "healthy"
	}

	// Check if at least one provider is configured
	models := s.registry.Models()
	if len(models) == 0 {
		checks["providers"] = "unhealthy: no providers configured"
		allHealthy = false
	} else {
		checks["providers"] = "healthy"
	}

	_ = ctx // Use context if needed

	status := HealthStatus{
		Timestamp: time.Now().Unix(),
		Checks:    checks,
	}

	if allHealthy {
		status.Status = "ready"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	} else {
		status.Status = "not_ready"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	if err := json.NewEncoder(w).Encode(status); err != nil {
		s.logger.ErrorContext(r.Context(), "failed to encode ready response", "error", err.Error())
	}
}
