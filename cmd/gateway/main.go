package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9"

	"github.com/ajac-zero/latticelm/internal/admin"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/config"
	"github.com/ajac-zero/latticelm/internal/conversation"
	slogger "github.com/ajac-zero/latticelm/internal/logger"
	"github.com/ajac-zero/latticelm/internal/observability"
	"github.com/ajac-zero/latticelm/internal/providers"
	"github.com/ajac-zero/latticelm/internal/ratelimit"
	"github.com/ajac-zero/latticelm/internal/server"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(configPath)
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
			logger.Error("failed to initialize tracing", slog.String("error", err.Error()))
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

	// Create provider registry with circuit breaker support
	var baseRegistry *providers.Registry
	if cfg.Observability.Enabled && cfg.Observability.Metrics.Enabled {
		// Pass observability callback for circuit breaker state changes
		baseRegistry, err = providers.NewRegistryWithCircuitBreaker(
			cfg.Providers,
			cfg.Models,
			observability.RecordCircuitBreakerStateChange,
		)
	} else {
		// No observability, use default registry
		baseRegistry, err = providers.NewRegistry(cfg.Providers, cfg.Models)
	}
	if err != nil {
		logger.Error("failed to initialize providers", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Wrap providers with observability
	var registry server.ProviderRegistry = baseRegistry
	if cfg.Observability.Enabled {
		registry = observability.WrapProviderRegistry(registry, metricsRegistry, tracerProvider)
		logger.Info("providers instrumented")
	}

	// Initialize authentication middleware
	authConfig := auth.Config{
		Enabled:  cfg.Auth.Enabled,
		Issuer:   cfg.Auth.Issuer,
		Audience: cfg.Auth.Audience,
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

	adminAuthMiddleware := auth.NewAdmin(auth.AdminConfig{
		Enabled:       cfg.Auth.Enabled && cfg.Admin.Enabled,
		Claim:         cfg.Admin.Claim,
		AllowedValues: cfg.Admin.AllowedValues,
	})

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

	gatewayServer := server.New(registry, convStore, logger)
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

	publicMux := http.NewServeMux()
	gatewayServer.RegisterPublicRoutes(publicMux)

	apiMux := http.NewServeMux()
	gatewayServer.RegisterAPIRoutes(apiMux)

	var adminMux *http.ServeMux

	// Register admin endpoints if enabled
	if cfg.Admin.Enabled {
		// Check if frontend dist exists
		if _, err := os.Stat("internal/admin/dist"); os.IsNotExist(err) {
			log.Fatalf("admin UI enabled but frontend dist not found")
		}

		buildInfo := admin.BuildInfo{
			Version:   "dev",
			BuildTime: time.Now().Format(time.RFC3339),
			GitCommit: "unknown",
			GoVersion: runtime.Version(),
		}
		adminServer := admin.New(registry, convStore, cfg, logger, buildInfo)
		adminMux = http.NewServeMux()
		adminServer.RegisterRoutes(adminMux)
		logger.Info("admin UI enabled", slog.String("path", "/admin/"))
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

	// Rate limiting is applied inside buildRouteMux AFTER auth, so identity is available
	mux := buildRouteMux(publicMux, apiMux, adminMux, authMiddleware, adminAuthMiddleware, rateLimitMiddleware, metricsPath, metricsHandler)

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

		// Close conversation store
		logger.Info("closing conversation store")
		if err := convStore.Close(); err != nil {
			logger.Error("error closing conversation store", slog.String("error", err.Error()))
		}

		logger.Info("shutdown complete")
	}
}

func initConversationStore(cfg config.ConversationConfig, logger *slog.Logger) (conversation.Store, string, error) {
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

	switch cfg.Store {
	case "sql":
		driver := cfg.Driver
		if driver == "" {
			driver = "sqlite3"
		}
		db, err := sql.Open(driver, cfg.DSN)
		if err != nil {
			return nil, "", fmt.Errorf("open database: %w", err)
		}
		store, err := conversation.NewSQLStore(db, driver, ttl)
		if err != nil {
			return nil, "", fmt.Errorf("init sql store: %w", err)
		}
		logger.Info("conversation store initialized",
			slog.String("backend", "sql"),
			slog.String("driver", driver),
			slog.Duration("ttl", ttl),
		)
		return store, "sql", nil
	case "redis":
		opts, err := redis.ParseURL(cfg.DSN)
		if err != nil {
			return nil, "", fmt.Errorf("parse redis dsn: %w", err)
		}
		client := redis.NewClient(opts)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Ping(ctx).Err(); err != nil {
			return nil, "", fmt.Errorf("connect to redis: %w", err)
		}

		logger.Info("conversation store initialized",
			slog.String("backend", "redis"),
			slog.Duration("ttl", ttl),
		)
		return conversation.NewRedisStore(client, ttl), "redis", nil
	default:
		logger.Info("conversation store initialized",
			slog.String("backend", "memory"),
			slog.Duration("ttl", ttl),
		)
		return conversation.NewMemoryStore(ttl), "memory", nil
	}
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

type handlerMiddleware interface {
	Handler(http.Handler) http.Handler
}

func buildRouteMux(publicHandler, apiHandler, adminHandler http.Handler, apiAuth, adminAuth, rateLimiter handlerMiddleware, metricsPath string, metricsHandler http.Handler) *http.ServeMux {
	root := http.NewServeMux()

	if publicHandler != nil {
		root.Handle("/health", publicHandler)
		root.Handle("/ready", publicHandler)
	}

	if apiHandler != nil {
		// Apply rate limiting AFTER auth so identity claims are available
		if rateLimiter != nil {
			apiHandler = rateLimiter.Handler(apiHandler)
		}
		if apiAuth != nil {
			apiHandler = apiAuth.Handler(apiHandler)
		}
		root.Handle("/v1/", apiHandler)
	}

	if adminHandler != nil {
		if adminAuth != nil {
			adminHandler = adminAuth.Handler(adminHandler)
		}
		if apiAuth != nil {
			adminHandler = apiAuth.Handler(adminHandler)
		}
		root.Handle("/admin/", adminHandler)
	}

	if metricsHandler != nil {
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		root.Handle(metricsPath, metricsHandler)
	}

	return root
}
