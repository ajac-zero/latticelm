package auth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test fixtures
var (
	testPrivateKey   *rsa.PrivateKey
	testPublicKey    *rsa.PublicKey
	testECPrivateKey *ecdsa.PrivateKey
	testECPublicKey  *ecdsa.PublicKey
	testKID          = "test-key-id-1"
	testECKID        = "test-ec-key-id-1"
	testAudience     = "test-client-id"
)

func init() {
	var err error
	testPrivateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("failed to generate RSA test key: %v", err))
	}
	testPublicKey = &testPrivateKey.PublicKey

	testECPrivateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(fmt.Sprintf("failed to generate ECDSA test key: %v", err))
	}
	testECPublicKey = &testECPrivateKey.PublicKey
}

// mockJWKSServer provides a mock OIDC/JWKS server for testing
type mockJWKSServer struct {
	server       *httptest.Server
	jwksResponse []byte
	mu           sync.Mutex
	requestCount int
	failNext     bool
}

func newMockJWKSServer(publicKey *rsa.PublicKey, kid string) *mockJWKSServer {
	m := &mockJWKSServer{}

	nBytes := publicKey.N.Bytes()
	eBytes := big.NewInt(int64(publicKey.E)).Bytes()
	n := base64.RawURLEncoding.EncodeToString(nBytes)
	e := base64.RawURLEncoding.EncodeToString(eBytes)

	jwks := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kid": kid,
				"kty": "RSA",
				"use": "sig",
				"n":   n,
				"e":   e,
			},
		},
	}
	m.jwksResponse, _ = json.Marshal(jwks)

	mux := http.NewServeMux()

	// OIDC discovery endpoint
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.requestCount++
		failNext := m.failNext
		if m.failNext {
			m.failNext = false
		}
		m.mu.Unlock()

		if failNext {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}

		oidcConfig := map[string]string{
			"jwks_uri": m.server.URL + "/jwks",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(oidcConfig)
	})

	// JWKS endpoint
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.requestCount++
		failNext := m.failNext
		if m.failNext {
			m.failNext = false
		}
		m.mu.Unlock()

		if failNext {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(m.jwksResponse)
	})

	m.server = httptest.NewServer(mux)
	return m
}

func (m *mockJWKSServer) close() {
	m.server.Close()
}

func (m *mockJWKSServer) setFailNext() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failNext = true
}

func (m *mockJWKSServer) updateJWKS(newResponse []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jwksResponse = newResponse
}

// generateTestJWT creates an RS256-signed JWT with the given claims.
func generateTestJWT(privateKey *rsa.PrivateKey, claims jwt.MapClaims, kid string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	return token.SignedString(privateKey)
}

// generateECTestJWT creates an ES256-signed JWT with the given claims.
func generateECTestJWT(privateKey *ecdsa.PrivateKey, claims jwt.MapClaims, kid string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = kid
	return token.SignedString(privateKey)
}

// newMockJWKSServerWithEC creates a mock OIDC/JWKS server serving a single EC P-256 key.
func newMockJWKSServerWithEC(publicKey *ecdsa.PublicKey, kid string) *mockJWKSServer {
	m := &mockJWKSServer{}

	x := base64.RawURLEncoding.EncodeToString(publicKey.X.Bytes())
	y := base64.RawURLEncoding.EncodeToString(publicKey.Y.Bytes())

	jwks := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kid": kid,
				"kty": "EC",
				"use": "sig",
				"alg": "ES256",
				"crv": "P-256",
				"x":   x,
				"y":   y,
			},
		},
	}
	m.jwksResponse, _ = json.Marshal(jwks)

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.requestCount++
		m.mu.Unlock()
		oidcConfig := map[string]string{"jwks_uri": m.server.URL + "/jwks"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(oidcConfig)
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.requestCount++
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(m.jwksResponse)
	})

	m.server = httptest.NewServer(mux)
	return m
}

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		setupServer func() *mockJWKSServer
		expectError bool
		validate    func(t *testing.T, m *Middleware)
	}{
		{
			name: "disabled auth returns empty middleware",
			config: Config{
				Enabled: false,
			},
			expectError: false,
			validate: func(t *testing.T, m *Middleware) {
				assert.False(t, m.cfg.Enabled)
				assert.Nil(t, m.keys)
				assert.Nil(t, m.client)
			},
		},
		{
			name: "enabled without issuer returns error",
			config: Config{
				Enabled: true,
				Issuer:  "",
			},
			expectError: true,
		},
		{
			name: "enabled with valid config fetches JWKS",
			setupServer: func() *mockJWKSServer {
				return newMockJWKSServer(testPublicKey, testKID)
			},
			expectError: false,
			validate: func(t *testing.T, m *Middleware) {
				assert.True(t, m.cfg.Enabled)
				assert.NotNil(t, m.keys)
				assert.NotNil(t, m.client)
				assert.Len(t, m.keys, 1)
				assert.Contains(t, m.keys, testKID)
			},
		},
		{
			name: "JWKS fetch failure returns error",
			setupServer: func() *mockJWKSServer {
				server := newMockJWKSServer(testPublicKey, testKID)
				server.setFailNext()
				return server
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *mockJWKSServer
			if tt.setupServer != nil {
				server = tt.setupServer()
				defer server.close()
				tt.config = Config{
					Enabled:  true,
					Issuer:   server.server.URL,
					Audience: testAudience,
				}
			}

			m, err := New(tt.config, slog.Default())

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, m)

			if tt.validate != nil {
				tt.validate(t, m)
			}
		})
	}
}

