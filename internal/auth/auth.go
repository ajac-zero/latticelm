package auth

import (
	"context"
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
type Config struct {
	Enabled  bool   `yaml:"enabled"`
	Issuer   string `yaml:"issuer"`   // e.g., "https://accounts.google.com"
	Audience string `yaml:"audience"` // e.g., your client ID
}

// AdminConfig holds authorization settings for admin-only routes.
type AdminConfig struct {
	Enabled       bool
	Claim         string
	AllowedValues []string
}

// Middleware provides JWT validation middleware.
type Middleware struct {
	cfg    Config
	keys   map[string]*rsa.PublicKey
	mu     sync.RWMutex
	client *http.Client
	logger *slog.Logger
}

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
		keys:   make(map[string]*rsa.PublicKey),
		client: &http.Client{Timeout: 10 * time.Second},
		logger: logger,
	}

	// Fetch JWKS on startup
	if err := m.refreshJWKS(); err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	// Refresh JWKS periodically
	go m.periodicRefresh()

	return m, nil
}

// Handler wraps an HTTP handler with authentication.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			m.logger.WarnContext(r.Context(), "auth failed: missing authorization header",
				logger.LogAttrsWithTrace(r.Context(),
					slog.String("request_id", logger.FromContext(r.Context())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)...,
			)
			writeUnauthorized(w)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			m.logger.WarnContext(r.Context(), "auth failed: invalid authorization header format",
				logger.LogAttrsWithTrace(r.Context(),
					slog.String("request_id", logger.FromContext(r.Context())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)...,
			)
			writeUnauthorized(w)
			return
		}

		tokenString := parts[1]

		// Validate token
		claims, err := m.validateToken(tokenString)
		if err != nil {
			m.logger.WarnContext(r.Context(), "auth failed: token validation failed",
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

		// Add claims and typed principal to context
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		ctx = ContextWithPrincipal(ctx, PrincipalFromClaims(claims))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// writeUnauthorized writes a generic 401 Unauthorized JSON response without
// exposing any internal validation details to the client.
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `{"error":{"message":"Unauthorized"}}`)
}

type contextKey string

const claimsKey contextKey = "jwt_claims"

// ClaimsContextKey returns the context key used for JWT claims.
// This allows other packages (e.g., rate limiting) to read claims from context.
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

		claims, ok := GetClaims(r.Context())
		if !ok || !hasAdminAccess(claims, m.cfg) {
			http.Error(w, "admin access required", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) validateToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Get key ID from token header
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}

		// Get public key
		m.mu.RLock()
		key, exists := m.keys[kid]
		m.mu.RUnlock()

		if !exists {
			// Try refreshing JWKS
			if err := m.refreshJWKS(); err != nil {
				return nil, fmt.Errorf("failed to refresh JWKS: %w", err)
			}

			m.mu.RLock()
			key, exists = m.keys[kid]
			m.mu.RUnlock()

			if !exists {
				return nil, fmt.Errorf("unknown key ID: %s", kid)
			}
		}

		return key, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate issuer
	if iss, ok := claims["iss"].(string); !ok || iss != m.cfg.Issuer {
		return nil, fmt.Errorf("invalid issuer: %s", iss)
	}

	// Validate audience if configured
	if m.cfg.Audience != "" {
		aud, ok := claims["aud"].(string)
		if !ok {
			// aud might be an array
			audArray, ok := claims["aud"].([]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid audience format")
			}
			found := false
			for _, a := range audArray {
				if audStr, ok := a.(string); ok && audStr == m.cfg.Audience {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("audience not matched")
			}
		} else if aud != m.cfg.Audience {
			return nil, fmt.Errorf("invalid audience: %s", aud)
		}
	}

	return claims, nil
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

func (m *Middleware) refreshJWKS() error {
	jwksURL := strings.TrimSuffix(m.cfg.Issuer, "/") + "/.well-known/openid-configuration"

	// Fetch OIDC discovery document
	resp, err := m.client.Get(jwksURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var oidcConfig struct {
		JwksURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oidcConfig); err != nil {
		return err
	}

	// Fetch JWKS
	resp, err = m.client.Get(oidcConfig.JwksURI)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return err
	}

	// Parse keys
	newKeys := make(map[string]*rsa.PublicKey)
	for _, key := range jwks.Keys {
		if key.Kty != "RSA" || key.Use != "sig" {
			continue
		}

		nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			continue
		}

		eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			continue
		}

		pubKey := &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}

		newKeys[key.Kid] = pubKey
	}

	m.mu.Lock()
	m.keys = newKeys
	m.mu.Unlock()

	return nil
}

func (m *Middleware) periodicRefresh() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		if err := m.refreshJWKS(); err != nil {
			m.logger.Error("failed to refresh JWKS",
				slog.String("issuer", m.cfg.Issuer),
				slog.String("error", err.Error()),
			)
		} else {
			m.logger.Debug("successfully refreshed JWKS",
				slog.String("issuer", m.cfg.Issuer),
			)
		}
	}
}
