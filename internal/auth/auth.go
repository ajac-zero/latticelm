package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ajac-zero/latticelm/internal/logger"
)

// Config holds OIDC authentication configuration.
//
// Security vs. availability tradeoffs:
//   - ClockSkew: a non-zero value accepts tokens slightly outside their exp/nbf window,
//     compensating for clock drift between issuer and gateway. Recommended: ≤ 60 s.
//   - StaleTTL: during IdP JWKS outages, previously fetched keys continue to be accepted
//     for up to this duration. A zero value means stale keys are used indefinitely, which
//     maximises availability but extends the window in which rotated or revoked keys
//     remain accepted. Set a non-zero value (e.g., 15 m) to bound that window at the
//     cost of hard failures for active tokens if the IdP outage is prolonged.
type Config struct {
	Enabled      bool
	Issuer       string        // e.g., "https://accounts.google.com"
	DiscoveryURL string        // optional; overrides the derived {Issuer}/.well-known/openid-configuration URL
	Audiences    []string      // e.g., your client ID(s)
	ClockSkew    time.Duration // allowance for clock drift; default 0
	StaleTTL     time.Duration // stale-key acceptance window; 0 = unlimited
	AdminClaim   string        // optional custom claim name whose values are extracted as roles (e.g., "permissions")
}

// AdminConfig holds authorization settings for admin-only routes.
type AdminConfig struct {
	Enabled       bool
	Claim         string
	AllowedValues []string
}

// Middleware provides JWT validation middleware.
type Middleware struct {
	cfg            Config
	keys           map[string]interface{} // kid → *rsa.PublicKey or *ecdsa.PublicKey
	lastFetchedAt  time.Time              // time of the last successful JWKS fetch
	mu             sync.RWMutex
	client         *http.Client
	logger         *slog.Logger
	oidcClient     *OIDCClient // optional OIDC client for session-based auth (enterprise-grade)
	authenticators []Authenticator

	// refreshMu serialises on-demand JWKS refreshes triggered by unknown key IDs to
	// prevent multiple concurrent requests from hammering the IdP simultaneously.
	refreshMu     sync.Mutex
	lastRefreshAt time.Time
}

const (
	// refreshCooldown is the minimum interval between on-demand JWKS refreshes.
	refreshCooldown = 30 * time.Second
	// normalRefreshInterval is the steady-state periodic refresh cadence.
	normalRefreshInterval = time.Hour
	// initialRetryInterval is the first backoff interval after a failed periodic refresh.
	initialRetryInterval = 5 * time.Minute
	// maxRetryInterval caps the exponential backoff for periodic refreshes.
	maxRetryInterval = 30 * time.Minute
)

// New creates an authentication middleware.
func New(cfg Config, logger *slog.Logger) (*Middleware, error) {
	if !cfg.Enabled {
		return &Middleware{cfg: cfg, logger: logger}, nil
	}

	if cfg.Issuer == "" {
		return nil, fmt.Errorf("auth enabled but issuer not configured")
	}

	m := &Middleware{
		cfg:    cfg,
		keys:   make(map[string]interface{}),
		client: &http.Client{Timeout: 10 * time.Second},
		logger: logger,
	}

	if err := m.refreshJWKS(); err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	// Register built-in authenticators: JWT Bearer → session cookie.
	m.authenticators = []Authenticator{
		&jwtAuthenticator{mw: m, adminClaim: cfg.AdminClaim},
		&sessionCookieAuthenticator{mw: m},
	}

	go m.periodicRefresh()

	return m, nil
}

// PrependAuthenticator inserts an authenticator at the front of the chain so
// it is evaluated before the built-in JWT and session authenticators.
func (m *Middleware) PrependAuthenticator(a Authenticator) {
	m.authenticators = append([]Authenticator{a}, m.authenticators...)
}

// SetOIDCClient allows the middleware to accept OIDC session cookies as an alternative
// to JWT Bearer tokens. The ID token is never exposed to the frontend (enterprise-grade).
func (m *Middleware) SetOIDCClient(oidcClient *OIDCClient) {
	m.oidcClient = oidcClient
	// Replace the generic session-cookie authenticator with one that also
	// knows about OIDC server-side sessions.
	for i, a := range m.authenticators {
		if _, ok := a.(*sessionCookieAuthenticator); ok {
			m.authenticators[i] = &sessionCookieAuthenticator{mw: m}
			return
		}
	}
	m.authenticators = append(m.authenticators, &sessionCookieAuthenticator{mw: m})
}

