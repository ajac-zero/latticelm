package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"

	"github.com/yourusername/go-llm-gateway/internal/auth"
	"github.com/yourusername/go-llm-gateway/internal/config"
	"github.com/yourusername/go-llm-gateway/internal/conversation"
	"github.com/yourusername/go-llm-gateway/internal/providers"
	"github.com/yourusername/go-llm-gateway/internal/server"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	registry, err := providers.NewRegistry(cfg.Providers, cfg.Models)
	if err != nil {
		log.Fatalf("init providers: %v", err)
	}

	logger := log.New(os.Stdout, "gateway ", log.LstdFlags|log.Lshortfile)

	// Initialize authentication middleware
	authConfig := auth.Config{
		Enabled:  cfg.Auth.Enabled,
		Issuer:   cfg.Auth.Issuer,
		Audience: cfg.Auth.Audience,
	}
	authMiddleware, err := auth.New(authConfig)
	if err != nil {
		log.Fatalf("init auth: %v", err)
	}

	if cfg.Auth.Enabled {
		logger.Printf("Authentication enabled (issuer: %s)", cfg.Auth.Issuer)
	} else {
		logger.Printf("Authentication disabled - WARNING: API is publicly accessible")
	}

	// Initialize conversation store
	convStore, err := initConversationStore(cfg.Conversations, logger)
	if err != nil {
		log.Fatalf("init conversation store: %v", err)
	}

	gatewayServer := server.New(registry, convStore, logger)
	mux := http.NewServeMux()
	gatewayServer.RegisterRoutes(mux)

	addr := cfg.Server.Address
	if addr == "" {
		addr = ":8080"
	}

	// Build handler chain: logging -> auth -> routes
	handler := loggingMiddleware(authMiddleware.Handler(mux), logger)

	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logger.Printf("Open Responses gateway listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server error: %v", err)
	}
}

func initConversationStore(cfg config.ConversationConfig, logger *log.Logger) (conversation.Store, error) {
	var ttl time.Duration
	if cfg.TTL != "" {
		parsed, err := time.ParseDuration(cfg.TTL)
		if err != nil {
			return nil, fmt.Errorf("invalid conversation ttl %q: %w", cfg.TTL, err)
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
			return nil, fmt.Errorf("open database: %w", err)
		}
		store, err := conversation.NewSQLStore(db, driver, ttl)
		if err != nil {
			return nil, fmt.Errorf("init sql store: %w", err)
		}
		logger.Printf("Conversation store initialized (sql/%s, TTL: %s)", driver, ttl)
		return store, nil
	default:
		logger.Printf("Conversation store initialized (memory, TTL: %s)", ttl)
		return conversation.NewMemoryStore(ttl), nil
	}
}
func loggingMiddleware(next http.Handler, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
