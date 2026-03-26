package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ajac-zero/latticelm/internal/users"
)

// ---- mock OIDC server ----

type mockOIDCServer struct {
	server       *httptest.Server
	tokenPayload map[string]interface{}
	tokenStatus  int
}

func newMockOIDCServer(t *testing.T) *mockOIDCServer {
	t.Helper()
	m := &mockOIDCServer{
		tokenPayload: map[string]interface{}{},
		tokenStatus:  http.StatusOK,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		base := m.server.URL
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": base + "/authorize",
			"token_endpoint":         base + "/token",
			"userinfo_endpoint":      base + "/userinfo",
		})
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(m.tokenStatus)
		if m.tokenStatus == http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(m.tokenPayload)
		} else {
			_, _ = w.Write([]byte("token error"))
		}
	})

	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

// buildIDToken creates a signed (but trivially verifiable) JWT for tests.
func buildIDToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

func newOIDCClient(t *testing.T, srv *mockOIDCServer, userStore *users.Store) *OIDCClient {
	t.Helper()
	sessionStore := NewSessionStore(time.Hour)
	cfg := OIDCClientConfig{
		Issuer:      srv.server.URL,
		ClientID:    "test-client",
		RedirectURI: "http://localhost/callback",
	}
	client, err := NewOIDCClient(cfg, sessionStore, userStore, newNopLogger())
	require.NoError(t, err)
	return client
}

// ---- DB helpers (shared container) ----

var oidcTestDB *sql.DB

func TestMain(m *testing.M) {
	// Only spin up the DB if the package doesn't already have a TestMain —
	// this file provides it. We need a real users.Store for the callback test.
	ctx := context.Background()

	pgCtr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		panic(err)
	}
	connStr, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}
	oidcTestDB, err = sql.Open("pgx", connStr)
	if err != nil {
		panic(err)
	}
	if _, err := users.Migrate(ctx, oidcTestDB, "pgx"); err != nil {
		panic(err)
	}

	code := m.Run()
	oidcTestDB.Close()
	_ = pgCtr.Terminate(ctx)

	// os.Exit is called by testing framework
	_ = code
}

func newUserStore(t *testing.T) *users.Store {
	t.Helper()
	return users.NewStore(oidcTestDB, "pgx")
}

// ---- pure helper tests ----

func TestWantsJSON(t *testing.T) {
	tests := []struct {
		accept string
		want   bool
	}{
		{"application/json", true},
		{"application/json, text/html", true},
		{"text/html", false},
		{"", false},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if tt.accept != "" {
			r.Header.Set("Accept", tt.accept)
		}
		assert.Equal(t, tt.want, wantsJSON(r), "accept=%q", tt.accept)
	}
}

func TestIsAPIRequest(t *testing.T) {
	tests := []struct {
		path   string
		accept string
		want   bool
	}{
		{"/api/users", "", true},
		{"/v1/responses", "", true},
		{"/", "", false},
		{"/dashboard", "", false},
		{"/dashboard", "application/json", true},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, tt.path, nil)
		if tt.accept != "" {
			r.Header.Set("Accept", tt.accept)
		}
		assert.Equal(t, tt.want, isAPIRequest(r), "path=%q accept=%q", tt.path, tt.accept)
	}
}

func TestGenerateRandomString(t *testing.T) {
	s1, err := generateRandomString(32)
	require.NoError(t, err)
	assert.Len(t, s1, 32)

	s2, err := generateRandomString(32)
	require.NoError(t, err)
	assert.NotEqual(t, s1, s2)

	s3, err := generateRandomString(43)
	require.NoError(t, err)
	assert.Len(t, s3, 43)
}

func TestCreateCodeChallenge(t *testing.T) {
	// RFC 7636 test vector isn't critical; just verify it's deterministic and non-empty.
	v := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	c1 := createCodeChallenge(v)
	c2 := createCodeChallenge(v)
	assert.Equal(t, c1, c2)
	assert.NotEmpty(t, c1)
	// Different verifier → different challenge.
	assert.NotEqual(t, createCodeChallenge("other"), c1)
}

func TestGetClaimString(t *testing.T) {
	claims := jwt.MapClaims{
		"sub":   "user-123",
		"email": "alice@example.com",
		"count": 42, // not a string
	}
	assert.Equal(t, "user-123", getClaimString(claims, "sub"))
	assert.Equal(t, "alice@example.com", getClaimString(claims, "email"))
	assert.Equal(t, "", getClaimString(claims, "count"))
	assert.Equal(t, "", getClaimString(claims, "missing"))
}