// SessionCookieName is the name of the HttpOnly session cookie used for admin UI authentication.
const SessionCookieName = "lattice_session"

// Handler wraps an HTTP handler with authentication.
//
// It iterates the registered authenticator chain in order. The first
// authenticator that returns a non-nil Principal wins; the first that
// returns a non-nil error causes the request to be rejected.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.Enabled && len(m.authenticators) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		for _, authn := range m.authenticators {
			principal, err := authn.Authenticate(r)
			if err != nil {
				m.logger.WarnContext(r.Context(), "auth failed: credential rejected",
					logger.LogAttrsWithTrace(r.Context(),
						slog.String("request_id", logger.FromContext(r.Context())),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.String("error", err.Error()),
					)...,
				)
				writeUnauthorized(w)
				return
			}
			if principal != nil {
				ctx := ContextWithPrincipal(r.Context(), principal)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		m.logger.WarnContext(r.Context(), "auth failed: no valid credentials",
			logger.LogAttrsWithTrace(r.Context(),
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)...,
		)
		writeUnauthorized(w)
	})
}

// jwtAuthenticator validates Bearer JWT tokens against the JWKS key set.
type jwtAuthenticator struct {
	mw         *Middleware
	adminClaim string // extra claim name to extract as roles
}

func (a *jwtAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, nil
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return nil, fmt.Errorf("invalid authorization header format")
	}
	tokenString := parts[1]

	// JWTs are three base64url segments separated by dots. Skip tokens
	// that are clearly not JWTs so the next authenticator can try them.
	if strings.Count(tokenString, ".") != 2 {
		return nil, nil
	}

	claims, err := a.mw.validateToken(tokenString)
	if err != nil {
		return nil, err
	}

	var extraClaims []string
	if a.adminClaim != "" {
		extraClaims = append(extraClaims, a.adminClaim)
	}
	return PrincipalFromClaims(claims, extraClaims...), nil
}

// sessionCookieAuthenticator validates session cookies (plain JWT cookie and
// OIDC server-side sessions).
type sessionCookieAuthenticator struct {
	mw *Middleware
}

func (a *sessionCookieAuthenticator) extraClaims() []string {
	if a.mw.cfg.AdminClaim != "" {
		return []string{a.mw.cfg.AdminClaim}
	}
	return nil
}

func (a *sessionCookieAuthenticator) Authenticate(r *http.Request) (*Principal, error) {
	// Check for OIDC server-side session first.
	if a.mw.oidcClient != nil {
		if session, ok := a.mw.oidcClient.getSession(r); ok {
			claims, err := a.mw.validateToken(session.IDToken)
			if err != nil {
				return nil, fmt.Errorf("OIDC session ID token invalid: %w", err)
			}
			return PrincipalFromClaims(claims, a.extraClaims()...), nil
		}
	}

	// Fall back to plain JWT stored in a session cookie.
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, nil
	}
	claims, err := a.mw.validateToken(cookie.Value)
	if err != nil {
		return nil, err
	}
	return PrincipalFromClaims(claims, a.extraClaims()...), nil
}

// writeUnauthorized writes a generic 401 Unauthorized JSON response without
// exposing any internal validation details to the client.
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `{"error":{"message":"Unauthorized"}}`)
}

// Validate validates a JWT token string and returns claims.
// Returns an error if auth is not enabled or the token is invalid.
func (m *Middleware) Validate(tokenString string) (jwt.MapClaims, error) {
	if !m.cfg.Enabled {
		return nil, fmt.Errorf("auth not enabled")
	}
	return m.validateToken(tokenString)
}

type contextKey string

const claimsKey contextKey = "jwt_claims"

// ClaimsContextKey returns the context key used for JWT claims.
func ClaimsContextKey() contextKey {
	return claimsKey
}

// GetClaims extracts JWT claims from request context.
func GetClaims(ctx context.Context) (jwt.MapClaims, bool) {
	claims, ok := ctx.Value(claimsKey).(jwt.MapClaims)
	return claims, ok
}

// AdminMiddleware enforces an admin claim on an already-authenticated request.
type AdminMiddleware struct {
	cfg AdminConfig
}

// NewAdmin creates an admin authorization middleware.
func NewAdmin(cfg AdminConfig) *AdminMiddleware {
	return &AdminMiddleware{cfg: cfg}
}

