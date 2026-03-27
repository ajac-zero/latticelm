package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"

	"github.com/ajac-zero/latticelm/internal/apikeys"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/config"
	"github.com/ajac-zero/latticelm/internal/conversation"
	slogger "github.com/ajac-zero/latticelm/internal/logger"
	"github.com/ajac-zero/latticelm/internal/observability"
	"github.com/ajac-zero/latticelm/internal/providers"
	"github.com/ajac-zero/latticelm/internal/ratelimit"
	"github.com/ajac-zero/latticelm/internal/server"
	"github.com/ajac-zero/latticelm/internal/ui"
	"github.com/ajac-zero/latticelm/internal/usage"
	"github.com/ajac-zero/latticelm/internal/users"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	configStore, cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	// Initialize logger from config
	logFormat := cfg.Logging.Format
	if logFormat == "" {
		logFormat = "json"
	}
	logLevel := cfg.Logging.Level
	if logLevel == "" {
		logLevel = "info"
	}
	logger := slogger.New(logFormat, logLevel)

	// Initialize tracing
	var tracerProvider *sdktrace.TracerProvider
	if cfg.Observability.Enabled && cfg.Observability.Tracing.Enabled {
		// Set defaults
		tracingCfg := cfg.Observability.Tracing
		if tracingCfg.ServiceName == "" {
			tracingCfg.ServiceName = "llm-gateway"
		}
		if tracingCfg.Sampler.Type == "" {
			tracingCfg.Sampler.Type = "probability"
			tracingCfg.Sampler.Rate = 0.1
		}

		tp, err := observability.InitTracer(tracingCfg)
		if err != nil {
			logger.Warn("tracing initialization failed, continuing without tracing",
				slog.String("error", err.Error()),
			)
		} else {
			tracerProvider = tp
			otel.SetTracerProvider(tracerProvider)
			logger.Info("tracing initialized",
				slog.String("exporter", tracingCfg.Exporter.Type),
				slog.String("sampler", tracingCfg.Sampler.Type),
			)
		}
	}

	// Initialize metrics
	var metricsRegistry *prometheus.Registry
	if cfg.Observability.Enabled && cfg.Observability.Metrics.Enabled {
		metricsRegistry = observability.InitMetrics()
		metricsPath := cfg.Observability.Metrics.Path
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		logger.Info("metrics initialized", slog.String("path", metricsPath))
	}

	// buildRegistry constructs a live registry from the supplied config, applying
	// observability wrapping when enabled. Used both at startup and by the
	// config-polling goroutine to hot-swap the registry without restarting.
	buildRegistry := func(provEntries map[string]config.ProviderEntry, mods []config.ModelEntry) (providers.ProviderRegistry, error) {
		var base *providers.Registry
		var buildErr error
		if cfg.Observability.Enabled && cfg.Observability.Metrics.Enabled {
			base, buildErr = providers.NewRegistryWithCircuitBreaker(provEntries, mods, observability.RecordCircuitBreakerStateChange)
		} else {
			base, buildErr = providers.NewRegistry(provEntries, mods)
		}
		if buildErr != nil {
			return nil, buildErr
		}
		if cfg.Observability.Enabled {
			return observability.WrapProviderRegistry(base, metricsRegistry, tracerProvider), nil
		}
		return base, nil
	}

	initialRegistry, err := buildRegistry(cfg.Providers, cfg.Models)
	if err != nil {
		logger.Error("failed to initialize providers", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if cfg.Observability.Enabled {
		logger.Info("providers instrumented")
	}

	holder := providers.NewRegistryHolder(initialRegistry)

	// Initialize authentication middleware
	authConfig := auth.Config{
		Enabled:      cfg.Auth.Enabled,
		Issuer:       cfg.Auth.Issuer,
		DiscoveryURL: cfg.Auth.DiscoveryURL,
		Audiences:    cfg.Auth.Audiences,
		AdminClaim:   cfg.UI.Claim,
	}
	authMiddleware, err := auth.New(authConfig, logger)
	if err != nil {
		logger.Error("failed to initialize auth", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if cfg.Auth.Enabled {
		logger.Info("authentication enabled", slog.String("issuer", cfg.Auth.Issuer))
	} else {
		logger.Warn("authentication disabled - API is publicly accessible")
	}

	adminAuthConfig := auth.AdminConfig{
		Enabled:       cfg.Auth.Enabled && cfg.UI.Enabled,
		Claim:         cfg.UI.Claim,
		AllowedValues: cfg.UI.AllowedValues,
	}
	adminAuthMiddleware := auth.NewAdmin(adminAuthConfig)

	// Initialize user store (requires DATABASE_URL)
	var userStore *users.Store
	var userDB *sql.DB // shared by user store and API keys
	dbURL := os.Getenv("DATABASE_URL")
	if cfg.Conversations.IsEnabled() && dbURL != "" {
		var err error
		userDB, err = sql.Open("pgx", dbURL)
		if err != nil {
			logger.Error("failed to open database for user store", slog.String("error", err.Error()))
			os.Exit(1)
		}

		// Run user migrations
		migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer migrateCancel()
		userSchemaVersion, err := users.Migrate(migrateCtx, userDB, "pgx")
		if err != nil {
			logger.Error("user migration failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("user schema ready", slog.Int("version", userSchemaVersion))

		userStore = users.NewStore(userDB, "pgx")
		logger.Info("user store initialized")
	} else if cfg.Auth.Enabled && cfg.UI.Enabled {
		logger.Warn("auth enabled but DATABASE_URL not configured for user management")
		logger.Warn("user roles will not be persisted; all authenticated users will have basic access")
	}

	// Initialize OIDC client for UI authentication
	var oidcClient *auth.OIDCClient
	var sessionStore *auth.SessionStore
	if cfg.Auth.Enabled && cfg.UI.Enabled && cfg.Auth.ClientID != "" {
		if userStore == nil {
			logger.Error("OIDC authentication requires a database for user management")
			logger.Error("please set DATABASE_URL and enable conversations")
			os.Exit(1)
		}
		sessionStore, err = initSessionStore(cfg.Session, logger)
		if err != nil {
			logger.Error("failed to initialize session store", slog.String("error", err.Error()))
			os.Exit(1)
		}
		oidcClientConfig := auth.OIDCClientConfig{
			Issuer:       cfg.Auth.Issuer,
			DiscoveryURL: cfg.Auth.DiscoveryURL,
			ClientID:     cfg.Auth.ClientID,
			ClientSecret: cfg.Auth.ClientSecret,
			RedirectURI:  cfg.Auth.RedirectURI,
			AdminEmail:   cfg.Auth.AdminEmail,
		}
		oidcClient, err = auth.NewOIDCClient(oidcClientConfig, sessionStore, userStore, logger)
		if err != nil {
			logger.Error("failed to initialize OIDC client", slog.String("error", err.Error()))
			os.Exit(1)
		}
		// Link OIDC client to JWT middleware for enterprise-grade session validation
		// ID tokens are validated server-side, never exposed to frontend
		authMiddleware.SetOIDCClient(oidcClient)
		logger.Info("OIDC client enabled for UI authentication",
			slog.String("client_id", cfg.Auth.ClientID),
			slog.String("redirect_uri", cfg.Auth.RedirectURI),
		)
		if cfg.Auth.AdminEmail != "" {
			logger.Info("auto-promotion configured for admin email",
				slog.String("email", cfg.Auth.AdminEmail),
			)
		}
	}

	// Initialize API key authentication
	var apiKeyStore *apikeys.Store
	if cfg.APIKeys.Enabled {
		if userDB == nil || userStore == nil {
			logger.Error("API keys require DATABASE_URL and conversations enabled for user management")
			os.Exit(1)
		}

		migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer migrateCancel()
		akSchemaVersion, err := apikeys.Migrate(migrateCtx, userDB)
		if err != nil {
			logger.Error("api_keys migration failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("api_keys schema ready", slog.Int("version", akSchemaVersion))

		apiKeyStore = apikeys.NewStore(userDB)

		// Prepend API key authenticator to the auth chain so sk-* tokens
		// are resolved before attempting JWT validation.
		authMiddleware.PrependAuthenticator(&apikeys.Authenticator{
			Store:  apiKeyStore,
			Logger: logger,
		})

		logger.Info("API key authentication enabled")
	}

	// Initialize conversation store
	var convStore conversation.Store
	storeBackend := "nop"
	if cfg.Conversations.IsEnabled() {
		var err error
		convStore, storeBackend, err = initConversationStore(cfg.Conversations, logger)
		if err != nil {
			logger.Error("failed to initialize conversation store", slog.String("error", err.Error()))
			os.Exit(1)
		}
	} else {
		convStore = conversation.NewNopStore()
		logger.Info("conversation storage disabled (default no-store)")
	}

	// Wrap conversation store with observability
	if cfg.Observability.Enabled && storeBackend != "nop" {
		convStore = observability.WrapConversationStore(convStore, storeBackend, metricsRegistry, tracerProvider)
		logger.Info("conversation store instrumented")
	}

	gatewayServer := server.New(holder, convStore, logger,
		server.WithAdminConfig(auth.AdminConfig{
			Enabled:       cfg.Auth.Enabled && cfg.UI.Enabled,
			Claim:         cfg.UI.Claim,
			AllowedValues: cfg.UI.AllowedValues,
		}),
	)
	gatewayServer.SetStoreByDefault(cfg.Conversations.StoreByDefault)

	// Initialize distributed rate limiting
	var rateLimitMiddleware *ratelimit.Middleware
	if cfg.RateLimit.Enabled {
		rateLimitConfig := ratelimit.Config{
			Enabled:               true,
			RedisURL:              cfg.RateLimit.RedisURL,
			TrustedProxyCIDRs:     cfg.RateLimit.TrustedProxyCIDRs,
			RequestsPerSecond:     cfg.RateLimit.RequestsPerSecond,
			Burst:                 cfg.RateLimit.Burst,
			MaxPromptTokens:       cfg.RateLimit.MaxPromptTokens,
			MaxOutputTokens:       cfg.RateLimit.MaxOutputTokens,
			MaxConcurrentRequests: cfg.RateLimit.MaxConcurrentRequests,
			DailyTokenQuota:       cfg.RateLimit.DailyTokenQuota,
		}

		if rateLimitConfig.RedisURL == "" {
			logger.Error("rate limiting requires redis_url configuration")
			os.Exit(1)
		}

		rlOpts, err := redis.ParseURL(rateLimitConfig.RedisURL)
		if err != nil {
			logger.Error("failed to parse rate limit redis URL", slog.String("error", err.Error()))
			os.Exit(1)
		}
		rlRedisClient := redis.NewClient(rlOpts)

		rlCtx, rlCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer rlCancel()
		if err := rlRedisClient.Ping(rlCtx).Err(); err != nil {
			logger.Error("failed to connect to rate limit Redis", slog.String("error", err.Error()))
			os.Exit(1)
		}

		backend := ratelimit.NewRedisBackend(rlRedisClient)
		rateLimitMiddleware, err = ratelimit.New(rateLimitConfig, backend, logger)
		if err != nil {
			logger.Error("failed to initialize rate limiting", slog.String("error", err.Error()))
			os.Exit(1)
		}

		// Set token limits on the gateway server
		gatewayServer.SetTokenLimits(server.TokenLimits{
			MaxPromptTokens: rateLimitConfig.MaxPromptTokens,
			MaxOutputTokens: rateLimitConfig.MaxOutputTokens,
		})

		logger.Info("distributed rate limiting enabled",
			slog.Float64("requests_per_second", rateLimitConfig.RequestsPerSecond),
			slog.Int("burst", rateLimitConfig.Burst),
			slog.Int("max_concurrent_requests", rateLimitConfig.MaxConcurrentRequests),
			slog.Int64("daily_token_quota", rateLimitConfig.DailyTokenQuota),
			slog.Int("max_output_tokens", rateLimitConfig.MaxOutputTokens),
			slog.Int("max_prompt_tokens", rateLimitConfig.MaxPromptTokens),
			slog.Int("trusted_proxy_cidrs", len(rateLimitConfig.TrustedProxyCIDRs)),
		)
	}

	// Initialize token usage tracking
	var usageStore usage.Backend
	if cfg.Usage.Enabled {
		usageDBURL := os.Getenv("DATABASE_URL")
		if usageDBURL == "" {
			logger.Error("usage tracking requires DATABASE_URL")
			os.Exit(1)
		}

		var flushInterval time.Duration
		if cfg.Usage.FlushInterval != "" {
			if d, parseErr := time.ParseDuration(cfg.Usage.FlushInterval); parseErr == nil {
				flushInterval = d
			}
		}

		usageDB, err := sql.Open("pgx", usageDBURL)
		if err != nil {
			logger.Error("failed to open usage database", slog.String("error", err.Error()))
			os.Exit(1)
		}

		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer pingCancel()
		if err := usageDB.PingContext(pingCtx); err != nil {
			_ = usageDB.Close()
			logger.Error("failed to ping usage database", slog.String("error", err.Error()))
			os.Exit(1)
		}

		usageDB.SetMaxOpenConns(10)
		usageDB.SetMaxIdleConns(5)
		usageDB.SetConnMaxLifetime(5 * time.Minute)

		migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer migrateCancel()
		usageSchemaVersion, err := usage.Migrate(migrateCtx, usageDB)
		if err != nil {
			logger.Error("usage migration failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("usage schema ready", slog.Int("version", usageSchemaVersion))

		usageStore = usage.NewStore(usageDB, logger, cfg.Usage.BufferSize, flushInterval)
		logger.Info("token usage tracking enabled",
			slog.Int("buffer_size", cfg.Usage.BufferSize),
			slog.String("flush_interval", cfg.Usage.FlushInterval),
		)
	}

	publicMux := http.NewServeMux()
	gatewayServer.RegisterPublicRoutes(publicMux)

	apiMux := http.NewServeMux()
	gatewayServer.RegisterAPIRoutes(apiMux)

	// Register API key management routes on the API mux (auth-protected)
	if apiKeyStore != nil {
		akAPI := apikeys.NewAPI(apiKeyStore, userStore, logger, cfg.APIKeys.MaxKeysPerUser)
		akAPI.RegisterRoutes(apiMux)
		logger.Info("API key management endpoints enabled")
	}

	// Register usage read API on the API mux (auth-protected via middleware below)
	if usageStore != nil {
		usageOpts := []func(*usage.API){}
		if userStore != nil {
			usageOpts = append(usageOpts, usage.WithUserResolver(userStore))
		}
		usageAPI := usage.NewAPI(usageStore, usageOpts...)
		usageAPI.RegisterRoutes(apiMux)
		logger.Info("usage read API enabled")
	}

	var adminHandler http.Handler
	var adminServer *ui.Server

	// Register admin endpoints if enabled
	if cfg.UI.Enabled {
		buildInfo := ui.BuildInfo{
			Version:   "dev",
			BuildTime: time.Now().Format(time.RFC3339),
			GitCommit: "unknown",
			GoVersion: runtime.Version(),
		}
		adminServer = ui.New(holder, convStore, cfg, configStore, logger, buildInfo)
		adminMux := http.NewServeMux()
		adminServer.RegisterRoutes(adminMux)

		// Register core gateway API under /api/v1/ so the embedded UI (session auth)
		// can call /api/v1/models and /api/v1/responses without a JWT bearer token.
		gatewayServer.RegisterAdminAPIRoutes(adminMux)

		// Register users API on admin mux (protected by session middleware)
		if userStore != nil {
			usersAPI := users.NewAPI(userStore)
			usersAPI.RegisterRoutes(adminMux)
		}

		// Register API key management on admin mux (session auth)
		if apiKeyStore != nil {
			akAPI := apikeys.NewAPI(apiKeyStore, userStore, logger, cfg.APIKeys.MaxKeysPerUser)
			akAPI.RegisterAdminRoutes(adminMux)
		}

		// Register usage read API on admin mux so the embedded UI (session auth)
		// can query /api/v1/usage/* without a JWT bearer token.
		if usageStore != nil {
			usageOpts := []func(*usage.API){}
			if userStore != nil {
				usageOpts = append(usageOpts, usage.WithUserResolver(userStore))
			}
			usageAPI := usage.NewAPI(usageStore, usageOpts...)
			usageAPI.RegisterAdminRoutes(adminMux)
		}

		// Parse IP allowlist CIDRs; fail fast if misconfigured.
		allowlist, err := ui.ParseCIDRs(cfg.UI.IPAllowlist)
		if err != nil {
			logger.Error("invalid admin ip_allowlist", slog.String("error", err.Error()))
			os.Exit(1)
		}

		// Wrap: security headers (outermost) then IP allowlist check.
		var wrapped http.Handler = adminMux
		wrapped = ui.IPAllowlistMiddleware(allowlist)(wrapped)
		wrapped = ui.SecurityHeadersMiddleware(wrapped)
		adminHandler = wrapped

		logger.Info("admin UI enabled", slog.String("path", "/"))
		if len(allowlist) > 0 {
			logger.Info("admin IP allowlist active", slog.Int("cidr_count", len(allowlist)))
		}
	}

	// Start config-polling goroutine when a DB-backed config store is available.
	// Every 30 s each pod independently reloads providers and models from the DB
	// and hot-swaps the registry via the holder. This keeps all horizontally-
	// scaled instances eventually consistent without any inter-pod signalling.
	pollCtx, pollCancel := context.WithCancel(context.Background())
	if configStore != nil {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					provEntries, listErr := configStore.ListProviders(pollCtx)
					if listErr != nil {
						logger.Error("registry poll: list providers", slog.String("error", listErr.Error()))
						continue
					}
					mods, listErr := configStore.ListModels(pollCtx)
					if listErr != nil {
						logger.Error("registry poll: list models", slog.String("error", listErr.Error()))
						continue
					}
					newReg, buildErr := buildRegistry(provEntries, mods)
					if buildErr != nil {
						logger.Error("registry poll: build registry", slog.String("error", buildErr.Error()))
						continue
					}
					holder.Swap(newReg)
					logger.Debug("registry reloaded from config store")
				case <-pollCtx.Done():
					return
				}
			}
		}()
		logger.Info("config polling enabled", slog.Duration("interval", 30*time.Second))
	}

	metricsPath := cfg.Observability.Metrics.Path
	if metricsPath == "" {
		metricsPath = "/metrics"
	}

	var metricsHandler http.Handler
	if cfg.Observability.Enabled && cfg.Observability.Metrics.Enabled {
		metricsHandler = promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})
		logger.Info("metrics endpoint registered", slog.String("path", metricsPath))
	}

	// Compose middleware on API handler before registering routes.
	// Order: auth (outermost) → rate limiting → usage recorder → handler
	var apiHandler http.Handler = apiMux
	if usageStore != nil {
		inner := apiHandler
		apiHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inner.ServeHTTP(w, r.WithContext(usage.WithRecorder(r.Context(), usageStore)))
		})
	}
	if rateLimitMiddleware != nil {
		apiHandler = rateLimitMiddleware.Handler(apiHandler)
	}
	if authMiddleware != nil {
		apiHandler = authMiddleware.Handler(apiHandler)
	}

	// Compose middleware on admin handler
	if adminHandler != nil {
		if usageStore != nil {
			inner := adminHandler
			adminHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				inner.ServeHTTP(w, r.WithContext(usage.WithRecorder(r.Context(), usageStore)))
			})
		}
		if oidcClient != nil {
			// Use session-based auth for UI
			adminHandler = oidcClient.SessionMiddleware(adminHandler)
		} else if adminAuthMiddleware != nil {
			// Fall back to JWT auth
			adminHandler = adminAuthMiddleware.Handler(adminHandler)
			if authMiddleware != nil {
				adminHandler = authMiddleware.Handler(adminHandler)
			}
		}
	}

	mux := buildRouteMux(publicMux, apiHandler, adminHandler, nil, metricsPath, metricsHandler)

	authAPI := auth.NewAPI(cfg.Auth.Enabled, oidcClient != nil, authMiddleware, oidcClient, userStore, adminAuthConfig)
	authAPI.RegisterRoutes(mux)

	addr := cfg.Server.Address
	if addr == "" {
		addr = ":8080"
	}

	// Determine max request body size
	maxRequestBodySize := cfg.Server.MaxRequestBodySize
	if maxRequestBodySize == 0 {
		maxRequestBodySize = server.MaxRequestBodyBytes // default: 10MB
	}

	logger.Info("server configuration",
		slog.Int64("max_request_body_bytes", maxRequestBodySize),
	)

	// Build handler chain: panic recovery -> request size limit -> logging -> tracing -> metrics -> routes
	// Rate limiting is applied per-route inside buildRouteMux after auth identity extraction
	handler := server.PanicRecoveryMiddleware(
		server.RequestSizeLimitMiddleware(
			loggingMiddleware(
				observability.TracingMiddleware(
					observability.MetricsMiddleware(
						mux,
						metricsRegistry,
						tracerProvider,
					),
					tracerProvider,
				),
				logger,
			),
			maxRequestBodySize,
		),
		logger,
	)

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Run server in a goroutine
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("open responses gateway listening", slog.String("address", addr))
		serverErrors <- srv.ListenAndServe()
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	case sig := <-sigChan:
		logger.Info("received shutdown signal", slog.String("signal", sig.String()))

		// Stop the config-polling goroutine.
		pollCancel()

		// Create shutdown context with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Shutdown the HTTP server gracefully
		logger.Info("shutting down server gracefully")
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown error", slog.String("error", err.Error()))
		}

		// Shutdown tracer provider
		if tracerProvider != nil {
			logger.Info("shutting down tracer")
			shutdownTracerCtx, shutdownTracerCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownTracerCancel()
			if err := observability.Shutdown(shutdownTracerCtx, tracerProvider); err != nil {
				logger.Error("error shutting down tracer", slog.String("error", err.Error()))
			}
		}

		// Flush and close usage store
		if usageStore != nil {
			logger.Info("flushing usage store")
			if err := usageStore.Close(); err != nil {
				logger.Error("error closing usage store", slog.String("error", err.Error()))
			}
		}

		// Close conversation store
		logger.Info("closing conversation store")
		if err := convStore.Close(); err != nil {
			logger.Error("error closing conversation store", slog.String("error", err.Error()))
		}

		// Close session store
		if sessionStore != nil {
			logger.Info("closing session store")
			if err := sessionStore.Close(); err != nil {
				logger.Error("error closing session store", slog.String("error", err.Error()))
			}
		}

		// Close config store DB connection if DB-backed config is in use
		if configStore != nil {
			if err := configStore.Close(); err != nil {
				logger.Error("error closing config store", slog.String("error", err.Error()))
			}
		}

		logger.Info("shutdown complete")
	}
}

