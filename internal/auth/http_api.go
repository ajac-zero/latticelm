package auth

import (
	"encoding/json"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

// API exposes a stable HTTP contract for UI authentication.
type API struct {
	authEnabled bool
	oidcEnabled bool
	tokenAuth   *Middleware
	oidcClient  *OIDCClient
	adminCfg    AdminConfig
}

// SessionResponse describes auth status for the current browser session.
type SessionResponse struct {
	AuthEnabled   bool         `json:"auth_enabled"`
	OIDCEnabled   bool         `json:"oidc_enabled"`
	Authenticated bool         `json:"authenticated"`
	Mode          string       `json:"mode"`
	User          *SessionUser `json:"user,omitempty"`
}

// SessionUser contains the authenticated user summary for UI consumption.
type SessionUser struct {
	ID      string `json:"id,omitempty"`
	Email   string `json:"email,omitempty"`
	Name    string `json:"name,omitempty"`
	IsAdmin bool   `json:"is_admin"`
}

// NewAPI creates auth API handlers used by the admin UI.
func NewAPI(authEnabled, oidcEnabled bool, tokenAuth *Middleware, oidcClient *OIDCClient, adminCfg AdminConfig) *API {
	return &API{
		authEnabled: authEnabled,
		oidcEnabled: oidcEnabled,
		tokenAuth:   tokenAuth,
		oidcClient:  oidcClient,
		adminCfg:    adminCfg,
	}
}

// RegisterRoutes wires the auth API endpoints.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/auth/session", a.HandleSession)
	mux.HandleFunc("/auth/token-login", a.HandleTokenLogin)
	mux.HandleFunc("/auth/logout", a.HandleLogout)

	if a.oidcClient != nil {
		mux.HandleFunc("/auth/oidc/login", a.oidcClient.HandleOIDCLogin)
		mux.HandleFunc("/auth/callback", a.oidcClient.HandleCallback)
	}
}

// HandleSession returns authentication status and user details for the caller.
func (a *API) HandleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := SessionResponse{
		AuthEnabled:   a.authEnabled,
		OIDCEnabled:   a.oidcEnabled,
		Authenticated: false,
		Mode:          a.defaultMode(),
	}

	if !a.authEnabled {
		writeJSONSuccess(w, response)
		return
	}

	if a.oidcClient != nil {
		if session, ok := a.oidcClient.getSession(r); ok {
			response.Authenticated = true
			response.Mode = "oidc"
			response.User = &SessionUser{
				ID:      session.UserID,
				Email:   session.Email,
				Name:    session.Name,
				IsAdmin: session.IsAdmin,
			}
			writeJSONSuccess(w, response)
			return
		}
	}

	claims, ok := a.tokenClaimsFromCookie(r)
	if ok {
		response.Authenticated = true
		response.Mode = "token"
		response.User = &SessionUser{
			ID:      getClaimString(claims, "sub"),
			Email:   getClaimString(claims, "email"),
			Name:    getClaimString(claims, "name"),
			IsAdmin: hasAdminAccess(claims, a.adminCfg),
		}
	}

	writeJSONSuccess(w, response)
}

// HandleTokenLogin validates a JWT and stores it as a browser session cookie.
func (a *API) HandleTokenLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !a.authEnabled {
		writeJSONError(w, "Authentication is disabled", http.StatusBadRequest)
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeJSONError(w, "Token is required", http.StatusBadRequest)
		return
	}

	if a.tokenAuth == nil {
		writeJSONError(w, "Authentication is not configured", http.StatusServiceUnavailable)
		return
	}

	claims, err := a.tokenAuth.Validate(req.Token)
	if err != nil {
		writeJSONError(w, "Invalid or expired token", http.StatusUnauthorized)
		return
	}

	if a.oidcClient != nil {
		a.oidcClient.clearSession(w, r)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    req.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
	})

	writeJSONSuccess(w, SessionResponse{
		AuthEnabled:   a.authEnabled,
		OIDCEnabled:   a.oidcEnabled,
		Authenticated: true,
		Mode:          "token",
		User: &SessionUser{
			ID:      getClaimString(claims, "sub"),
			Email:   getClaimString(claims, "email"),
			Name:    getClaimString(claims, "name"),
			IsAdmin: hasAdminAccess(claims, a.adminCfg),
		},
	})
}

// HandleLogout clears both token and OIDC session cookies.
func (a *API) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	if a.oidcClient != nil {
		a.oidcClient.clearSession(w, r)
	}

	writeJSONSuccess(w, map[string]string{"message": "logged out"})
}

func (a *API) tokenClaimsFromCookie(r *http.Request) (jwt.MapClaims, bool) {
	if a.tokenAuth == nil {
		return nil, false
	}

	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}

	claims, err := a.tokenAuth.Validate(cookie.Value)
	if err != nil {
		return nil, false
	}

	return claims, true
}

func (a *API) defaultMode() string {
	if !a.authEnabled {
		return "none"
	}
	if a.oidcEnabled {
		return "oidc"
	}
	return "token"
}

func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