func TestMiddleware_Handler(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	// Create a test handler that echoes back claims
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r.Context())
		if ok {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf("sub:%s", claims["sub"])))
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("no-claims"))
		}
	})

	handler := m.Handler(testHandler)

	tests := []struct {
		name           string
		setupRequest   func() *http.Request
		expectStatus   int
		expectBody     string
		validateClaims bool
	}{
		{
			name: "missing authorization header",
			setupRequest: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/test", nil)
			},
			expectStatus: http.StatusUnauthorized,
			expectBody:   "Unauthorized",
		},
		{
			name: "malformed authorization header - no bearer",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "invalid-token")
				return req
			},
			expectStatus: http.StatusUnauthorized,
			expectBody:   "Unauthorized",
		},
		{
			name: "malformed authorization header - wrong scheme",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Basic dGVzdDp0ZXN0")
				return req
			},
			expectStatus: http.StatusUnauthorized,
			expectBody:   "Unauthorized",
		},
		{
			name: "valid token with correct claims",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": testAudience,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(testPrivateKey, claims, testKID)
				require.NoError(t, err)

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			expectStatus:   http.StatusOK,
			expectBody:     "sub:user123",
			validateClaims: true,
		},
		{
			name: "expired token",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": testAudience,
					"exp": time.Now().Add(-time.Hour).Unix(),
					"iat": time.Now().Add(-2 * time.Hour).Unix(),
				}
				token, err := generateTestJWT(testPrivateKey, claims, testKID)
				require.NoError(t, err)

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			expectStatus: http.StatusUnauthorized,
			expectBody:   "Unauthorized",
		},
		{
			name: "token with wrong issuer",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": "https://wrong-issuer.example.com",
					"aud": testAudience,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(testPrivateKey, claims, testKID)
				require.NoError(t, err)

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			expectStatus: http.StatusUnauthorized,
			expectBody:   "Unauthorized",
		},
		{
			name: "token with wrong audience",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": "wrong-audience",
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(testPrivateKey, claims, testKID)
				require.NoError(t, err)

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			expectStatus: http.StatusUnauthorized,
			expectBody:   "Unauthorized",
		},
		{
			name: "token with missing kid",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": testAudience,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
				// Don't set kid header
				tokenString, err := token.SignedString(testPrivateKey)
				require.NoError(t, err)

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Bearer "+tokenString)
				return req
			},
			expectStatus: http.StatusUnauthorized,
			expectBody:   "Unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code)
			if tt.expectBody != "" {
				assert.Contains(t, rec.Body.String(), tt.expectBody)
			}
		})
	}
}

