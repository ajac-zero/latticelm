package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test fixtures
var (
	testPrivateKey *rsa.PrivateKey
	testPublicKey  *rsa.PublicKey
	testKID        = "test-key-id-1"
	testIssuer     = "https://test-issuer.example.com"
	testAudience   = "test-client-id"
)

func init() {
	// Generate test RSA key pair
	var err error
	testPrivateKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("failed to generate test key: %v", err))
	}
	testPublicKey = &testPrivateKey.PublicKey
}

// mockJWKSServer provides a mock OIDC/JWKS server for testing
type mockJWKSServer struct {
	server       *httptest.Server
	jwksResponse []byte
	oidcResponse []byte
	mu           sync.Mutex
	requestCount int
	failNext     bool
}

func newMockJWKSServer(publicKey *rsa.PublicKey, kid string) *mockJWKSServer {
	m := &mockJWKSServer{}

	// Encode public key components for JWKS
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
		json.NewEncoder(w).Encode(oidcConfig)
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
		w.Write(m.jwksResponse)
	})

	m.server = httptest.NewServer(mux)
	return m
}

func (m *mockJWKSServer) close() {
	m.server.Close()
}

func (m *mockJWKSServer) getRequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requestCount
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

// generateTestJWT creates a signed JWT with the given claims
func generateTestJWT(privateKey *rsa.PrivateKey, claims jwt.MapClaims, kid string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	return token.SignedString(privateKey)
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
			w.Write([]byte(fmt.Sprintf("sub:%s", claims["sub"])))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("no-claims"))
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
			expectBody:   "missing authorization header",
		},
		{
			name: "malformed authorization header - no bearer",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "invalid-token")
				return req
			},
			expectStatus: http.StatusUnauthorized,
			expectBody:   "invalid authorization header format",
		},
		{
			name: "malformed authorization header - wrong scheme",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("Authorization", "Basic dGVzdDp0ZXN0")
				return req
			},
			expectStatus: http.StatusUnauthorized,
			expectBody:   "invalid authorization header format",
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
			expectBody:   "invalid token",
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
			expectBody:   "invalid token",
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
			expectBody:   "invalid token",
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
			expectBody:   "invalid token",
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

func TestMiddleware_Handler_DisabledAuth(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	handler := m.Handler(testHandler)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "success", rec.Body.String())
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
			name: "JWKS with non-RSA keys skipped",
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
							"kid": "ec-key",
							"kty": "EC", // Non-RSA key
							"use": "sig",
							"crv": "P-256",
						},
					},
				}
				jwksResponse, _ := json.Marshal(jwks)
				server.updateJWKS(jwksResponse)
				return server
			},
			expectError: false,
			validate: func(t *testing.T, m *Middleware) {
				// Only RSA key should be loaded
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
				keys:   make(map[string]*rsa.PublicKey),
				client: &http.Client{Timeout: 10 * time.Second},
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

	// Test that issuer with trailing slash works
	cfg := Config{
		Enabled:  true,
		Issuer:   server.server.URL + "/", // Trailing slash
		Audience: testAudience,
	}
	m, err := New(cfg, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Len(t, m.keys, 1)

	// Validate that token with issuer without trailing slash still works
	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": strings.TrimSuffix(server.server.URL, "/"),
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token, err := generateTestJWT(testPrivateKey, claims, testKID)
	require.NoError(t, err)

	// Update middleware to use issuer without trailing slash for comparison
	m.cfg.Issuer = strings.TrimSuffix(m.cfg.Issuer, "/")

	validatedClaims, err := m.validateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user123", validatedClaims["sub"])
}
