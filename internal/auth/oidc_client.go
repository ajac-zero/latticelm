package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ajac-zero/latticelm/internal/users"
	"github.com/golang-jwt/jwt/v5"
)

const (
	oidcStateCookieName    = "oidc_state"
	oidcVerifierCookieName = "oidc_verifier"

	// Context keys for storing user information
	userIDKey  contextKey = "user_id"
	isAdminKey contextKey = "is_admin"
)

// OIDCClientConfig holds OIDC client configuration.
type OIDCClientConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	AdminEmail   string // Optional: auto-promote this email to admin on first login
}

// OIDCClient handles OIDC authorization code flow.
type OIDCClient struct {
	cfg          OIDCClientConfig
	client       *http.Client
	logger       *slog.Logger
	sessionStore *SessionStore
	userStore    *users.Store

	// OIDC discovery endpoints
	authorizationEndpoint string
	tokenEndpoint         string
	userinfoEndpoint      string
}

// NewOIDCClient creates a new OIDC client.
func NewOIDCClient(cfg OIDCClientConfig, sessionStore *SessionStore, userStore *users.Store, logger *slog.Logger) (*OIDCClient, error) {
	client := &OIDCClient{
		cfg:          cfg,
		client:       &http.Client{Timeout: 10 * time.Second},
		logger:       logger,
		sessionStore: sessionStore,
		userStore:    userStore,
	}

	// Discover OIDC endpoints
	if err := client.discover(); err != nil {
		return nil, fmt.Errorf("OIDC discovery failed: %w", err)
	}

	return client, nil
}