func TestMiddleware_Handler_SanitizedErrors(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}

	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	m, err := New(cfg, log)
	require.NoError(t, err)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := m.Handler(testHandler)

	tests := []struct {
		name               string
		setupRequest       func() *http.Request
		leakPatterns       []string // strings that must NOT appear in response body
		expectLogSubstring string   // string that MUST appear in server logs
	}{
		{
			name: "expired token does not leak expiry details",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": testAudience,
					"exp": time.Now().Add(-time.Hour).Unix(),
					"iat": time.Now().Add(-2 * time.Hour).Unix(),
				}
				token, err := generateTestJWT(testPrivateKey, claims, testKID)
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			leakPatterns:       []string{"expired", "token is expired", "exp"},
			expectLogSubstring: "token validation failed",
		},
		{
			name: "unknown key ID does not leak key ID details",
			setupRequest: func() *http.Request {
				unknownKey, err := rsa.GenerateKey(rand.Reader, 2048)
				require.NoError(t, err)
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": testAudience,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(unknownKey, claims, "unknown-kid-xyz")
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			leakPatterns:       []string{"unknown-kid-xyz", "unknown key ID"},
			expectLogSubstring: "token validation failed",
		},
		{
			name: "wrong issuer does not leak issuer details",
			setupRequest: func() *http.Request {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": "https://attacker.example.com",
					"aud": testAudience,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(testPrivateKey, claims, testKID)
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			leakPatterns:       []string{"attacker.example.com", "invalid issuer"},
			expectLogSubstring: "token validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logBuf.Reset()
			req := tt.setupRequest()
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			responseBody := rec.Body.String()
			assert.Equal(t, `{"error":{"message":"Unauthorized"}}`, responseBody)

			// Verify internal details are NOT in the response
			for _, pattern := range tt.leakPatterns {
				assert.NotContains(t, responseBody, pattern, "response must not leak: %s", pattern)
			}

			// Verify detailed error IS logged server-side
			if tt.expectLogSubstring != "" {
				assert.Contains(t, logBuf.String(), tt.expectLogSubstring)
			}
		})
	}
}

func TestMiddleware_Handler_DisabledAuth(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	handler := m.Handler(testHandler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "success", rec.Body.String())
}

func TestMiddleware_Handler_SessionCookie(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetClaims(r.Context())
		if ok {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf("sub:%s", claims["sub"])))
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("no-claims"))
		}
	})

	handler := m.Handler(testHandler)

	validClaims := jwt.MapClaims{
		"sub": "cookie-user",
		"iss": server.server.URL,
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	validToken, err := generateTestJWT(testPrivateKey, validClaims, testKID)
	require.NoError(t, err)

	tests := []struct {
		name         string
		setupRequest func() *http.Request
		expectStatus int
		expectBody   string
	}{
		{
			name: "valid session cookie authenticates",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: validToken})
				return req
			},
			expectStatus: http.StatusOK,
			expectBody:   "sub:cookie-user",
		},
		{
			name: "missing cookie and header returns unauthorized",
			setupRequest: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/test", nil)
			},
			expectStatus: http.StatusUnauthorized,
			expectBody:   "Unauthorized",
		},
		{
			name: "bearer header takes precedence over cookie",
			setupRequest: func() *http.Request {
				bearerClaims := jwt.MapClaims{
					"sub": "bearer-user",
					"iss": server.server.URL,
					"aud": testAudience,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				bearerToken, err := generateTestJWT(testPrivateKey, bearerClaims, testKID)
				require.NoError(t, err)

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Bearer "+bearerToken)
				req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: validToken})
				return req
			},
			expectStatus: http.StatusOK,
			expectBody:   "sub:bearer-user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.expectStatus, rec.Code)
			if tt.expectBody != "" {
				assert.Contains(t, rec.Body.String(), tt.expectBody)
			}
		})
	}
}

func TestAdminMiddleware_Handler(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	tests := []struct {
		name         string
		cfg          AdminConfig
		claims       jwt.MapClaims
		expectStatus int
	}{
		{
			name: "disabled middleware allows request",
			cfg: AdminConfig{
				Enabled: false,
			},
			expectStatus: http.StatusOK,
		},
		{
			name: "default role claim allows admin",
			cfg: AdminConfig{
				Enabled: true,
			},
			claims: jwt.MapClaims{
				"role": "admin",
			},
			expectStatus: http.StatusOK,
		},
		{
			name: "groups claim allows configured admin value",
			cfg: AdminConfig{
				Enabled:       true,
				AllowedValues: []string{"platform-admin"},
			},
			claims: jwt.MapClaims{
				"groups": []interface{}{"engineering", "platform-admin"},
			},
			expectStatus: http.StatusOK,
		},
		{
			name: "custom claim allows configured value",
			cfg: AdminConfig{
				Enabled:       true,
				Claim:         "permissions",
				AllowedValues: []string{"gateway:admin"},
			},
			claims: jwt.MapClaims{
				"permissions": []string{"gateway:read", "gateway:admin"},
			},
			expectStatus: http.StatusOK,
		},
		{
			name: "missing claims denied",
			cfg: AdminConfig{
				Enabled: true,
			},
			expectStatus: http.StatusForbidden,
		},
		{
			name: "non admin claim denied",
			cfg: AdminConfig{
				Enabled: true,
			},
			claims: jwt.MapClaims{
				"role": "user",
			},
			expectStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			if tt.claims != nil {
				req = req.WithContext(context.WithValue(req.Context(), claimsKey, tt.claims))
			}

			rec := httptest.NewRecorder()
			NewAdmin(tt.cfg).Handler(next).ServeHTTP(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code)
		})
	}
}

