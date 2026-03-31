package server

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/sony/gobreaker"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/providers"
)

// TokenLimits defines per-request token limits enforced by the gateway.
type TokenLimits struct {
	MaxPromptTokens int
	MaxOutputTokens int
}

// GatewayServer hosts the Open Responses API for the gateway.
type GatewayServer struct {
	registry       providers.ProviderRegistry
	convs          conversation.Store
	logger         *slog.Logger
	tokenLimits    TokenLimits
	storeByDefault bool
	adminConfig    auth.AdminConfig
}

// New creates a GatewayServer bound to the provider registry.
func New(registry providers.ProviderRegistry, convs conversation.Store, logger *slog.Logger, opts ...Option) *GatewayServer {
	s := &GatewayServer{
		registry:       registry,
		convs:          convs,
		logger:         logger,
		storeByDefault: false,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Option configures optional GatewayServer behaviour.
type Option func(*GatewayServer)

// WithAdminConfig attaches the admin authorization config used for
// conversation ownership overrides.
func WithAdminConfig(cfg auth.AdminConfig) Option {
	return func(s *GatewayServer) {
		s.adminConfig = cfg
	}
}

// SetStoreByDefault configures whether conversations are stored when the
// client does not explicitly set the "store" field.
func (s *GatewayServer) SetStoreByDefault(v bool) {
	s.storeByDefault = v
}

// shouldStore determines whether the conversation should be persisted based
// on the request's store field and the server-level default policy.
func (s *GatewayServer) shouldStore(req *api.ResponseRequest) bool {
	if req.Store != nil {
		return *req.Store
	}
	return s.storeByDefault
}

// SetTokenLimits configures per-request token limits.
func (s *GatewayServer) SetTokenLimits(limits TokenLimits) {
	s.tokenLimits = limits
}

// isCircuitBreakerError checks if the error is from a circuit breaker.
func isCircuitBreakerError(err error) bool {
	return errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)
}

// RegisterRoutes wires the HTTP handlers onto the provided mux.
func (s *GatewayServer) RegisterRoutes(mux *http.ServeMux) {
	s.RegisterAPIRoutes(mux)
	s.RegisterPublicRoutes(mux)
}

// RegisterAPIRoutes wires the authenticated API handlers onto the provided mux.
func (s *GatewayServer) RegisterAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/responses", s.handleResponses)
	mux.HandleFunc("/v1/responses/", s.handleResponseByID)
	mux.HandleFunc("/v1/models", s.handleModels)
}

// RegisterAdminAPIRoutes registers the core API handlers under /api/v1/ so that
// the embedded admin UI (session-based auth) can reach them without a JWT token.
func (s *GatewayServer) RegisterAdminAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/responses", s.handleResponses)
	mux.HandleFunc("/api/v1/responses/", s.handleResponseByID)
	mux.HandleFunc("/api/v1/models", s.handleModels)
}

// RegisterPublicRoutes wires the unauthenticated probe handlers onto the provided mux.
func (s *GatewayServer) RegisterPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
}
