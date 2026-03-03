package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	registry := newMockRegistry()
	convStore := newMockConversationStore()

	server := New(registry, convStore, logger)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{
			name:           "GET returns healthy status",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST returns method not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/health", nil)
			w := httptest.NewRecorder()

			server.handleHealth(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var status HealthStatus
				if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if status.Status != "healthy" {
					t.Errorf("expected status 'healthy', got %q", status.Status)
				}

				if status.Timestamp == 0 {
					t.Error("expected non-zero timestamp")
				}
			}
		})
	}
}

func TestReadyEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name           string
		setupRegistry  func() *mockRegistry
		convStore      *mockConversationStore
		expectedStatus int
		expectedReady  bool
	}{
		{
			name: "returns ready when all checks pass",
			setupRegistry: func() *mockRegistry {
				reg := newMockRegistry()
				reg.addModel("test-model", "test-provider")
				return reg
			},
			convStore:      newMockConversationStore(),
			expectedStatus: http.StatusOK,
			expectedReady:  true,
		},
		{
			name: "returns not ready when no providers configured",
			setupRegistry: func() *mockRegistry {
				return newMockRegistry()
			},
			convStore:      newMockConversationStore(),
			expectedStatus: http.StatusServiceUnavailable,
			expectedReady:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := New(tt.setupRegistry(), tt.convStore, logger)

			req := httptest.NewRequest(http.MethodGet, "/ready", nil)
			w := httptest.NewRecorder()

			server.handleReady(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var status HealthStatus
			if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if tt.expectedReady {
				if status.Status != "ready" {
					t.Errorf("expected status 'ready', got %q", status.Status)
				}
			} else {
				if status.Status != "not_ready" {
					t.Errorf("expected status 'not_ready', got %q", status.Status)
				}
			}

			if status.Timestamp == 0 {
				t.Error("expected non-zero timestamp")
			}

			if status.Checks == nil {
				t.Error("expected checks map to be present")
			}
		})
	}
}

func TestReadyEndpointMethodNotAllowed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	registry := newMockRegistry()
	convStore := newMockConversationStore()
	server := New(registry, convStore, logger)

	req := httptest.NewRequest(http.MethodPost, "/ready", nil)
	w := httptest.NewRecorder()

	server.handleReady(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}