func TestValidateToken(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	tests := []struct {
		name        string
		setupToken  func() string
		expectError bool
		validate    func(t *testing.T, claims jwt.MapClaims)
	}{
		{
			name: "valid token with all required claims",
			setupToken: func() string {
				claims := jwt.MapClaims{
					"sub":   "user123",
					"email": "user@example.com",
					"iss":   server.server.URL,
					"aud":   testAudience,
					"exp":   time.Now().Add(time.Hour).Unix(),
					"iat":   time.Now().Unix(),
				}
				token, err := generateTestJWT(testPrivateKey, claims, testKID)
				require.NoError(t, err)
				return token
			},
			expectError: false,
			validate: func(t *testing.T, claims jwt.MapClaims) {
				assert.Equal(t, "user123", claims["sub"])
				assert.Equal(t, "user@example.com", claims["email"])
			},
		},
		{
			name: "token with audience as array",
			setupToken: func() string {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": []interface{}{testAudience, "other-audience"},
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(testPrivateKey, claims, testKID)
				require.NoError(t, err)
				return token
			},
			expectError: false,
		},
		{
			name: "token with audience array not matching",
			setupToken: func() string {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": []interface{}{"wrong-audience", "other-audience"},
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(testPrivateKey, claims, testKID)
				require.NoError(t, err)
				return token
			},
			expectError: true,
		},
		{
			name: "token with invalid audience format",
			setupToken: func() string {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": 12345, // Invalid type
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(testPrivateKey, claims, testKID)
				require.NoError(t, err)
				return token
			},
			expectError: true,
		},
		{
			name: "token signed with wrong key",
			setupToken: func() string {
				wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
				require.NoError(t, err)

				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": testAudience,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(wrongKey, claims, testKID)
				require.NoError(t, err)
				return token
			},
			expectError: true,
		},
		{
			name: "token with unknown kid triggers JWKS refresh",
			setupToken: func() string {
				// Create a new key pair
				newKey, err := rsa.GenerateKey(rand.Reader, 2048)
				require.NoError(t, err)
				newKID := "new-key-id"

				// Update the JWKS to include the new key
				nBytes := newKey.PublicKey.N.Bytes()
				eBytes := big.NewInt(int64(newKey.PublicKey.E)).Bytes()
				n := base64.RawURLEncoding.EncodeToString(nBytes)
				e := base64.RawURLEncoding.EncodeToString(eBytes)

				jwks := map[string]interface{}{
					"keys": []map[string]interface{}{
						{
							"kid": testKID,
							"kty": "RSA",
							"use": "sig",
							"n":   base64.RawURLEncoding.EncodeToString(testPublicKey.N.Bytes()),
							"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(testPublicKey.E)).Bytes()),
						},
						{
							"kid": newKID,
							"kty": "RSA",
							"use": "sig",
							"n":   n,
							"e":   e,
						},
					},
				}
				jwksResponse, _ := json.Marshal(jwks)
				server.updateJWKS(jwksResponse)

				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": testAudience,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(newKey, claims, newKID)
				require.NoError(t, err)
				return token
			},
			expectError: false,
			validate: func(t *testing.T, claims jwt.MapClaims) {
				assert.Equal(t, "user123", claims["sub"])
			},
		},
		{
			name: "token with completely unknown kid after refresh",
			setupToken: func() string {
				unknownKey, err := rsa.GenerateKey(rand.Reader, 2048)
				require.NoError(t, err)

				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": testAudience,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token, err := generateTestJWT(unknownKey, claims, "completely-unknown-kid")
				require.NoError(t, err)
				return token
			},
			expectError: true,
		},
		{
			name: "malformed token",
			setupToken: func() string {
				return "not.a.valid.jwt.token"
			},
			expectError: true,
		},
		{
			name: "token with non-RSA signing method",
			setupToken: func() string {
				claims := jwt.MapClaims{
					"sub": "user123",
					"iss": server.server.URL,
					"aud": testAudience,
					"exp": time.Now().Add(time.Hour).Unix(),
					"iat": time.Now().Unix(),
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				token.Header["kid"] = testKID
				tokenString, err := token.SignedString([]byte("secret"))
				require.NoError(t, err)
				return tokenString
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := tt.setupToken()
			claims, err := m.validateToken(token)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, claims)

			if tt.validate != nil {
				tt.validate(t, claims)
			}
		})
	}
}

