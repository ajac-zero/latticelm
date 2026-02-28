package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/yourusername/go-llm-gateway/internal/config"
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

	registry, err := providers.NewRegistry(cfg.Providers)
	if err != nil {
		log.Fatalf("init providers: %v", err)
	}

	logger := log.New(os.Stdout, "gateway ", log.LstdFlags|log.Lshortfile)

	gatewayServer := server.New(registry, logger)
	mux := http.NewServeMux()
	gatewayServer.RegisterRoutes(mux)

	addr := cfg.Server.Address
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      loggingMiddleware(mux, logger),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logger.Printf("Open Responses gateway listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server error: %v", err)
	}
}

func loggingMiddleware(next http.Handler, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