// Handler wraps an HTTP handler with admin authorization.
func (m *AdminMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		principal := PrincipalFromContext(r.Context())
		if principal == nil || !principal.HasAdminRole(m.cfg) {
			http.Error(w, "admin access required", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) validateToken(tokenString string) (jwt.MapClaims, error) {
	// Enforce stale-TTL: if the last successful JWKS fetch exceeds the configured
	// window, attempt a forced refresh before accepting any token.
	if m.cfg.StaleTTL > 0 {
		m.mu.RLock()
		age := time.Since(m.lastFetchedAt)
		m.mu.RUnlock()

		if age > m.cfg.StaleTTL {
			if err := m.forceRefresh(); err != nil {
				return nil, fmt.Errorf("JWKS keys are stale (%v old) and refresh failed: %w", age.Round(time.Second), err)
			}
		}
	}

	parseOpts := []jwt.ParserOption{
		jwt.WithIssuer(m.cfg.Issuer),
		jwt.WithExpirationRequired(),
	}
	if len(m.cfg.Audiences) > 0 {
		parseOpts = append(parseOpts, jwt.WithAudience(m.cfg.Audiences...))
	}
	if m.cfg.ClockSkew > 0 {
		parseOpts = append(parseOpts, jwt.WithLeeway(m.cfg.ClockSkew))
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Restrict signing algorithms to asymmetric methods only.
		// Symmetric algorithms (HMAC) are rejected because the IdP secret is not shared.
		switch token.Method.(type) {
		case *jwt.SigningMethodRSA, *jwt.SigningMethodRSAPSS, *jwt.SigningMethodECDSA:
			// accepted
		default:
			return nil, fmt.Errorf("unsupported signing algorithm: %v", token.Header["alg"])
		}

		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}

		m.mu.RLock()
		key, exists := m.keys[kid]
		m.mu.RUnlock()

		if !exists {
			// Trigger a rate-limited on-demand JWKS refresh for unknown key IDs.
			// This supports key rotation: the IdP issues tokens with a new kid before
			// the gateway's periodic refresh fires.
			if refreshErr := m.refreshIfCooledDown(); refreshErr != nil {
				m.logger.Warn("on-demand JWKS refresh failed",
					slog.String("kid", kid),
					slog.String("error", refreshErr.Error()),
				)
			}

			m.mu.RLock()
			key, exists = m.keys[kid]
			m.mu.RUnlock()

			if !exists {
				return nil, fmt.Errorf("unknown key ID: %s", kid)
			}
		}

		return key, nil
	}, parseOpts...)

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// refreshIfCooledDown performs a JWKS refresh only when the cooldown window has elapsed,
// preventing thundering-herd refreshes when multiple concurrent requests see an unknown kid.
func (m *Middleware) refreshIfCooledDown() error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()

	if time.Since(m.lastRefreshAt) < refreshCooldown {
		return nil
	}

	err := m.refreshJWKS()
	m.lastRefreshAt = time.Now()
	return err
}

// forceRefresh always performs a JWKS refresh, bypassing the cooldown.
// It is used when the stale-TTL has been exceeded.
func (m *Middleware) forceRefresh() error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()

	err := m.refreshJWKS()
	m.lastRefreshAt = time.Now()
	return err
}