// discover fetches OIDC provider configuration.
func (c *OIDCClient) discover() error {
	discoveryURL := strings.TrimSuffix(c.cfg.Issuer, "/") + "/.well-known/openid-configuration"

	resp, err := c.client.Get(discoveryURL)
	if err != nil {
		return fmt.Errorf("discovery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery endpoint returned HTTP %d", resp.StatusCode)
	}

	var config struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		UserinfoEndpoint      string `json:"userinfo_endpoint"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return fmt.Errorf("failed to decode discovery document: %w", err)
	}

	c.authorizationEndpoint = config.AuthorizationEndpoint
	c.tokenEndpoint = config.TokenEndpoint
	c.userinfoEndpoint = config.UserinfoEndpoint

	c.logger.Info("OIDC endpoints discovered",
		slog.String("auth_endpoint", c.authorizationEndpoint),
		slog.String("token_endpoint", c.tokenEndpoint),
	)

	return nil
}

// HandleLogin initiates the OIDC authorization code flow.
func (c *OIDCClient) HandleLogin(w http.ResponseWriter, r *http.Request) {
	authURL, err := c.prepareAuthorizationURL(w, r)
	if err != nil {
		c.logger.Error("failed to initiate OIDC login", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleOIDCLogin starts OIDC login for API clients.
// POST returns a JSON payload with the authorization URL while also setting
// state/verifier cookies. GET is kept as a convenience for direct browser use.
func (c *OIDCClient) HandleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c.HandleLogin(w, r)
		return
	case http.MethodPost:
		authURL, err := c.prepareAuthorizationURL(w, r)
		if err != nil {
			c.logger.Error("failed to initiate OIDC login", slog.String("error", err.Error()))
			writeJSONError(w, "Authentication failed", http.StatusInternalServerError)
			return
		}

		writeJSONSuccess(w, map[string]string{"authorization_url": authURL})
		return
	default:
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func (c *OIDCClient) prepareAuthorizationURL(w http.ResponseWriter, r *http.Request) (string, error) {
	state, err := generateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	codeVerifier, err := generateRandomString(43)
	if err != nil {
		return "", fmt.Errorf("generate code verifier: %w", err)
	}

	codeChallenge := createCodeChallenge(codeVerifier)

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	cookieDomain := ""
	if strings.Contains(r.Host, "localhost") {
		cookieDomain = "localhost"
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    state,
		Domain:   cookieDomain,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     oidcVerifierCookieName,
		Value:    codeVerifier,
		Domain:   cookieDomain,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	authURL := c.authorizationEndpoint + "?" + url.Values{
		"client_id":             {c.cfg.ClientID},
		"response_type":         {"code"},
		"scope":                 {"openid email profile"},
		"redirect_uri":          {c.cfg.RedirectURI},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	return authURL, nil
}

// wantsJSON checks if the client prefers JSON response
func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

// HandleCallback handles the OIDC callback after user authentication.
func (c *OIDCClient) HandleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get authorization code first - log it for debugging
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	c.logger.Info("OIDC callback received",
		slog.Bool("has_code", code != ""),
		slog.Bool("has_state", state != ""),
		slog.String("url", r.URL.String()),
	)

	if code == "" {
		c.logger.Warn("missing authorization code",
			slog.String("query_string", r.URL.RawQuery),
		)
		http.Error(w, "Missing code", http.StatusBadRequest)
		return
	}

	// Verify state parameter
	stateCookie, err := r.Cookie(oidcStateCookieName)
	if err != nil {
		c.logger.Warn("missing state cookie",
			slog.String("error", err.Error()),
			slog.String("cookies", fmt.Sprintf("%v", r.Cookies())),
		)
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	if state != stateCookie.Value {
		c.logger.Warn("state mismatch", slog.String("expected", stateCookie.Value), slog.String("got", state))
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	// Get code verifier
	verifierCookie, err := r.Cookie(oidcVerifierCookieName)
	if err != nil {
		c.logger.Warn("missing verifier cookie", slog.String("error", err.Error()))
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Clear temporary cookies
	cookieDomain := ""
	if strings.Contains(r.Host, "localhost") {
		cookieDomain = "localhost"
	}
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookieName, Domain: cookieDomain, MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: oidcVerifierCookieName, Domain: cookieDomain, MaxAge: -1, Path: "/"})

	// Exchange code for tokens
	tokens, err := c.exchangeCode(ctx, code, verifierCookie.Value)
	if err != nil {
		c.logger.Error("code exchange failed", slog.String("error", err.Error()))
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	// Parse ID token to get user info
	claims, err := c.parseIDToken(tokens.IDToken)
	if err != nil {
		c.logger.Error("failed to parse ID token", slog.String("error", err.Error()))
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	// Auto-provision or get existing user from database
	user, err := c.userStore.GetOrCreate(ctx,
		getClaimString(claims, "iss"),    // OIDC issuer
		getClaimString(claims, "sub"),    // OIDC subject
		getClaimString(claims, "email"),  // Email from OIDC
		getClaimString(claims, "name"),   // Name from OIDC
	)
	if err != nil {
		c.logger.Error("failed to provision user", slog.String("error", err.Error()))
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	// Auto-promote to admin if email matches configured admin_email
	if c.cfg.AdminEmail != "" && user.Email == c.cfg.AdminEmail && user.Role == users.RoleUser {
		c.logger.Info("auto-promoting user to admin",
			slog.String("user_id", user.ID),
			slog.String("email", user.Email),
		)
		if err := c.userStore.UpdateRole(ctx, user.ID, users.RoleAdmin); err != nil {
			c.logger.Error("failed to promote user to admin", slog.String("error", err.Error()))
			// Don't fail the login, just log the error
		} else {
			user.Role = users.RoleAdmin // Update in-memory object
			c.logger.Info("user promoted to admin",
				slog.String("user_id", user.ID),
				slog.String("email", user.Email),
			)
		}
	}

	// Check if user account is active
	if !user.IsActive() {
		c.logger.Warn("inactive user attempted login",
			slog.String("user_id", user.ID),
			slog.String("status", string(user.Status)),
		)
		http.Error(w, "Account is not active", http.StatusForbidden)
		return
	}

	// Create session with application user data
	sessionData := &SessionData{
		UserID:       user.ID,           // Our database ID (not OIDC sub)
		Email:        user.Email,
		Name:         user.Name,
		IDToken:      tokens.IDToken,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		IsAdmin:      user.IsAdmin(),    // From database users.role column
	}

	sessionID, err := c.sessionStore.Create(sessionData)
	if err != nil {
		c.logger.Error("failed to create session", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	// In dev mode, set Domain=localhost so the cookie works across ports
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	// cookieDomain already declared above when clearing temporary cookies

	http.SetCookie(w, &http.Cookie{
		Name:     OIDCSessionCookieName,
		Value:    sessionID,
		Domain:   cookieDomain,
		Path:     "/",
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	c.logger.Info("user authenticated",
		slog.String("user_id", sessionData.UserID),
		slog.String("email", sessionData.Email),
		slog.Bool("is_admin", sessionData.IsAdmin),
	)

	// Redirect to home page
	// Check for FRONTEND_URL environment variable for dev mode
	// In dev: set FRONTEND_URL=http://localhost:5173
	// In production: leave unset to use relative redirect
	redirectURL := os.Getenv("FRONTEND_URL")
	if redirectURL == "" {
		redirectURL = "/" // Relative redirect for production (embedded UI)
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// HandleLogout clears the user session.
func (c *OIDCClient) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	c.clearSession(w, r)

	if r.Method == http.MethodPost || wantsJSON(r) {
		writeJSONSuccess(w, map[string]string{"message": "logged out"})
		return
	}

	// Redirect to login for browser GET callers.
	redirectURL := os.Getenv("FRONTEND_URL")
	if redirectURL == "" {
		redirectURL = "/auth/login"
	} else {
		redirectURL = redirectURL + "/auth/login"
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (c *OIDCClient) clearSession(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	cookie, err := r.Cookie(OIDCSessionCookieName)
	if err == nil {
		c.sessionStore.Delete(cookie.Value)
	}

	// Clear session cookie with same domain strategy as login
	cookieDomain := ""
	if strings.Contains(r.Host, "localhost") {
		cookieDomain = "localhost"
	}

	http.SetCookie(w, &http.Cookie{
		Name:     OIDCSessionCookieName,
		Value:    "",
		Domain:   cookieDomain,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (c *OIDCClient) getSession(r *http.Request) (*SessionData, bool) {
	cookie, err := r.Cookie(OIDCSessionCookieName)
	if err != nil {
		return nil, false
	}

	return c.sessionStore.Get(cookie.Value)
}

// HandleUser returns current user information.
func (c *OIDCClient) HandleUser(w http.ResponseWriter, r *http.Request) {
	session, exists := c.getSession(r)
	if !exists {
		writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Return user info in standard API response format
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"email":    session.Email,
			"name":     session.Name,
			"is_admin": session.IsAdmin,
		},
	}); err != nil {
		c.logger.Error("failed to encode user info", "error", err)
	}
}

func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error": map[string]string{
			"message": message,
		},
	})
}

func writeJSONSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    data,
	})
}

// SessionMiddleware checks for a valid session cookie.
func (c *OIDCClient) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth endpoints
		if strings.HasPrefix(r.URL.Path, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		// Validate session
		session, exists := c.getSession(r)
		if !exists {
			// For API requests, return 401 instead of redirecting
			if isAPIRequest(r) {
				writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			// For page requests, redirect to login
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		// Add session data to context
		claims := jwt.MapClaims{
			"sub":   session.UserID,
			"email": session.Email,
			"name":  session.Name,
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		ctx = ContextWithPrincipal(ctx, PrincipalFromClaims(claims))
		// Add user_id for convenience (used by /api/users/me)
		ctx = context.WithValue(ctx, userIDKey, session.UserID)
		// Add is_admin for authorization checks
		ctx = context.WithValue(ctx, isAdminKey, session.IsAdmin)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isAPIRequest determines if a request is for an API endpoint (should return JSON)
// versus a page request (should redirect on auth failure).
func isAPIRequest(r *http.Request) bool {
	// Check if path starts with /api/ or /v1/
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/v1/") {
		return true
	}

	// Check Accept header - if client prefers JSON, treat as API request
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

// exchangeCode exchanges authorization code for tokens.
func (c *OIDCClient) exchangeCode(ctx context.Context, code, codeVerifier string) (*tokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.cfg.RedirectURI},
		"client_id":     {c.cfg.ClientID},
		"code_verifier": {codeVerifier},
	}

	if c.cfg.ClientSecret != "" {
		data.Set("client_secret", c.cfg.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokens tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, err
	}

	return &tokens, nil
}

// parseIDToken parses and validates an ID token (basic validation, not signature check).
func (c *OIDCClient) parseIDToken(idToken string) (jwt.MapClaims, error) {
	// Parse without verification (we trust the token from the IdP's token endpoint)
	token, _, err := new(jwt.Parser).ParseUnverified(idToken, jwt.MapClaims{})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}

	return claims, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:length], nil
}

// createCodeChallenge creates a PKCE code challenge using S256 method.
// Per RFC 7636: code_challenge = BASE64URL(SHA256(ASCII(code_verifier)))
func createCodeChallenge(verifier string) string {
	h := sha256.New()
	h.Write([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func getClaimString(claims jwt.MapClaims, key string) string {
	if val, ok := claims[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
