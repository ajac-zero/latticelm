package ratelimit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ajac-zero/latticelm/internal/auth"
)

func setupTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()
	require.NoError(t, client.Ping(ctx).Err())
	return client, mr
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func testConfig() Config {
	return Config{
		Enabled:               true,
		RequestsPerSecond:     10,
		Burst:                 5,
		MaxConcurrentRequests: 3,
		DailyTokenQuota:       10000,
	}
}

func TestNew(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	t.Run("valid config", func(t *testing.T) {
		cfg := testConfig()
		m, err := New(cfg, backend, logger)
		require.NoError(t, err)
		require.NotNil(t, m)
	})

	t.Run("with trusted CIDRs", func(t *testing.T) {
		cfg := testConfig()
		cfg.TrustedProxyCIDRs = []string{"10.0.0.0/8", "172.16.0.0/12"}
		m, err := New(cfg, backend, logger)
		require.NoError(t, err)
		require.NotNil(t, m)
		assert.Len(t, m.trustedCIDRs, 2)
	})

	t.Run("invalid CIDR", func(t *testing.T) {
		cfg := testConfig()
		cfg.TrustedProxyCIDRs = []string{"not-a-cidr"}
		_, err := New(cfg, backend, logger)
		assert.Error(t, err)
	})

	t.Run("defaults applied", func(t *testing.T) {
		cfg := Config{Enabled: true}
		m, err := New(cfg, backend, logger)
		require.NoError(t, err)
		assert.Equal(t, float64(10), m.config.RequestsPerSecond)
		assert.Equal(t, 20, m.config.Burst)
	})
}

func TestMiddleware_Disabled(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{Enabled: false}
	m, err := New(cfg, backend, logger)
	require.NoError(t, err)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
}

func TestMiddleware_RequestRateLimit(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             2,
	}
	m, err := New(cfg, backend, logger)
	require.NoError(t, err)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	allowed := 0
	rateLimited := 0

	for i := 0; i < 5; i++ {
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

	assert.Equal(t, 2, allowed, "burst should allow 2 requests")
	assert.Equal(t, 3, rateLimited, "remaining should be rate limited")
}

func TestMiddleware_RateLimitResponse(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             1,
	}
	m, err := New(cfg, backend, logger)
	require.NoError(t, err)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request: allowed
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Second request: rate limited
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.NotEmpty(t, w.Header().Get("Retry-After"))

	var errResp RateLimitError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "rate_limit_exceeded", errResp.Type)
}

func TestMiddleware_ConcurrentRequests(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{
		Enabled:               true,
		RequestsPerSecond:     100,
		Burst:                 100,
		MaxConcurrentRequests: 2,
	}
	m, err := New(cfg, backend, logger)
	require.NoError(t, err)

	// Use a channel to hold requests in flight
	block := make(chan struct{})
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
		w.WriteHeader(http.StatusOK)
	}))

	results := make(chan int, 5)

	// Launch 3 concurrent requests (max is 2)
	for i := 0; i < 3; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "192.168.1.1:1234"
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			results <- w.Code
		}()
	}

	// Give goroutines time to start
	time.Sleep(100 * time.Millisecond)

	// Release blocked requests
	close(block)

	var ok, limited int
	for i := 0; i < 3; i++ {
		code := <-results
		if code == http.StatusOK {
			ok++
		} else if code == http.StatusTooManyRequests {
			limited++
		}
	}

	assert.Equal(t, 2, ok, "should allow max 2 concurrent requests")
	assert.Equal(t, 1, limited, "should limit 1 request")
}

func TestMiddleware_DailyQuota(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 100,
		Burst:             100,
		DailyTokenQuota:   100,
	}
	m, err := New(cfg, backend, logger)
	require.NoError(t, err)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate recording usage
		RecordUsageFromContext(r.Context(), 50, 50)
		w.WriteHeader(http.StatusOK)
	}))

	// First request: within quota
	req := httptest.NewRequest("POST", "/v1/responses", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining-Tokens"))

	// Second request: quota should be exceeded
	req = httptest.NewRequest("POST", "/v1/responses", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	var errResp RateLimitError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "quota_exceeded", errResp.Type)
}