func TestGetTenantID(t *testing.T) {
	tests := []struct {
		claims jwt.MapClaims
		want   string
	}{
		{jwt.MapClaims{"org_id": "org-1"}, "org-1"},
		{jwt.MapClaims{"tenant_id": "tenant-1"}, "tenant-1"},
		{jwt.MapClaims{"tid": "tid-1"}, "tid-1"},
		{jwt.MapClaims{}, ""},
		{jwt.MapClaims{"org_id": ""}, ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, getTenantID(tt.claims))
	}
}

func TestParseIDToken(t *testing.T) {
	claims := jwt.MapClaims{
		"sub":   "user-abc",
		"email": "test@example.com",
		"iss":   "https://issuer.example",
	}
	idToken := buildIDToken(t, claims)

	sessionStore := NewSessionStore(time.Hour)
	srv := newMockOIDCServer(t)
	cfg := OIDCClientConfig{Issuer: srv.server.URL, ClientID: "c", RedirectURI: "http://localhost/cb"}
	c, err := NewOIDCClient(cfg, sessionStore, nil, newNopLogger())
	require.NoError(t, err)

	parsed, err := c.parseIDToken(idToken)
	require.NoError(t, err)
	assert.Equal(t, "user-abc", getClaimString(parsed, "sub"))
	assert.Equal(t, "test@example.com", getClaimString(parsed, "email"))
}

func TestParseIDToken_Invalid(t *testing.T) {
	srv := newMockOIDCServer(t)
	cfg := OIDCClientConfig{Issuer: srv.server.URL, ClientID: "c", RedirectURI: "http://localhost/cb"}
	c, err := NewOIDCClient(cfg, NewSessionStore(time.Hour), nil, newNopLogger())
	require.NoError(t, err)

	_, err = c.parseIDToken("not.a.jwt")
	assert.Error(t, err)
}

// ---- discovery tests ----

func TestNewOIDCClient_DiscoverySuccess(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)
	assert.Equal(t, srv.server.URL+"/authorize", client.authorizationEndpoint)
	assert.Equal(t, srv.server.URL+"/token", client.tokenEndpoint)
}

func TestNewOIDCClient_DiscoveryFailure(t *testing.T) {
	sessionStore := NewSessionStore(time.Hour)
	cfg := OIDCClientConfig{
		Issuer:      "http://127.0.0.1:19999", // nothing listening
		ClientID:    "c",
		RedirectURI: "http://localhost/cb",
	}
	_, err := NewOIDCClient(cfg, sessionStore, nil, newNopLogger())
	assert.Error(t, err)
}

func TestNewOIDCClient_DiscoveryBadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	sessionStore := NewSessionStore(time.Hour)
	cfg := OIDCClientConfig{Issuer: ts.URL, ClientID: "c", RedirectURI: "http://localhost/cb"}
	_, err := NewOIDCClient(cfg, sessionStore, nil, newNopLogger())
	assert.Error(t, err)
}

func TestNewOIDCClient_CustomDiscoveryURL(t *testing.T) {
	// Discovery URL overrides the derived one.
	customPath := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": "https://custom/authorize",
			"token_endpoint":         "https://custom/token",
			"userinfo_endpoint":      "https://custom/userinfo",
		})
	}))
	defer customPath.Close()

	sessionStore := NewSessionStore(time.Hour)
	cfg := OIDCClientConfig{
		Issuer:       "https://issuer.invalid",
		DiscoveryURL: customPath.URL,
		ClientID:     "c",
		RedirectURI:  "http://localhost/cb",
	}
	c, err := NewOIDCClient(cfg, sessionStore, nil, newNopLogger())
	require.NoError(t, err)
	assert.Equal(t, "https://custom/authorize", c.authorizationEndpoint)
}

// ---- login handler tests ----

