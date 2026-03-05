package ratelimit

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestRateLimitMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name               string
		config             Config
		requestCount       int
		expectedAllowed    int
		expectedRateLimited int
	}{
		{
			name: "disabled rate limiting allows all requests",
			config: Config{
				Enabled:           false,
				RequestsPerSecond: 1,
				Burst:             1,
			},
			requestCount:       10,
			expectedAllowed:    10,
			expectedRateLimited: 0,
		},
		{
			name: "enabled rate limiting enforces limits",
			config: Config{
				Enabled:           true,
				RequestsPerSecond: 1,
				Burst:             2,
			},
			requestCount:       5,
			expectedAllowed:    2, // Burst allows 2 immediately
			expectedRateLimited: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := New(tt.config, logger)

			handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			allowed := 0
			rateLimited := 0

			for i := 0; i < tt.requestCount; i++ {
				req := httptest.NewRequest("GET", "/test", nil)
				req.RemoteAddr = "192.168.1.1:1234"
				w := httptest.NewRecorder()

				handler.ServeHTTP(w, req)

				if w.Code == http.StatusOK {
					allowed++
				} else if w.Code == http.StatusTooManyRequests {
					rateLimited++
				}
			}

			if allowed != tt.expectedAllowed {
				t.Errorf("expected %d allowed requests, got %d", tt.expectedAllowed, allowed)
			}
			if rateLimited != tt.expectedRateLimited {
				t.Errorf("expected %d rate limited requests, got %d", tt.expectedRateLimited, rateLimited)
			}
		})
	}
}

func TestGetClientIP(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	middleware := New(Config{Enabled: false}, logger)

	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expectedIP string
	}{
		{
			name:       "uses X-Forwarded-For if present",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.1, 198.51.100.1"},
			remoteAddr: "192.168.1.1:1234",
			expectedIP: "203.0.113.1",
		},
		{
			name:       "uses X-Real-IP if X-Forwarded-For not present",
			headers:    map[string]string{"X-Real-IP": "203.0.113.1"},
			remoteAddr: "192.168.1.1:1234",
			expectedIP: "203.0.113.1",
		},
		{
			name:       "uses RemoteAddr as fallback",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.1:1234",
			expectedIP: "192.168.1.1:1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip := middleware.getClientIP(req)
			if ip != tt.expectedIP {
				t.Errorf("expected IP %q, got %q", tt.expectedIP, ip)
			}
		})
	}
}

func TestRateLimitRefill(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	config := Config{
		Enabled:           true,
		RequestsPerSecond: 10, // 10 requests per second
		Burst:             5,
	}
	middleware := New(config, logger)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Use up the burst
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d should be allowed, got status %d", i, w.Code)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected rate limit, got status %d", w.Code)
	}

	// Wait for tokens to refill (100ms = 1 token at 10/s)
	time.Sleep(150 * time.Millisecond)

	// Should be allowed now
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("request should be allowed after refill, got status %d", w.Code)
	}
}