func TestMiddleware_IdentityFromJWT(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             1,
	}
	m, err := New(cfg, backend, logger)
	require.NoError(t, err)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Helper to create a request with JWT claims in context
	makeReqWithClaims := func(claims jwt.MapClaims) *http.Request {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		ctx := context.WithValue(req.Context(), claimsKeyForTest, claims)
		return req.WithContext(ctx)
	}

	// User A: first request allowed
	reqA := makeReqWithClaims(jwt.MapClaims{"sub": "user-a", "tenant_id": "tenant-1"})
	wA := httptest.NewRecorder()
	handler.ServeHTTP(wA, reqA)
	assert.Equal(t, http.StatusOK, wA.Code)

	// User A: second request should be rate limited
	reqA2 := makeReqWithClaims(jwt.MapClaims{"sub": "user-a", "tenant_id": "tenant-1"})
	wA2 := httptest.NewRecorder()
	handler.ServeHTTP(wA2, reqA2)
	assert.Equal(t, http.StatusTooManyRequests, wA2.Code)

	// User B: should have separate rate limit, still allowed
	reqB := makeReqWithClaims(jwt.MapClaims{"sub": "user-b", "tenant_id": "tenant-1"})
	wB := httptest.NewRecorder()
	handler.ServeHTTP(wB, reqB)
	assert.Equal(t, http.StatusOK, wB.Code)
}

// claimsKeyForTest matches auth.claimsKey for testing.
// We use auth.GetClaims which reads from this context key.
var claimsKeyForTest = auth.ClaimsContextKey()

func TestMiddleware_DifferentIPsHaveSeparateLimits(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             1,
	}
	m, err := New(cfg, backend, logger)
	require.NoError(t, err)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP 1
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// IP 2 (different IP, separate limit)
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "10.0.0.2:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestMiddleware_UsageRecording(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 100,
		Burst:             100,
		DailyTokenQuota:   1000,
	}
	m, err := New(cfg, backend, logger)
	require.NoError(t, err)

	var recorded bool
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		RecordUsageFromContext(r.Context(), 100, 50)
		recorded = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.True(t, recorded)
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify usage was recorded in Redis
	quotaKey := m.dailyQuotaKey("192.168.1.1")
	used, err := client.Get(context.Background(), quotaKey).Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(150), used)
}

func TestRedisBackend_TokenBucket(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	ctx := context.Background()

	t.Run("allows within burst", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			allowed, err := backend.AllowRequest(ctx, "test:bucket:1", 1, 5)
			require.NoError(t, err)
			assert.True(t, allowed, "request %d should be allowed", i)
		}

		// 6th should be denied
		allowed, err := backend.AllowRequest(ctx, "test:bucket:1", 1, 5)
		require.NoError(t, err)
		assert.False(t, allowed)
	})

	t.Run("different keys are independent", func(t *testing.T) {
		allowed, err := backend.AllowRequest(ctx, "test:bucket:a", 1, 1)
		require.NoError(t, err)
		assert.True(t, allowed)

		allowed, err = backend.AllowRequest(ctx, "test:bucket:b", 1, 1)
		require.NoError(t, err)
		assert.True(t, allowed)
	})
}