func TestValidateToken_NoAudienceConfigured(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: "", // No audience required
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	// Token without audience should be valid
	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": server.server.URL,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token, err := generateTestJWT(testPrivateKey, claims, testKID)
	require.NoError(t, err)

	validatedClaims, err := m.validateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user123", validatedClaims["sub"])
}

func TestRefreshJWKS(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *mockJWKSServer
		expectError bool
		validate    func(t *testing.T, m *Middleware)
	}{
		{
			name: "successful JWKS fetch and parse",
			setupServer: func() *mockJWKSServer {
				return newMockJWKSServer(testPublicKey, testKID)
			},
			expectError: false,
			validate: func(t *testing.T, m *Middleware) {
				assert.Len(t, m.keys, 1)
				assert.Contains(t, m.keys, testKID)
			},
		},
		{
			name: "OIDC discovery failure",
			setupServer: func() *mockJWKSServer {
				server := newMockJWKSServer(testPublicKey, testKID)
				server.setFailNext()
				return server
			},
			expectError: true,
		},
		{
			name: "JWKS with multiple keys",
			setupServer: func() *mockJWKSServer {
				server := newMockJWKSServer(testPublicKey, testKID)

				// Add another key
				key2, _ := rsa.GenerateKey(rand.Reader, 2048)
				kid2 := "test-key-id-2"
				nBytes := key2.PublicKey.N.Bytes()
				eBytes := big.NewInt(int64(key2.PublicKey.E)).Bytes()
				n := base64.RawURLEncoding.EncodeToString(nBytes)
				e := base64.RawURLEncoding.EncodeToString(eBytes)

				jwks := map[string]interface{}{
					"keys": []map[string]interface{}{
						{
							"kid": testKID,
							"kty": "RSA",
							"use": "sig",
							"n":   base64.RawURLEncoding.EncodeToString(testPublicKey.N.Bytes()),
							"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(testPublicKey.E)).Bytes()),
						},
						{
							"kid": kid2,
							"kty": "RSA",
							"use": "sig",
							"n":   n,
							"e":   e,
						},
					},
				}
				jwksResponse, _ := json.Marshal(jwks)
				server.updateJWKS(jwksResponse)
				return server
			},
			expectError: false,
			validate: func(t *testing.T, m *Middleware) {
				assert.Len(t, m.keys, 2)
				assert.Contains(t, m.keys, testKID)
				assert.Contains(t, m.keys, "test-key-id-2")
			},
		},
		{
			name: "JWKS with EC key with missing coordinates skipped gracefully",
			setupServer: func() *mockJWKSServer {
				server := newMockJWKSServer(testPublicKey, testKID)

				jwks := map[string]interface{}{
					"keys": []map[string]interface{}{
						{
							"kid": testKID,
							"kty": "RSA",
							"use": "sig",
							"n":   base64.RawURLEncoding.EncodeToString(testPublicKey.N.Bytes()),
							"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(testPublicKey.E)).Bytes()),
						},
						{
							"kid": "ec-key-bad",
							"kty": "EC",
							"use": "sig",
							"crv": "P-256",
							// x and y missing — parseECKey should fail, key skipped
						},
					},
				}
				jwksResponse, _ := json.Marshal(jwks)
				server.updateJWKS(jwksResponse)
				return server
			},
			expectError: false,
			validate: func(t *testing.T, m *Middleware) {
				// Only the valid RSA key should be loaded; malformed EC key skipped.
				assert.Len(t, m.keys, 1)
				assert.Contains(t, m.keys, testKID)
			},
		},
		{
			name: "JWKS with wrong use field skipped",
			setupServer: func() *mockJWKSServer {
				server := newMockJWKSServer(testPublicKey, testKID)

				jwks := map[string]interface{}{
					"keys": []map[string]interface{}{
						{
							"kid": testKID,
							"kty": "RSA",
							"use": "sig",
							"n":   base64.RawURLEncoding.EncodeToString(testPublicKey.N.Bytes()),
							"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(testPublicKey.E)).Bytes()),
						},
						{
							"kid": "enc-key",
							"kty": "RSA",
							"use": "enc", // Wrong use
							"n":   base64.RawURLEncoding.EncodeToString(testPublicKey.N.Bytes()),
							"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(testPublicKey.E)).Bytes()),
						},
					},
				}
				jwksResponse, _ := json.Marshal(jwks)
				server.updateJWKS(jwksResponse)
				return server
			},
			expectError: false,
			validate: func(t *testing.T, m *Middleware) {
				// Only key with use=sig should be loaded
				assert.Len(t, m.keys, 1)
				assert.Contains(t, m.keys, testKID)
			},
		},
		{
			name: "JWKS with invalid base64 encoding skipped",
			setupServer: func() *mockJWKSServer {
				server := newMockJWKSServer(testPublicKey, testKID)

				jwks := map[string]interface{}{
					"keys": []map[string]interface{}{
						{
							"kid": testKID,
							"kty": "RSA",
							"use": "sig",
							"n":   base64.RawURLEncoding.EncodeToString(testPublicKey.N.Bytes()),
							"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(testPublicKey.E)).Bytes()),
						},
						{
							"kid": "bad-key",
							"kty": "RSA",
							"use": "sig",
							"n":   "!!!invalid-base64!!!",
							"e":   "AQAB",
						},
					},
				}
				jwksResponse, _ := json.Marshal(jwks)
				server.updateJWKS(jwksResponse)
				return server
			},
			expectError: false,
			validate: func(t *testing.T, m *Middleware) {
				// Only valid key should be loaded
				assert.Len(t, m.keys, 1)
				assert.Contains(t, m.keys, testKID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.close()

			cfg := Config{
				Enabled:  true,
				Issuer:   server.server.URL,
				Audience: testAudience,
			}

			m := &Middleware{
				cfg:    cfg,
				keys:   make(map[string]interface{}),
				client: &http.Client{Timeout: 10 * time.Second},
				logger: slog.Default(),
			}

			err := m.refreshJWKS()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.validate != nil {
				tt.validate(t, m)
			}
		})
	}
}

