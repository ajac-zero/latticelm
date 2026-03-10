package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// OIDCClientConfig holds OIDC client configuration.
type OIDCClientConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// OIDCClient handles OIDC authorization code flow.
type OIDCClient struct {
	cfg          OIDCClientConfig
	client       *http.Client
	logger       *slog.Logger
	sessionStore *SessionStore
	adminCfg     AdminConfig

	// OIDC discovery endpoints
	authorizationEndpoint string
	tokenEndpoint         string
	userinfoEndpoint      string
}

// NewOIDCClient creates a new OIDC client.
func NewOIDCClient(cfg OIDCClientConfig, sessionStore *SessionStore, adminCfg AdminConfig, logger *slog.Logger) (*OIDCClient, error) {
	client := &OIDCClient{
		cfg:          cfg,
		client:       &http.Client{Timeout: 10 * time.Second},
		logger:       logger,
		sessionStore: sessionStore,
		adminCfg:     adminCfg,
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
	// Generate state parameter for CSRF protection
	state, err := generateRandomString(32)
	if err != nil {
		c.logger.Error("failed to generate state", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Generate PKCE code verifier and challenge
	codeVerifier, err := generateRandomString(43)
	if err != nil {
		c.logger.Error("failed to generate code verifier", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Store state and code_verifier in a temporary session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_verifier",
		Value:    codeVerifier,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Build authorization URL
	authURL := c.authorizationEndpoint + "?" + url.Values{
		"client_id":             {c.cfg.ClientID},
		"response_type":         {"code"},
		"scope":                 {"openid email profile"},
		"redirect_uri":          {c.cfg.RedirectURI},
		"state":                 {state},
		"code_challenge":        {codeVerifier}, // For PKCE, we're using plain (not S256) for simplicity
		"code_challenge_method": {"plain"},
	}.Encode()

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback handles the OIDC callback after user authentication.
func (c *OIDCClient) HandleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Verify state parameter
	stateCookie, err := r.Cookie("oidc_state")
	if err != nil {
		c.logger.Warn("missing state cookie", slog.String("error", err.Error()))
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	if state != stateCookie.Value {
		c.logger.Warn("state mismatch", slog.String("expected", stateCookie.Value), slog.String("got", state))
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	// Get code verifier
	verifierCookie, err := r.Cookie("oidc_verifier")
	if err != nil {
		c.logger.Warn("missing verifier cookie", slog.String("error", err.Error()))
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Clear temporary cookies
	http.SetCookie(w, &http.Cookie{Name: "oidc_state", MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "oidc_verifier", MaxAge: -1, Path: "/"})

	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		c.logger.Warn("missing authorization code")
		http.Error(w, "Missing code", http.StatusBadRequest)
		return
	}

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

	// Check if user is admin
	isAdmin := hasAdminAccess(claims, c.adminCfg)

	// Create session
	sessionData := &SessionData{
		UserID:       getClaimString(claims, "sub"),
		Email:        getClaimString(claims, "email"),
		Name:         getClaimString(claims, "name"),
		IDToken:      tokens.IDToken,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		IsAdmin:      isAdmin,
	}

	sessionID, err := c.sessionStore.Create(sessionData)
	if err != nil {
		c.logger.Error("failed to create session", slog.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	c.logger.Info("user authenticated",
		slog.String("email", sessionData.Email),
		slog.Bool("is_admin", isAdmin),
	)

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleLogout clears the user session.
func (c *OIDCClient) HandleLogout(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	cookie, err := r.Cookie("session")
	if err == nil {
		c.sessionStore.Delete(cookie.Value)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to login
	http.Redirect(w, r, "/auth/login", http.StatusFound)
}

// HandleUser returns current user information.
func (c *OIDCClient) HandleUser(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get session data
	session, exists := c.sessionStore.Get(cookie.Value)
	if !exists {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Return user info
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"email":    session.Email,
		"name":     session.Name,
		"is_admin": session.IsAdmin,
	}); err != nil {
		c.logger.Error("failed to encode user info", "error", err)
	}
}

// SessionMiddleware checks for a valid session cookie.
func (c *OIDCClient) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth endpoints
		if strings.HasPrefix(r.URL.Path, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		// Get session cookie
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		// Validate session
		session, exists := c.sessionStore.Get(cookie.Value)
		if !exists {
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

		next.ServeHTTP(w, r.WithContext(ctx))
	})
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

func getClaimString(claims jwt.MapClaims, key string) string {
	if val, ok := claims[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
