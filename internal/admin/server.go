package admin

import (
	"log/slog"
	"runtime"
	"time"

	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/config"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/providers"
)

// ProviderRegistry is an interface for provider registries.
type ProviderRegistry interface {
	Get(name string) (providers.Provider, bool)
	Models() []struct{ Provider, Model string }
	ResolveModelID(model string) string
	Default(model string) (providers.Provider, error)
}

// BuildInfo contains build-time information.
type BuildInfo struct {
	Version   string
	BuildTime string
	GitCommit string
	GoVersion string
}

// AdminServer hosts the admin API and UI.
type AdminServer struct {
	registry       ProviderRegistry
	convStore      conversation.Store
	cfg            *config.Config
	logger         *slog.Logger
	startTime      time.Time
	buildInfo      BuildInfo
	authMiddleware *auth.Middleware
}

// New creates an AdminServer instance.
func New(registry ProviderRegistry, convStore conversation.Store, cfg *config.Config, logger *slog.Logger, buildInfo BuildInfo, authMiddleware *auth.Middleware) *AdminServer {
	return &AdminServer{
		registry:       registry,
		convStore:      convStore,
		cfg:            cfg,
		logger:         logger,
		startTime:      time.Now(),
		buildInfo:      buildInfo,
		authMiddleware: authMiddleware,
	}
}

// GetBuildInfo returns a default BuildInfo if none provided.
func DefaultBuildInfo() BuildInfo {
	return BuildInfo{
		Version:   "dev",
		BuildTime: time.Now().Format(time.RFC3339),
		GitCommit: "unknown",
		GoVersion: runtime.Version(),
	}
}