// refreshJWKS fetches the current JWKS from the IdP discovery document and atomically
// replaces the in-memory key set on success. On failure the previous key set is
// preserved so in-flight tokens are not rejected during transient IdP outages.
func (m *Middleware) refreshJWKS() error {
	jwksURL, err := m.discoverJWKSURL()
	if err != nil {
		return err
	}

	resp, err := m.client.Get(jwksURL) //#nosec G704 -- jwksURL is trusted from OIDC discovery
	if err != nil {
		return fmt.Errorf("JWKS fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned HTTP %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Use string `json:"use"`
			// RSA fields
			N string `json:"n"`
			E string `json:"e"`
			// EC fields
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}

	newKeys := make(map[string]interface{})
	for _, k := range jwks.Keys {
		// Only load signature keys; skip encryption keys.
		if k.Use != "sig" {
			continue
		}

		switch k.Kty {
		case "RSA":
			key, err := parseRSAKey(k.N, k.E)
			if err != nil {
				m.logger.Warn("skipping invalid RSA key in JWKS",
					slog.String("kid", k.Kid),
					slog.String("error", err.Error()),
				)
				continue
			}
			newKeys[k.Kid] = key

		case "EC":
			key, err := parseECKey(k.Crv, k.X, k.Y)
			if err != nil {
				m.logger.Warn("skipping invalid EC key in JWKS",
					slog.String("kid", k.Kid),
					slog.String("error", err.Error()),
				)
				continue
			}
			newKeys[k.Kid] = key
		}
	}

	if len(newKeys) == 0 {
		return fmt.Errorf("JWKS contained no usable signing keys")
	}

	m.mu.Lock()
	m.keys = newKeys
	m.lastFetchedAt = time.Now()
	m.mu.Unlock()

	return nil
}

// discoverJWKSURL retrieves the JWKS URI from the OIDC discovery document.
func (m *Middleware) discoverJWKSURL() (string, error) {
	discoveryURL := m.cfg.DiscoveryURL
	if discoveryURL == "" {
		discoveryURL = strings.TrimSuffix(m.cfg.Issuer, "/") + "/.well-known/openid-configuration"
	}

	resp, err := m.client.Get(discoveryURL) //#nosec G704 -- discoveryURL is constructed from trusted config
	if err != nil {
		return "", fmt.Errorf("OIDC discovery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC discovery endpoint returned HTTP %d", resp.StatusCode)
	}

	var oidcConfig struct {
		JwksURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oidcConfig); err != nil {
		return "", fmt.Errorf("failed to decode OIDC discovery document: %w", err)
	}

	if oidcConfig.JwksURI == "" {
		return "", fmt.Errorf("OIDC discovery document missing jwks_uri field")
	}

	return oidcConfig.JwksURI, nil
}

// parseRSAKey builds an RSA public key from base64url-encoded modulus and exponent.
func parseRSAKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("invalid RSA modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("invalid RSA exponent: %w", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}

// parseECKey builds an ECDSA public key from a JWKS EC key entry.
// Supported curves: P-256, P-384, P-521.
func parseECKey(crv, xB64, yB64 string) (*ecdsa.PublicKey, error) {
	var curve elliptic.Curve
	switch crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported EC curve: %s", crv)
	}

	if xB64 == "" || yB64 == "" {
		return nil, fmt.Errorf("EC key missing x or y coordinate")
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(xB64)
	if err != nil {
		return nil, fmt.Errorf("invalid EC x coordinate: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(yB64)
	if err != nil {
		return nil, fmt.Errorf("invalid EC y coordinate: %w", err)
	}

	return &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}

// periodicRefresh refreshes JWKS on a normal schedule, backing off exponentially on
// consecutive failures. Stale keys remain in use during failures so existing tokens
// are not rejected during transient IdP outages.
func (m *Middleware) periodicRefresh() {
	retryInterval := initialRetryInterval
	timer := time.NewTimer(normalRefreshInterval)
	defer timer.Stop()

	for range timer.C {
		if err := m.refreshJWKS(); err != nil {
			m.logger.Error("periodic JWKS refresh failed; retrying with backoff",
				slog.String("issuer", m.cfg.Issuer),
				slog.String("retry_in", retryInterval.String()),
				slog.String("error", err.Error()),
			)
			timer.Reset(retryInterval)
			retryInterval = min(retryInterval*2, maxRetryInterval)
		} else {
			m.logger.Debug("JWKS refreshed successfully",
				slog.String("issuer", m.cfg.Issuer),
			)
			retryInterval = initialRetryInterval
			timer.Reset(normalRefreshInterval)
		}
	}
}

func hasAdminAccess(claims jwt.MapClaims, cfg AdminConfig) bool {
	allowedValues := cfg.AllowedValues
	if len(allowedValues) == 0 {
		allowedValues = []string{"admin"}
	}

	if cfg.Claim != "" {
		return claimHasAllowedValue(claims[cfg.Claim], allowedValues)
	}

	for _, claimName := range []string{"role", "roles", "groups"} {
		if claimHasAllowedValue(claims[claimName], allowedValues) {
			return true
		}
	}

	return false
}

func claimHasAllowedValue(rawValue any, allowedValues []string) bool {
	if rawValue == nil {
		return false
	}

	allowed := make(map[string]struct{}, len(allowedValues))
	for _, value := range allowedValues {
		allowed[value] = struct{}{}
	}

	switch value := rawValue.(type) {
	case string:
		_, ok := allowed[value]
		return ok
	case []string:
		for _, entry := range value {
			if _, ok := allowed[entry]; ok {
				return true
			}
		}
	case []interface{}:
		for _, entry := range value {
			text, ok := entry.(string)
			if !ok {
				continue
			}
			if _, ok := allowed[text]; ok {
				return true
			}
		}
	}

	return false
}
