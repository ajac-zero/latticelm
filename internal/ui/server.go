package ui

import (
	"log/slog"
	"runtime"
	"time"

	"github.com/ajac-zero/latticelm/internal/config"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/providers"
)

// BuildInfo contains build-time information.
type BuildInfo struct {
	Version   string
	BuildTime string
	GitCommit string
	GoVersion string
}

// Server hosts the admin API and UI.
type Server struct {
	registry    providers.ProviderRegistry
	convStore   conversation.Store
	cfg         *config.Config
	configStore *config.Store
	logger      *slog.Logger
	startTime   time.Time
	buildInfo   BuildInfo
}

// New creates a Server instance.
func New(registry providers.ProviderRegistry, convStore conversation.Store, cfg *config.Config, configStore *config.Store, logger *slog.Logger, buildInfo BuildInfo) *Server {
	return &Server{
		registry:    registry,
		convStore:   convStore,
		cfg:         cfg,
		configStore: configStore,
		logger:      logger,
		startTime:   time.Now(),
		buildInfo:   buildInfo,
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