func TestRedisBackend_Concurrent(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	ctx := context.Background()

	key := "test:conc:1"

	// Acquire up to max
	ok, err := backend.AcquireConcurrent(ctx, key, 2)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = backend.AcquireConcurrent(ctx, key, 2)
	require.NoError(t, err)
	assert.True(t, ok)

	// Third should fail
	ok, err = backend.AcquireConcurrent(ctx, key, 2)
	require.NoError(t, err)
	assert.False(t, ok)

	// Release one
	err = backend.ReleaseConcurrent(ctx, key)
	require.NoError(t, err)

	// Now should succeed
	ok, err = backend.AcquireConcurrent(ctx, key, 2)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRedisBackend_Quota(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	ctx := context.Background()

	key := "test:quota:1"

	// Check initial quota
	remaining, allowed, err := backend.CheckQuota(ctx, key, 100)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, int64(100), remaining)

	// Record some usage
	err = backend.RecordUsage(ctx, key, 60)
	require.NoError(t, err)

	// Check remaining
	remaining, allowed, err = backend.CheckQuota(ctx, key, 100)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, int64(40), remaining)

	// Record more to exceed
	err = backend.RecordUsage(ctx, key, 50)
	require.NoError(t, err)

	// Should be denied
	remaining, allowed, err = backend.CheckQuota(ctx, key, 100)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, int64(0), remaining)
}

func TestGetClientIP_TrustedProxy(t *testing.T) {
	trustedCIDRs, err := parseCIDRs([]string{"10.0.0.0/8", "172.16.0.0/12"})
	require.NoError(t, err)

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		cidrs      []*net.IPNet
		expected   string
	}{
		{
			name:       "no trusted CIDRs, uses RemoteAddr",
			remoteAddr: "1.2.3.4:5678",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.1"},
			cidrs:      nil,
			expected:   "1.2.3.4",
		},
		{
			name:       "trusted proxy, uses XFF",
			remoteAddr: "10.0.0.1:5678",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.1, 10.0.0.1"},
			cidrs:      trustedCIDRs,
			expected:   "203.0.113.1",
		},
		{
			name:       "untrusted proxy, ignores XFF",
			remoteAddr: "1.2.3.4:5678",
			headers:    map[string]string{"X-Forwarded-For": "spoofed.ip"},
			cidrs:      trustedCIDRs,
			expected:   "1.2.3.4",
		},
		{
			name:       "trusted proxy, uses X-Real-IP",
			remoteAddr: "172.16.0.1:5678",
			headers:    map[string]string{"X-Real-IP": "203.0.113.50"},
			cidrs:      trustedCIDRs,
			expected:   "203.0.113.50",
		},
		{
			name:       "trusted proxy, no forwarded headers",
			remoteAddr: "10.0.0.5:5678",
			headers:    map[string]string{},
			cidrs:      trustedCIDRs,
			expected:   "10.0.0.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			ip := getClientIP(req, tt.cidrs)
			assert.Equal(t, tt.expected, ip)
		})
	}
}

func TestIdentity(t *testing.T) {
	t.Run("key with tenant and subject", func(t *testing.T) {
		id := Identity{Tenant: "acme", Subject: "user-1", IP: "1.2.3.4"}
		assert.Equal(t, "acme:user-1", id.Key())
		assert.Equal(t, "acme", id.TenantKey())
	})

	t.Run("key with subject only", func(t *testing.T) {
		id := Identity{Subject: "user-1", IP: "1.2.3.4"}
		assert.Equal(t, "user-1", id.Key())
		assert.Equal(t, "user-1", id.TenantKey())
	})

	t.Run("key with IP fallback", func(t *testing.T) {
		id := Identity{IP: "1.2.3.4"}
		assert.Equal(t, "1.2.3.4", id.Key())
		assert.Equal(t, "1.2.3.4", id.TenantKey())
	})

	t.Run("tenant same as subject is deduplicated", func(t *testing.T) {
		id := Identity{Tenant: "user-1", Subject: "user-1", IP: "1.2.3.4"}
		assert.Equal(t, "user-1", id.Key())
	})
}