func TestRefreshJWKS_Concurrency(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	// Trigger concurrent refreshes
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.refreshJWKS()
		}()
	}

	wg.Wait()

	// Verify keys are still valid
	m.mu.RLock()
	defer m.mu.RUnlock()
	assert.Len(t, m.keys, 1)
	assert.Contains(t, m.keys, testKID)
}

func TestGetClaims(t *testing.T) {
	tests := []struct {
		name            string
		setupContext    func() context.Context
		expectFound     bool
		validateSubject string
	}{
		{
			name: "context with claims",
			setupContext: func() context.Context {
				claims := jwt.MapClaims{
					"sub":   "user123",
					"email": "user@example.com",
				}
				return context.WithValue(context.Background(), claimsKey, claims)
			},
			expectFound:     true,
			validateSubject: "user123",
		},
		{
			name: "context without claims",
			setupContext: func() context.Context {
				return context.Background()
			},
			expectFound: false,
		},
		{
			name: "context with wrong type",
			setupContext: func() context.Context {
				return context.WithValue(context.Background(), claimsKey, "not-claims")
			},
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupContext()
			claims, ok := GetClaims(ctx)

			if tt.expectFound {
				assert.True(t, ok)
				assert.NotNil(t, claims)
				if tt.validateSubject != "" {
					assert.Equal(t, tt.validateSubject, claims["sub"])
				}
			} else {
				assert.False(t, ok)
			}
		})
	}
}

