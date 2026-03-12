package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/system/info", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
	assert.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))
	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
}

func TestSecurityHeadersMiddlewareOnErrorResponse(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
}

func TestIPAllowlistMiddleware_EmptyAllowlist(t *testing.T) {
	// Empty allowlist should allow all IPs
	handler := IPAllowlistMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestIPAllowlistMiddleware_AllowedIP(t *testing.T) {
	allowlist, err := ParseCIDRs([]string{"10.0.0.0/8", "192.168.1.0/24"})
	require.NoError(t, err)

	handler := IPAllowlistMiddleware(allowlist)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		remoteAddr string
		wantStatus int
	}{
		{"10.0.0.1:1234", http.StatusOK},
		{"10.255.255.255:9000", http.StatusOK},
		{"192.168.1.50:443", http.StatusOK},
		{"192.168.2.1:8080", http.StatusForbidden},
		{"8.8.8.8:443", http.StatusForbidden},
		{"172.16.0.1:80", http.StatusForbidden},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
		req.RemoteAddr = tt.remoteAddr
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, tt.wantStatus, rec.Code, "remoteAddr=%s", tt.remoteAddr)
	}
}

func TestIPAllowlistMiddleware_IPv6(t *testing.T) {
	allowlist, err := ParseCIDRs([]string{"::1/128"})
	require.NoError(t, err)

	handler := IPAllowlistMiddleware(allowlist)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req.RemoteAddr = "[::1]:5000"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	req2 := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req2.RemoteAddr = "[::2]:5000"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusForbidden, rec2.Code)
}

func TestParseCIDRs(t *testing.T) {
	t.Run("valid CIDRs", func(t *testing.T) {
		nets, err := ParseCIDRs([]string{"10.0.0.0/8", "192.168.0.0/16", "::1/128"})
		require.NoError(t, err)
		assert.Len(t, nets, 3)
	})

	t.Run("empty list", func(t *testing.T) {
		nets, err := ParseCIDRs(nil)
		require.NoError(t, err)
		assert.Empty(t, nets)
	})

	t.Run("invalid CIDR returns error", func(t *testing.T) {
		_, err := ParseCIDRs([]string{"not-a-cidr"})
		assert.Error(t, err)
	})

	t.Run("single IP without mask returns error", func(t *testing.T) {
		_, err := ParseCIDRs([]string{"10.0.0.1"})
		assert.Error(t, err)
	})
}