func TestHandleLogin_Redirect(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rr := httptest.NewRecorder()
	client.HandleLogin(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
	location := rr.Header().Get("Location")
	assert.Contains(t, location, srv.server.URL+"/authorize")
	assert.Contains(t, location, "client_id=test-client")
	assert.Contains(t, location, "code_challenge_method=S256")

	// Both state and verifier cookies must be set.
	cookies := rr.Result().Cookies()
	cookieNames := make(map[string]bool)
	for _, c := range cookies {
		cookieNames[c.Name] = true
	}
	assert.True(t, cookieNames[oidcStateCookieName])
	assert.True(t, cookieNames[oidcVerifierCookieName])
}

func TestHandleOIDCLogin_GET(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/login", nil)
	rr := httptest.NewRecorder()
	client.HandleOIDCLogin(rr, req)
	assert.Equal(t, http.StatusFound, rr.Code)
}

func TestHandleOIDCLogin_POST(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	req := httptest.NewRequest(http.MethodPost, "/auth/oidc/login", nil)
	rr := httptest.NewRecorder()
	client.HandleOIDCLogin(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	data := resp["data"].(map[string]interface{})
	assert.Contains(t, data["authorization_url"], "/authorize")
}

func TestHandleOIDCLogin_MethodNotAllowed(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)
	req := httptest.NewRequest(http.MethodDelete, "/auth/oidc/login", nil)
	rr := httptest.NewRecorder()
	client.HandleOIDCLogin(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

// ---- callback handler tests ----

func TestHandleCallback_MissingCode(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)
	req := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	rr := httptest.NewRecorder()
	client.HandleCallback(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCallback_MissingStateCookie(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc&state=xyz", nil)
	rr := httptest.NewRecorder()
	client.HandleCallback(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCallback_StateMismatch(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc&state=wrong-state", nil)
	req.AddCookie(&http.Cookie{Name: oidcStateCookieName, Value: "expected-state"})
	req.AddCookie(&http.Cookie{Name: oidcVerifierCookieName, Value: "verifier"})
	rr := httptest.NewRecorder()
	client.HandleCallback(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCallback_MissingVerifierCookie(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc&state=mystate", nil)
	req.AddCookie(&http.Cookie{Name: oidcStateCookieName, Value: "mystate"})
	// No verifier cookie.
	rr := httptest.NewRecorder()
	client.HandleCallback(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleCallback_TokenExchangeError(t *testing.T) {
	srv := newMockOIDCServer(t)
	srv.tokenStatus = http.StatusInternalServerError
	client := newOIDCClient(t, srv, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=mycode&state=mystate", nil)
	req.AddCookie(&http.Cookie{Name: oidcStateCookieName, Value: "mystate"})
	req.AddCookie(&http.Cookie{Name: oidcVerifierCookieName, Value: "verifier"})
	rr := httptest.NewRecorder()
	client.HandleCallback(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleCallback_InvalidIDToken(t *testing.T) {
	srv := newMockOIDCServer(t)
	srv.tokenPayload = map[string]interface{}{
		"access_token": "at",
		"id_token":     "not.a.jwt",
	}
	client := newOIDCClient(t, srv, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=mycode&state=mystate", nil)
	req.AddCookie(&http.Cookie{Name: oidcStateCookieName, Value: "mystate"})
	req.AddCookie(&http.Cookie{Name: oidcVerifierCookieName, Value: "verifier"})
	rr := httptest.NewRecorder()
	client.HandleCallback(rr, req)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleCallback_HappyPath(t *testing.T) {
	srv := newMockOIDCServer(t)
	userStore := newUserStore(t)

	idToken := buildIDToken(t, jwt.MapClaims{
		"iss":   srv.server.URL,
		"sub":   fmt.Sprintf("sub-oidc-%d", time.Now().UnixNano()),
		"email": fmt.Sprintf("oidc-%d@example.com", time.Now().UnixNano()),
		"name":  "OIDC User",
	})
	srv.tokenPayload = map[string]interface{}{
		"access_token":  "at-1",
		"refresh_token": "rt-1",
		"id_token":      idToken,
	}

	client := newOIDCClient(t, srv, userStore)

	// Prepare a real state/verifier by calling HandleLogin first.
	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	loginRR := httptest.NewRecorder()
	client.HandleLogin(loginRR, loginReq)
	require.Equal(t, http.StatusFound, loginRR.Code)

	// Extract cookies from login response.
	var stateCookie, verifierCookie *http.Cookie
	for _, c := range loginRR.Result().Cookies() {
		switch c.Name {
		case oidcStateCookieName:
			stateCookie = c
		case oidcVerifierCookieName:
			verifierCookie = c
		}
	}
	require.NotNil(t, stateCookie)
	require.NotNil(t, verifierCookie)

	// Make callback request with matching state.
	callbackURL := fmt.Sprintf("/auth/callback?code=mycode&state=%s", stateCookie.Value)
	req := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.AddCookie(stateCookie)
	req.AddCookie(verifierCookie)
	rr := httptest.NewRecorder()
	client.HandleCallback(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
	// Session cookie must be set.
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == OIDCSessionCookieName {
			found = true
		}
	}
	assert.True(t, found, "session cookie not set")
}

// ---- logout tests ----

func TestHandleLogout_ClearsSession(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	// Create a session.
	sessionID, err := client.sessionStore.Create(&SessionData{UserID: "u1", Email: "x@x.com"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: OIDCSessionCookieName, Value: sessionID})
	rr := httptest.NewRecorder()
	client.HandleLogout(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Session should be deleted.
	_, exists := client.sessionStore.Get(sessionID)
	assert.False(t, exists)
}

func TestHandleLogout_GET_Redirect(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	rr := httptest.NewRecorder()
	client.HandleLogout(rr, req)
	assert.Equal(t, http.StatusFound, rr.Code)
}

func TestHandleLogout_MethodNotAllowed(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)
	req := httptest.NewRequest(http.MethodPut, "/auth/logout", nil)
	rr := httptest.NewRecorder()
	client.HandleLogout(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleLogout_NoSession(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)
	// POST without cookie — should still succeed (no-op).
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rr := httptest.NewRecorder()
	client.HandleLogout(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

// ---- HandleUser tests ----

func TestHandleUser_OK(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	sessionID, err := client.sessionStore.Create(&SessionData{
		UserID:  "u1",
		Email:   "user@example.com",
		Name:    "Test User",
		IsAdmin: false,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/auth/user", nil)
	req.AddCookie(&http.Cookie{Name: OIDCSessionCookieName, Value: sessionID})
	rr := httptest.NewRecorder()
	client.HandleUser(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "user@example.com", data["email"])
}

func TestHandleUser_Unauthorized(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/user", nil)
	rr := httptest.NewRecorder()
	client.HandleUser(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// ---- SessionMiddleware tests ----

func TestSessionMiddleware_ValidSession(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	sessionID, err := client.sessionStore.Create(&SessionData{
		UserID:  "u1",
		Email:   "mid@example.com",
		IsAdmin: true,
	})
	require.NoError(t, err)

	var capturedCtx context.Context
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	})

	handler := client.SessionMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: OIDCSessionCookieName, Value: sessionID})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	principal := PrincipalFromContext(capturedCtx)
	require.NotNil(t, principal)
	assert.Equal(t, "u1", principal.Subject)
}

func TestSessionMiddleware_NoSession_APIRequest(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := client.SessionMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestSessionMiddleware_NoSession_PageRequest(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := client.SessionMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "/auth/login", rr.Header().Get("Location"))
}

func TestSessionMiddleware_SkipsAuthPaths(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := client.SessionMiddleware(next)
	for _, path := range []string{"/auth/login", "/assets/app.js", "/favicon.ico", "/manifest.json", "/robots.txt"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "path=%s should be skipped", path)
	}
}

// ---- prepareAuthorizationURL localhost cookie domain ----

func TestPrepareAuthorizationURL_LocalhostCookieDomain(t *testing.T) {
	srv := newMockOIDCServer(t)
	client := newOIDCClient(t, srv, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	req.Host = "localhost:8080"
	rr := httptest.NewRecorder()
	client.HandleLogin(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
	for _, c := range rr.Result().Cookies() {
		if c.Name == oidcStateCookieName || c.Name == oidcVerifierCookieName {
			assert.Equal(t, "localhost", c.Domain)
		}
	}
}

// ---- writeJSON helpers ----

func TestWriteJSONError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSONError(rr, "something went wrong", http.StatusBadRequest)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["success"])
}

func TestWriteJSONSuccess(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSONSuccess(rr, map[string]string{"key": "value"})
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["success"])
}

// ---- admin auto-promotion ----

func TestHandleCallback_AdminAutoPromotion(t *testing.T) {
	srv := newMockOIDCServer(t)
	userStore := newUserStore(t)

	email := fmt.Sprintf("admin-%d@example.com", time.Now().UnixNano())
	sub := fmt.Sprintf("sub-admin-%d", time.Now().UnixNano())

	idToken := buildIDToken(t, jwt.MapClaims{
		"iss":   srv.server.URL,
		"sub":   sub,
		"email": email,
		"name":  "Admin User",
	})
	srv.tokenPayload = map[string]interface{}{
		"access_token": "at",
		"id_token":     idToken,
	}

	sessionStore := NewSessionStore(time.Hour)
	cfg := OIDCClientConfig{
		Issuer:      srv.server.URL,
		ClientID:    "test-client",
		RedirectURI: "http://localhost/callback",
		AdminEmail:  email, // auto-promote this email
	}
	client, err := NewOIDCClient(cfg, sessionStore, userStore, newNopLogger())
	require.NoError(t, err)

	// Get valid cookies via login.
	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	loginRR := httptest.NewRecorder()
	client.HandleLogin(loginRR, loginReq)

	var stateCookie, verifierCookie *http.Cookie
	for _, c := range loginRR.Result().Cookies() {
		switch c.Name {
		case oidcStateCookieName:
			stateCookie = c
		case oidcVerifierCookieName:
			verifierCookie = c
		}
	}

	callbackURL := fmt.Sprintf("/auth/callback?code=mycode&state=%s", stateCookie.Value)
	req := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.AddCookie(stateCookie)
	req.AddCookie(verifierCookie)
	rr := httptest.NewRecorder()
	client.HandleCallback(rr, req)

	require.Equal(t, http.StatusFound, rr.Code)

	// Verify user was promoted to admin in the DB.
	u, err := userStore.GetByEmail(context.Background(), email)
	require.NoError(t, err)
	assert.Equal(t, users.RoleAdmin, u.Role)
}

// ---- helpers ----

func newNopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

