package ratelimit

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Config defines rate limiting configuration.
type Config struct {
	// RequestsPerSecond is the number of requests allowed per second per IP.
	RequestsPerSecond float64
	// Burst is the maximum burst size allowed.
	Burst int
	// Enabled controls whether rate limiting is active.
	Enabled bool
}

// Middleware provides per-IP rate limiting using token bucket algorithm.
type Middleware struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	config   Config
	logger   *slog.Logger
}

// New creates a new rate limiting middleware.
func New(config Config, logger *slog.Logger) *Middleware {
	m := &Middleware{
		limiters: make(map[string]*rate.Limiter),
		config:   config,
		logger:   logger,
	}

	// Start cleanup goroutine to remove old limiters
	if config.Enabled {
		go m.cleanupLimiters()
	}

	return m
}

// Handler wraps an http.Handler with rate limiting.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Extract client IP (handle X-Forwarded-For for proxies)
		ip := m.getClientIP(r)

		limiter := m.getLimiter(ip)
		if !limiter.Allow() {
			m.logger.Warn("rate limit exceeded",
				slog.String("ip", ip),
				slog.String("path", r.URL.Path),
			)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limit exceeded","message":"too many requests"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getLimiter returns the rate limiter for a given IP, creating one if needed.
func (m *Middleware) getLimiter(ip string) *rate.Limiter {
	m.mu.RLock()
	limiter, exists := m.limiters[ip]
	m.mu.RUnlock()

	if exists {
		return limiter
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	limiter, exists = m.limiters[ip]
	if exists {
		return limiter
	}

	limiter = rate.NewLimiter(rate.Limit(m.config.RequestsPerSecond), m.config.Burst)
	m.limiters[ip] = limiter
	return limiter
}

// getClientIP extracts the client IP from the request.
func (m *Middleware) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies/load balancers)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For can be a comma-separated list, use the first IP
		for idx := 0; idx < len(xff); idx++ {
			if xff[idx] == ',' {
				return xff[:idx]
			}
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// cleanupLimiters periodically removes unused limiters to prevent memory leaks.
func (m *Middleware) cleanupLimiters() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		// Clear all limiters periodically
		// In production, you might want a more sophisticated LRU cache
		m.limiters = make(map[string]*rate.Limiter)
		m.mu.Unlock()

		m.logger.Debug("cleaned up rate limiters")
	}
}