// loadConfig builds the gateway configuration. Infrastructure config always
// comes from environment variables via LoadFromEnv. When DATABASE_URL and
// ENCRYPTION_KEY are both set, providers and models are loaded from (and
// persisted to) the database; on the first run with an empty database they
// are expected to be added through the UI or seeding mechanism.
//
// The returned *config.Store is nil when DATABASE_URL/ENCRYPTION_KEY are absent.
func loadConfig() (*config.Store, *config.Config, error) {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return nil, nil, fmt.Errorf("load config from env: %w", err)
	}

	dbURL := os.Getenv("DATABASE_URL")
	encKey := os.Getenv("ENCRYPTION_KEY")

	if dbURL == "" || encKey == "" {
		cfgPath := os.Getenv("GATEWAY_CONFIG")
		if cfgPath == "" {
			return nil, nil, fmt.Errorf("no provider configuration: set GATEWAY_CONFIG (file path) or DATABASE_URL + ENCRYPTION_KEY (database-backed store)")
		}
		providers, models, err := config.LoadFromFile(cfgPath)
		if err != nil {
			return nil, nil, fmt.Errorf("GATEWAY_CONFIG: %w", err)
		}
		cfg.Providers = providers
		cfg.Models = models
		if err := cfg.Validate(); err != nil {
			return nil, nil, fmt.Errorf("GATEWAY_CONFIG validation: %w", err)
		}
		return nil, cfg, nil
	}

	if !strings.HasPrefix(dbURL, "postgres://") && !strings.HasPrefix(dbURL, "postgresql://") {
		return nil, nil, fmt.Errorf("DATABASE_URL must be a PostgreSQL connection string (got %q)", dbURL)
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return nil, nil, fmt.Errorf("open config database: %w", err)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("connect to config database: %w", err)
	}

	configStore, err := config.NewStore(db, encKey)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("init config store: %w", err)
	}

	migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer migrateCancel()
	if err := configStore.Migrate(migrateCtx); err != nil {
		_ = configStore.Close()
		return nil, nil, fmt.Errorf("config migration: %w", err)
	}

	ctx := context.Background()

	// Seed from GATEWAY_CONFIG if the database has no providers yet.
	if cfgPath := os.Getenv("GATEWAY_CONFIG"); cfgPath != "" {
		seeded, err := configStore.IsSeeded(ctx)
		if err != nil {
			_ = configStore.Close()
			return nil, nil, fmt.Errorf("check config store seed status: %w", err)
		}
		if !seeded {
			fileProviders, fileModels, err := config.LoadFromFile(cfgPath)
			if err != nil {
				_ = configStore.Close()
				return nil, nil, fmt.Errorf("GATEWAY_CONFIG: %w", err)
			}
			if err := configStore.SeedIfEmpty(ctx, fileProviders, fileModels); err != nil {
				_ = configStore.Close()
				return nil, nil, fmt.Errorf("seed config store from GATEWAY_CONFIG: %w", err)
			}
		}
	}

	providers, err := configStore.ListProviders(ctx)
	if err != nil {
		_ = configStore.Close()
		return nil, nil, fmt.Errorf("load providers from database: %w", err)
	}
	models, err := configStore.ListModels(ctx)
	if err != nil {
		_ = configStore.Close()
		return nil, nil, fmt.Errorf("load models from database: %w", err)
	}

	cfg.Providers = providers
	cfg.Models = models

	if err := cfg.Validate(); err != nil {
		_ = configStore.Close()
		return nil, nil, fmt.Errorf("config validation: %w", err)
	}

	return configStore, cfg, nil
}

