package auth

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthAPIHandleSessionAuthDisabled(t *testing.T) {
	api := NewAPI(false, false, nil, nil, AdminConfig{})

	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	rec := httptest.NewRecorder()

	api.HandleSession(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	data := decodeResponseData(t, rec)
	assert.Equal(t, false, data["auth_enabled"])
	assert.Equal(t, false, data["authenticated"])
	assert.Equal(t, "none", data["mode"])
}

func TestAuthAPITokenLoginAndSession(t *testing.T) {
	mockServer := newMockJWKSServer(testPublicKey, testKID)
	defer mockServer.close()

	middleware, err := New(Config{Enabled: true, Issuer: mockServer.server.URL}, slog.Default())
	require.NoError(t, err)

	api := NewAPI(true, false, middleware, nil, AdminConfig{
		Enabled:       true,
		Claim:         "role",
		AllowedValues: []string{"admin"},
	})

	claims := jwt.MapClaims{
		"iss":   mockServer.server.URL,
		"sub":   "user-123",
		"email": "admin@example.com",
		"name":  "Admin User",
		"role":  "admin",
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	}
	token, err := generateTestJWT(testPrivateKey, claims, testKID)
	require.NoError(t, err)

	body, err := json.Marshal(map[string]string{"token": token})
	require.NoError(t, err)

	loginReq := httptest.NewRequest(http.MethodPost, "/auth/token-login", bytes.NewReader(body))
	loginRec := httptest.NewRecorder()

	api.HandleTokenLogin(loginRec, loginReq)

	require.Equal(t, http.StatusOK, loginRec.Code)
	loginCookies := loginRec.Result().Cookies()
	require.NotEmpty(t, loginCookies)

	var tokenCookie *http.Cookie
	for _, cookie := range loginCookies {
		if cookie.Name == SessionCookieName {
			tokenCookie = cookie
			break
		}
	}
	require.NotNil(t, tokenCookie)

	sessionReq := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	sessionReq.AddCookie(tokenCookie)
	sessionRec := httptest.NewRecorder()

	api.HandleSession(sessionRec, sessionReq)

	require.Equal(t, http.StatusOK, sessionRec.Code)
	sessionData := decodeResponseData(t, sessionRec)

	assert.Equal(t, true, sessionData["auth_enabled"])
	assert.Equal(t, true, sessionData["authenticated"])
	assert.Equal(t, "token", sessionData["mode"])

	user, ok := sessionData["user"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "admin@example.com", user["email"])
	assert.Equal(t, "Admin User", user["name"])
	assert.Equal(t, true, user["is_admin"])
}

func TestAuthAPIHandleSessionPrefersOIDCSession(t *testing.T) {
	store := NewSessionStore(time.Hour)
	sessionID, err := store.Create(&SessionData{
		UserID:  "oidc-user",
		Email:   "oidc@example.com",
		Name:    "OIDC User",
		IsAdmin: true,
	})
	require.NoError(t, err)

	oidcClient := &OIDCClient{sessionStore: store}
	api := NewAPI(true, true, nil, oidcClient, AdminConfig{})

	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	req.AddCookie(&http.Cookie{Name: OIDCSessionCookieName, Value: sessionID})
	rec := httptest.NewRecorder()

	api.HandleSession(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	data := decodeResponseData(t, rec)

	assert.Equal(t, true, data["authenticated"])
	assert.Equal(t, "oidc", data["mode"])

	user, ok := data["user"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "oidc@example.com", user["email"])
	assert.Equal(t, true, user["is_admin"])
}

func decodeResponseData(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()

	var payload struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}

	require.NoError(t, json.NewDecoder(rec.Body).Decode(&payload))
	require.True(t, payload.Success)
	require.NotNil(t, payload.Data)

	return payload.Data
}