func TestMiddleware_IssuerWithTrailingSlash(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	// Issuer is preserved as-is; the token iss must match exactly.
	issuerWithSlash := server.server.URL + "/"
	cfg := Config{
		Enabled:  true,
		Issuer:   issuerWithSlash,
		Audience: testAudience,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Len(t, m.keys, 1)
	assert.Equal(t, issuerWithSlash, m.cfg.Issuer)

	// Token iss with trailing slash must be accepted.
	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": issuerWithSlash,
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token, err := generateTestJWT(testPrivateKey, claims, testKID)
	require.NoError(t, err)

	validatedClaims, err := m.validateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user123", validatedClaims["sub"])

	// Token iss WITHOUT trailing slash must be rejected.
	claimsNoSlash := jwt.MapClaims{
		"sub": "user456",
		"iss": server.server.URL,
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	tokenNoSlash, err := generateTestJWT(testPrivateKey, claimsNoSlash, testKID)
	require.NoError(t, err)

	_, err = m.validateToken(tokenNoSlash)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// New tests: algorithm support, nbf, clock-skew, stale-key, rate limiting
// ---------------------------------------------------------------------------

func TestValidateToken_ES256(t *testing.T) {
	server := newMockJWKSServerWithEC(testECPublicKey, testECKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	claims := jwt.MapClaims{
		"sub": "user-ec",
		"iss": server.server.URL,
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token, err := generateECTestJWT(testECPrivateKey, claims, testECKID)
	require.NoError(t, err)

	validated, err := m.validateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-ec", validated["sub"])
}

func TestValidateToken_UnsupportedAlgorithm(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": server.server.URL,
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = testKID
	tokenString, err := token.SignedString([]byte("symmetric-secret"))
	require.NoError(t, err)

	_, err = m.validateToken(tokenString)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported signing algorithm")
}

func TestValidateToken_NotBefore(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	// nbf one minute in the future → token not yet valid.
	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": server.server.URL,
		"aud": testAudience,
		"exp": time.Now().Add(2 * time.Hour).Unix(),
		"nbf": time.Now().Add(time.Minute).Unix(),
		"iat": time.Now().Unix(),
	}
	token, err := generateTestJWT(testPrivateKey, claims, testKID)
	require.NoError(t, err)

	_, err = m.validateToken(token)
	assert.Error(t, err, "token with future nbf should be rejected")
}

func TestValidateToken_ClockSkew_NBF(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:   true,
		Issuer:    server.server.URL,
		Audience:  testAudience,
		ClockSkew: 30 * time.Second,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	// nbf 10 s in the future — within the 30 s leeway window.
	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": server.server.URL,
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"nbf": time.Now().Add(10 * time.Second).Unix(),
		"iat": time.Now().Unix(),
	}
	token, err := generateTestJWT(testPrivateKey, claims, testKID)
	require.NoError(t, err)

	validated, err := m.validateToken(token)
	require.NoError(t, err, "token with nbf within clock-skew leeway should be accepted")
	assert.Equal(t, "user123", validated["sub"])
}

func TestValidateToken_ClockSkew_Expiry(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:   true,
		Issuer:    server.server.URL,
		Audience:  testAudience,
		ClockSkew: 30 * time.Second,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	// exp 10 s in the past — within the 30 s leeway window.
	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": server.server.URL,
		"aud": testAudience,
		"exp": time.Now().Add(-10 * time.Second).Unix(),
		"iat": time.Now().Add(-time.Hour).Unix(),
	}
	token, err := generateTestJWT(testPrivateKey, claims, testKID)
	require.NoError(t, err)

	validated, err := m.validateToken(token)
	require.NoError(t, err, "token expired within clock-skew leeway should be accepted")
	assert.Equal(t, "user123", validated["sub"])
}

func TestValidateToken_ClockSkew_ExpiredBeyondLeeway(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:   true,
		Issuer:    server.server.URL,
		Audience:  testAudience,
		ClockSkew: 30 * time.Second,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	// exp 2 min in the past — beyond the 30 s leeway window.
	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": server.server.URL,
		"aud": testAudience,
		"exp": time.Now().Add(-2 * time.Minute).Unix(),
		"iat": time.Now().Add(-3 * time.Minute).Unix(),
	}
	token, err := generateTestJWT(testPrivateKey, claims, testKID)
	require.NoError(t, err)

	_, err = m.validateToken(token)
	assert.Error(t, err, "token expired beyond clock-skew leeway should be rejected")
}

// TestStaleKeys_IssuerOutage verifies that previously fetched keys continue to be
// used when the IdP JWKS endpoint becomes temporarily unavailable.
func TestStaleKeys_IssuerOutage(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
		// StaleTTL=0: serve stale keys indefinitely during outages.
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	validClaims := jwt.MapClaims{
		"sub": "user123",
		"iss": server.server.URL,
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	// Confirm token works while IdP is up.
	token, err := generateTestJWT(testPrivateKey, validClaims, testKID)
	require.NoError(t, err)
	_, err = m.validateToken(token)
	require.NoError(t, err)

	// Simulate IdP going down by closing the test server.
	server.server.Close()

	// Force a failed periodic-refresh cycle.
	_ = m.refreshJWKS()

	// Token with known cached key should still validate using stale keys.
	token2, err := generateTestJWT(testPrivateKey, validClaims, testKID)
	require.NoError(t, err)
	validated, err := m.validateToken(token2)
	require.NoError(t, err, "stale keys should keep serving known key IDs during outage")
	assert.Equal(t, "user123", validated["sub"])
}

// TestStaleTTL_EnforcedOnExpiry verifies that tokens are rejected when stale keys
// exceed the configured StaleTTL and refresh continues to fail.
func TestStaleTTL_EnforcedOnExpiry(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
		StaleTTL: 50 * time.Millisecond,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	// Backdate lastFetchedAt beyond StaleTTL.
	m.mu.Lock()
	m.lastFetchedAt = time.Now().Add(-time.Second)
	m.mu.Unlock()

	// Bring IdP down so forced refresh fails.
	server.server.Close()

	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": server.server.URL,
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token, err := generateTestJWT(testPrivateKey, claims, testKID)
	require.NoError(t, err)

	_, err = m.validateToken(token)
	assert.Error(t, err, "tokens should be rejected when stale keys exceed StaleTTL and refresh fails")
	assert.Contains(t, err.Error(), "stale")
}

// TestRefresh_RateLimiting verifies that rapid unknown-kid requests trigger at most
// one on-demand JWKS refresh within the cooldown window.
func TestRefresh_RateLimiting(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	// Record the request count after initialisation.
	server.mu.Lock()
	baseline := server.requestCount
	server.mu.Unlock()

	// Set lastRefreshAt to now so the cooldown is active.
	m.refreshMu.Lock()
	m.lastRefreshAt = time.Now()
	m.refreshMu.Unlock()

	// Issue multiple concurrent refreshIfCooledDown calls — all should be no-ops.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.refreshIfCooledDown()
		}()
	}
	wg.Wait()

	server.mu.Lock()
	after := server.requestCount
	server.mu.Unlock()

	assert.Equal(t, baseline, after, "no additional JWKS requests should be made within cooldown window")
}