func initConversationStore(cfg config.ConversationConfig, logger *slog.Logger) (conversation.Store, string, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, "", fmt.Errorf("conversations enabled but DATABASE_URL is not set")
	}

	var ttl time.Duration
	if cfg.TTL != "" {
		parsed, err := time.ParseDuration(cfg.TTL)
		if err != nil {
			return nil, "", fmt.Errorf("invalid conversation ttl %q: %w", cfg.TTL, err)
		}
		ttl = parsed
	}

	// Enforce max_ttl ceiling
	if cfg.MaxTTL != "" {
		maxTTL, err := time.ParseDuration(cfg.MaxTTL)
		if err != nil {
			return nil, "", fmt.Errorf("invalid conversation max_ttl %q: %w", cfg.MaxTTL, err)
		}
		if maxTTL > 0 && (ttl == 0 || ttl > maxTTL) {
			logger.Warn("clamping conversation TTL to max_ttl",
				slog.Duration("original_ttl", ttl),
				slog.Duration("max_ttl", maxTTL),
			)
			ttl = maxTTL
		}
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, "", fmt.Errorf("open database: %w", err)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, "", fmt.Errorf("ping database: %w", err)
	}

	maxOpenConns := cfg.MaxOpenConns
	if maxOpenConns == 0 {
		maxOpenConns = 25
	}
	maxIdleConns := cfg.MaxIdleConns
	if maxIdleConns == 0 {
		maxIdleConns = 5
	}
	connMaxLifetime := 5 * time.Minute
	if cfg.ConnMaxLifetime != "" {
		if d, parseErr := time.ParseDuration(cfg.ConnMaxLifetime); parseErr == nil {
			connMaxLifetime = d
		}
	}
	connMaxIdleTime := 1 * time.Minute
	if cfg.ConnMaxIdleTime != "" {
		if d, parseErr := time.ParseDuration(cfg.ConnMaxIdleTime); parseErr == nil {
			connMaxIdleTime = d
		}
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	store, err := conversation.NewSQLStore(db, "pgx", ttl)
	if err != nil {
		_ = db.Close()
		return nil, "", fmt.Errorf("init sql store: %w", err)
	}
	logger.Info("conversation store initialized",
		slog.String("backend", "postgresql"),
		slog.Duration("ttl", ttl),
		slog.Int("max_open_conns", maxOpenConns),
		slog.Int("max_idle_conns", maxIdleConns),
		slog.Duration("conn_max_lifetime", connMaxLifetime),
		slog.Duration("conn_max_idle_time", connMaxIdleTime),
	)
	return store, "sql", nil
}