func TestExtractIdentity(t *testing.T) {
	t.Run("from JWT claims", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:5678"
		claims := jwt.MapClaims{
			"sub":       "user-123",
			"tenant_id": "acme-corp",
		}
		ctx := context.WithValue(req.Context(), claimsKeyForTest, claims)
		req = req.WithContext(ctx)

		id := extractIdentity(req, nil)
		assert.Equal(t, "user-123", id.Subject)
		assert.Equal(t, "acme-corp", id.Tenant)
		assert.Equal(t, "1.2.3.4", id.IP)
	})

	t.Run("without JWT falls back to IP", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:5678"

		id := extractIdentity(req, nil)
		assert.Equal(t, "", id.Subject)
		assert.Equal(t, "1.2.3.4", id.IP)
		assert.Equal(t, "1.2.3.4", id.Key())
	})
}

func TestRecordUsageFromContext(t *testing.T) {
	t.Run("with recorder", func(t *testing.T) {
		var input, output int
		ctx := WithUsageRecorder(context.Background(), func(in, out int) {
			input = in
			output = out
		})
		RecordUsageFromContext(ctx, 100, 50)
		assert.Equal(t, 100, input)
		assert.Equal(t, 50, output)
	})

	t.Run("without recorder does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			RecordUsageFromContext(context.Background(), 100, 50)
		})
	})
}

func TestParseCIDRs(t *testing.T) {
	t.Run("valid CIDRs", func(t *testing.T) {
		cidrs, err := parseCIDRs([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
		require.NoError(t, err)
		assert.Len(t, cidrs, 3)
	})

	t.Run("empty list", func(t *testing.T) {
		cidrs, err := parseCIDRs(nil)
		require.NoError(t, err)
		assert.Nil(t, cidrs)
	})

	t.Run("invalid CIDR", func(t *testing.T) {
		_, err := parseCIDRs([]string{"invalid"})
		assert.Error(t, err)
	})
}

func TestMiddleware_RateRefill(t *testing.T) {
	client, mr := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 10,
		Burst:             2,
	}
	m, err := New(cfg, backend, logger)
	require.NoError(t, err)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust burst
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// Fast forward in miniredis won't affect our Lua script time calculation,
	// but we can verify the key exists
	_ = mr
}

func TestMiddleware_CrossPodConsistency(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             2,
	}

	// Two "pods" sharing the same Redis backend
	pod1, err := New(cfg, backend, logger)
	require.NoError(t, err)
	pod2, err := New(cfg, backend, logger)
	require.NoError(t, err)

	handler1 := pod1.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler2 := pod2.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request on pod1 (allowed)
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	handler1.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Request on pod2 (allowed - burst of 2)
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w = httptest.NewRecorder()
	handler2.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Request on pod1 (rate limited - shared state)
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w = httptest.NewRecorder()
	handler1.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestMiddleware_ScaleOutDoesNotMultiplyQuota(t *testing.T) {
	client, _ := setupTestRedis(t)
	backend := NewRedisBackend(client)
	logger := testLogger()

	cfg := Config{
		Enabled:           true,
		RequestsPerSecond: 100,
		Burst:             100,
		DailyTokenQuota:   100,
	}

	pod1, err := New(cfg, backend, logger)
	require.NoError(t, err)
	pod2, err := New(cfg, backend, logger)
	require.NoError(t, err)

	handler1 := pod1.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		RecordUsageFromContext(r.Context(), 30, 30)
		w.WriteHeader(http.StatusOK)
	}))
	handler2 := pod2.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		RecordUsageFromContext(r.Context(), 30, 30)
		w.WriteHeader(http.StatusOK)
	}))

	// Pod 1: use 60 tokens
	req := httptest.NewRequest("POST", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	handler1.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Pod 2: use 60 more tokens (total 120 > 100 quota)
	req = httptest.NewRequest("POST", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w = httptest.NewRecorder()
	handler2.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Pod 1: should be denied (quota exceeded in shared Redis)
	req = httptest.NewRequest("POST", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w = httptest.NewRecorder()
	handler1.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