// TestRefreshJWKS_ECKeys verifies that EC keys in the JWKS are parsed and loaded correctly.
func TestRefreshJWKS_ECKeys(t *testing.T) {
	server := newMockJWKSServerWithEC(testECPublicKey, testECKID)
	defer server.close()

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}

	m := &Middleware{
		cfg:    cfg,
		keys:   make(map[string]interface{}),
		client: &http.Client{Timeout: 10 * time.Second},
		logger: slog.Default(),
	}

	err := m.refreshJWKS()
	require.NoError(t, err)
	assert.Len(t, m.keys, 1)
	assert.Contains(t, m.keys, testECKID)

	key, ok := m.keys[testECKID].(*ecdsa.PublicKey)
	require.True(t, ok, "loaded key should be *ecdsa.PublicKey")
	assert.Equal(t, elliptic.P256(), key.Curve)
}

// TestRefreshJWKS_MixedRSAandEC verifies that a JWKS containing both RSA and EC keys
// loads all usable signing keys.
func TestRefreshJWKS_MixedRSAandEC(t *testing.T) {
	server := newMockJWKSServer(testPublicKey, testKID)
	defer server.close()

	jwks := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kid": testKID,
				"kty": "RSA",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(testPublicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(testPublicKey.E)).Bytes()),
			},
			{
				"kid": testECKID,
				"kty": "EC",
				"use": "sig",
				"alg": "ES256",
				"crv": "P-256",
				"x":   base64.RawURLEncoding.EncodeToString(testECPublicKey.X.Bytes()),
				"y":   base64.RawURLEncoding.EncodeToString(testECPublicKey.Y.Bytes()),
			},
		},
	}
	jwksResponse, _ := json.Marshal(jwks)
	server.updateJWKS(jwksResponse)

	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL,
		Audience: testAudience,
	}

	m := &Middleware{
		cfg:    cfg,
		keys:   make(map[string]interface{}),
		client: &http.Client{Timeout: 10 * time.Second},
		logger: slog.Default(),
	}

	err := m.refreshJWKS()
	require.NoError(t, err)
	assert.Len(t, m.keys, 2)
	assert.Contains(t, m.keys, testKID)
	assert.Contains(t, m.keys, testECKID)
	_, rsaOK := m.keys[testKID].(*rsa.PublicKey)
	_, ecOK := m.keys[testECKID].(*ecdsa.PublicKey)
	assert.True(t, rsaOK, "RSA key should be *rsa.PublicKey")
	assert.True(t, ecOK, "EC key should be *ecdsa.PublicKey")
}
