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
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9"

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

	baseRegistry, err := providers.NewRegistry(cfg.Providers, cfg.Models)
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
	authMiddleware, err := auth.New(authConfig)
	if err != nil {
		logger.Error("failed to initialize auth", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if cfg.Auth.Enabled {
		logger.Info("authentication enabled", slog.String("issuer", cfg.Auth.Issuer))
	} else {
		logger.Warn("authentication disabled - API is publicly accessible")
	}

	// Initialize conversation store
	convStore, storeBackend, err := initConversationStore(cfg.Conversations, logger)
	if err != nil {
		logger.Error("failed to initialize conversation store", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Wrap conversation store with observability
	if cfg.Observability.Enabled && convStore != nil {
		convStore = observability.WrapConversationStore(convStore, storeBackend, metricsRegistry, tracerProvider)
		logger.Info("conversation store instrumented")
	}

	gatewayServer := server.New(registry, convStore, logger)
	mux := http.NewServeMux()
	gatewayServer.RegisterRoutes(mux)

	// Register metrics endpoint if enabled
	if cfg.Observability.Enabled && cfg.Observability.Metrics.Enabled {
		metricsPath := cfg.Observability.Metrics.Path
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		mux.Handle(metricsPath, promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}))
		logger.Info("metrics endpoint registered", slog.String("path", metricsPath))
	}

	addr := cfg.Server.Address
	if addr == "" {
		addr = ":8080"
	}

	// Initialize rate limiting
	rateLimitConfig := ratelimit.Config{
		Enabled:           cfg.RateLimit.Enabled,
		RequestsPerSecond: cfg.RateLimit.RequestsPerSecond,
		Burst:             cfg.RateLimit.Burst,
	}
	// Set defaults if not configured
	if rateLimitConfig.Enabled && rateLimitConfig.RequestsPerSecond == 0 {
		rateLimitConfig.RequestsPerSecond = 10 // default 10 req/s
	}
	if rateLimitConfig.Enabled && rateLimitConfig.Burst == 0 {
		rateLimitConfig.Burst = 20 // default burst of 20
	}
	rateLimitMiddleware := ratelimit.New(rateLimitConfig, logger)

	if cfg.RateLimit.Enabled {
		logger.Info("rate limiting enabled",
			slog.Float64("requests_per_second", rateLimitConfig.RequestsPerSecond),
			slog.Int("burst", rateLimitConfig.Burst),
		)
	}

	// Determine max request body size
	maxRequestBodySize := cfg.Server.MaxRequestBodySize
	if maxRequestBodySize == 0 {
		maxRequestBodySize = server.MaxRequestBodyBytes // default: 10MB
	}

	logger.Info("server configuration",
		slog.Int64("max_request_body_bytes", maxRequestBodySize),
	)

	// Build handler chain: panic recovery -> request size limit -> logging -> tracing -> metrics -> rate limiting -> auth -> routes
	handler := server.PanicRecoveryMiddleware(
		server.RequestSizeLimitMiddleware(
			loggingMiddleware(
				observability.TracingMiddleware(
					observability.MetricsMiddleware(
						rateLimitMiddleware.Handler(authMiddleware.Handler(mux)),
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
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
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