func initSessionStore(cfg config.SessionConfig, logger *slog.Logger) (*auth.SessionStore, error) {
	var ttl time.Duration = 24 * time.Hour
	if cfg.TTL != "" {
		parsed, err := time.ParseDuration(cfg.TTL)
		if err != nil {
			return nil, fmt.Errorf("invalid session ttl %q: %w", cfg.TTL, err)
		}
		ttl = parsed
	}

	// Use Redis backend if configured
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("parse session redis URL: %w", err)
		}
		client := redis.NewClient(opts)

		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer pingCancel()
		if err := client.Ping(pingCtx).Err(); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("ping session redis: %w", err)
		}

		backend := auth.NewRedisSessionBackend(client)
		store := auth.NewSessionStore(ttl, backend)
		logger.Info("session store initialized",
			slog.String("backend", "redis"),
			slog.Duration("ttl", ttl),
		)
		return store, nil
	}

	// Default to in-memory backend
	store := auth.NewSessionStore(ttl, nil)
	logger.Info("session store initialized",
		slog.String("backend", "memory"),
		slog.Duration("ttl", ttl),
		slog.String("warning", "in-memory sessions do not work with multiple replicas"),
	)
	return store, nil
}

type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
	wroteHeader  bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.wroteHeader = true
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.wroteHeader = true
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID
		requestID := uuid.NewString()
		ctx := slogger.WithRequestID(r.Context(), requestID)
		r = r.WithContext(ctx)

		// Wrap response writer to capture status code
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Add request ID header
		w.Header().Set("X-Request-ID", requestID)

		// Log request start
		logger.InfoContext(ctx, "request started",
			slog.String("request_id", requestID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		)

		next.ServeHTTP(rw, r)

		duration := time.Since(start)

		// Log request completion with appropriate level
		logLevel := slog.LevelInfo
		if rw.statusCode >= 500 {
			logLevel = slog.LevelError
		} else if rw.statusCode >= 400 {
			logLevel = slog.LevelWarn
		}

		logger.Log(ctx, logLevel, "request completed",
			slog.String("request_id", requestID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status_code", rw.statusCode),
			slog.Int("response_bytes", rw.bytesWritten),
			slog.Duration("duration", duration),
			slog.Float64("duration_ms", float64(duration.Milliseconds())),
		)
	})
}

func buildRouteMux(publicHandler, apiHandler, adminHandler, authHandler http.Handler, metricsPath string, metricsHandler http.Handler) *http.ServeMux {
	root := http.NewServeMux()

	if publicHandler != nil {
		root.Handle("/health", publicHandler)
		root.Handle("/ready", publicHandler)
	}

	if apiHandler != nil {
		root.Handle("/v1/", apiHandler)
	}

	if authHandler != nil {
		root.Handle("/auth/", authHandler)
	}

	if adminHandler != nil {
		root.Handle("/", adminHandler)
	}

	if metricsHandler != nil {
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		root.Handle(metricsPath, metricsHandler)
	}

	return root
}
