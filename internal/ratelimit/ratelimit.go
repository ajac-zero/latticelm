package ratelimit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Config defines distributed rate limiting configuration.
type Config struct {
	Enabled               bool     `yaml:"enabled"`
	RedisURL              string   `yaml:"redis_url"`
	TrustedProxyCIDRs     []string `yaml:"trusted_proxy_cidrs"`
	RequestsPerSecond     float64  `yaml:"requests_per_second"`
	Burst                 int      `yaml:"burst"`
	MaxPromptTokens       int      `yaml:"max_prompt_tokens"`
	MaxOutputTokens       int      `yaml:"max_output_tokens"`
	MaxConcurrentRequests int      `yaml:"max_concurrent_requests"`
	DailyTokenQuota       int64    `yaml:"daily_token_quota"`
}

// RateLimitError is a structured error response for rate limit violations.
type RateLimitError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Middleware provides distributed, identity-based rate limiting.
type Middleware struct {
	backend      Backend
	config       Config
	logger       *slog.Logger
	trustedCIDRs []*net.IPNet
}

// New creates a new distributed rate limiting middleware.
func New(config Config, backend Backend, logger *slog.Logger) (*Middleware, error) {
	cidrs, err := parseCIDRs(config.TrustedProxyCIDRs)
	if err != nil {
		return nil, fmt.Errorf("parse trusted proxy CIDRs: %w", err)
	}

	if config.RequestsPerSecond == 0 {
		config.RequestsPerSecond = 10
	}
	if config.Burst == 0 {
		config.Burst = 20
	}

	return &Middleware{
		backend:      backend,
		config:       config,
		logger:       logger,
		trustedCIDRs: cidrs,
	}, nil
}

// Handler wraps an http.Handler with distributed rate limiting.
// This middleware expects to run AFTER authentication middleware so that
// JWT claims are available in the request context.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		identity := extractIdentity(r, m.trustedCIDRs)

		// Check request rate
		rateKey := fmt.Sprintf("rl:rate:%s", identity.Key())
		allowed, err := m.backend.AllowRequest(r.Context(), rateKey, m.config.RequestsPerSecond, m.config.Burst)
		if err != nil {
			m.logger.Error("rate limit backend error, failing open",
				slog.String("identity", identity.Key()),
				slog.String("error", err.Error()),
			)
		} else if !allowed {
			m.logger.Warn("rate limit exceeded",
				slog.String("identity", identity.Key()),
				slog.String("tenant", identity.TenantKey()),
				slog.String("ip", identity.IP),
				slog.String("path", r.URL.Path),
				slog.String("limit_type", "request_rate"),
			)
			writeRateLimitResponse(w, http.StatusTooManyRequests,
				"rate limit exceeded", "rate_limit_exceeded", 1)
			return
		}

		// Check concurrent requests
		if m.config.MaxConcurrentRequests > 0 {
			concKey := fmt.Sprintf("rl:conc:%s", identity.TenantKey())
			acquired, err := m.backend.AcquireConcurrent(r.Context(), concKey, m.config.MaxConcurrentRequests)
			if err != nil {
				m.logger.Error("concurrent limit backend error, failing open",
					slog.String("identity", identity.Key()),
					slog.String("error", err.Error()),
				)
			} else if !acquired {
				m.logger.Warn("concurrent request limit exceeded",
					slog.String("identity", identity.Key()),
					slog.String("tenant", identity.TenantKey()),
					slog.String("ip", identity.IP),
					slog.String("path", r.URL.Path),
					slog.String("limit_type", "concurrent_requests"),
					slog.Int("max_concurrent", m.config.MaxConcurrentRequests),
				)
				writeRateLimitResponse(w, http.StatusTooManyRequests,
					"too many concurrent requests", "concurrent_limit_exceeded", 1)
				return
			} else {
				defer func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if err := m.backend.ReleaseConcurrent(ctx, concKey); err != nil {
						m.logger.Error("failed to release concurrent slot",
							slog.String("identity", identity.Key()),
							slog.String("error", err.Error()),
						)
					}
				}()
			}
		}

		// Check daily token quota
		if m.config.DailyTokenQuota > 0 {
			quotaKey := m.dailyQuotaKey(identity.TenantKey())
			remaining, quotaOK, err := m.backend.CheckQuota(r.Context(), quotaKey, m.config.DailyTokenQuota)
			if err != nil {
				m.logger.Error("quota check backend error, failing open",
					slog.String("identity", identity.Key()),
					slog.String("error", err.Error()),
				)
			} else if !quotaOK {
				m.logger.Warn("daily token quota exceeded",
					slog.String("identity", identity.Key()),
					slog.String("tenant", identity.TenantKey()),
					slog.String("ip", identity.IP),
					slog.String("path", r.URL.Path),
					slog.String("limit_type", "daily_quota"),
					slog.Int64("daily_quota", m.config.DailyTokenQuota),
				)
				writeRateLimitResponse(w, http.StatusForbidden,
					"daily token quota exceeded", "quota_exceeded", 0)
				return
			} else {
				w.Header().Set("X-RateLimit-Remaining-Tokens", fmt.Sprintf("%d", remaining))
			}
		}

		// Put usage recorder in context for downstream handlers
		ctx := WithUsageRecorder(r.Context(), func(inputTokens, outputTokens int) {
			if m.config.DailyTokenQuota > 0 {
				totalTokens := int64(inputTokens + outputTokens)
				quotaKey := m.dailyQuotaKey(identity.TenantKey())
				recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := m.backend.RecordUsage(recordCtx, quotaKey, totalTokens); err != nil {
					m.logger.Error("failed to record token usage",
						slog.String("identity", identity.Key()),
						slog.String("error", err.Error()),
						slog.Int64("tokens", totalTokens),
					)
				}
			}
		})

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetConfig returns the rate limiter configuration (for token limit enforcement).
func (m *Middleware) GetConfig() Config {
	return m.config
}

func (m *Middleware) dailyQuotaKey(tenant string) string {
	date := time.Now().UTC().Format("2006-01-02")
	return fmt.Sprintf("rl:quota:%s:%s", tenant, date)
}

func writeRateLimitResponse(w http.ResponseWriter, status int, message, errType string, retryAfter int) {
	w.Header().Set("Content-Type", "application/json")
	if retryAfter > 0 {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
	}
	w.WriteHeader(status)
	resp := RateLimitError{
		Error:   errType,
		Message: message,
		Type:    errType,
	}
	_ = json.NewEncoder(w).Encode(resp)
}
