package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ajac-zero/latticelm/internal/config"
)

var _ http.Flusher = (*responseWriter)(nil)

type countingFlusherRecorder struct {
	*httptest.ResponseRecorder
	flushCount int
}

func newCountingFlusherRecorder() *countingFlusherRecorder {
	return &countingFlusherRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (r *countingFlusherRecorder) Flush() {
	r.flushCount++
}

func TestResponseWriterWriteHeaderOnlyOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusCreated)
	rw.WriteHeader(http.StatusInternalServerError)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, http.StatusCreated, rw.statusCode)
}

func TestResponseWriterWriteSetsImplicitStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	n, err := rw.Write([]byte("ok"))

	assert.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, http.StatusOK, rw.statusCode)
	assert.Equal(t, 2, rw.bytesWritten)
}

func TestResponseWriterFlushDelegates(t *testing.T) {
	rec := newCountingFlusherRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.Flush()

	assert.Equal(t, 1, rec.flushCount)
}

type stubMiddleware struct {
	handler func(http.Handler) http.Handler
}

func (m stubMiddleware) Handler(next http.Handler) http.Handler {
	return m.handler(next)
}

func TestBuildRouteMuxSeparatesSecurityBoundaries(t *testing.T) {
	publicMux := http.NewServeMux()
	publicMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("healthy"))
	})
	publicMux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("models"))
	})

	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/admin/api/v1/system/info", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("admin"))
	})

	apiAuth := stubMiddleware{
		handler: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-API-Auth") != "ok" {
					http.Error(w, "missing api auth", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
			})
		},
	}

	adminAuth := stubMiddleware{
		handler: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Admin") != "ok" {
					http.Error(w, "missing admin auth", http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
			})
		},
	}

	// Compose middleware at the call site (matching production code)
	var apiHandler http.Handler = apiMux
	apiHandler = apiAuth.Handler(apiHandler)

	var adminHandler http.Handler = adminMux
	adminHandler = adminAuth.Handler(adminHandler)
	adminHandler = apiAuth.Handler(adminHandler)

	mux := buildRouteMux(
		publicMux,
		apiHandler,
		adminHandler,
		nil, // authHandler not needed for this test
		"/metrics",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("metrics"))
		}),
	)

	tests := []struct {
		name         string
		path         string
		headers      map[string]string
		expectStatus int
		expectBody   string
	}{
		{
			name:         "health is public",
			path:         "/health",
			expectStatus: http.StatusOK,
			expectBody:   "healthy",
		},
		{
			name:         "metrics is public",
			path:         "/metrics",
			expectStatus: http.StatusOK,
			expectBody:   "metrics",
		},
		{
			name:         "api requires auth",
			path:         "/v1/models",
			expectStatus: http.StatusUnauthorized,
			expectBody:   "missing api auth",
		},
		{
			name: "/v1 models allows authenticated requests",
			path: "/v1/models",
			headers: map[string]string{
				"X-API-Auth": "ok",
			},
			expectStatus: http.StatusOK,
			expectBody:   "models",
		},
		{
			name: "admin rejects non admin user",
			path: "/admin/api/v1/system/info",
			headers: map[string]string{
				"X-API-Auth": "ok",
			},
			expectStatus: http.StatusForbidden,
			expectBody:   "missing admin auth",
		},
		{
			name: "admin allows admin user",
			path: "/admin/api/v1/system/info",
			headers: map[string]string{
				"X-API-Auth": "ok",
				"X-Admin":    "ok",
			},
			expectStatus: http.StatusOK,
			expectBody:   "admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.expectBody)
		})
	}
}

func TestInitConversationStore_BadDSN_FailsFast(t *testing.T) {
	cfg := config.ConversationConfig{
		Store:  "sql",
		Driver: "sqlite3",
		// Path in a non-existent directory; sqlite3 cannot create the file.
		DSN: "/nonexistent_dir_for_latticelm_test/bad.db",
	}
	store, _, err := initConversationStore(cfg, slog.Default())
	require.Error(t, err, "expected error for bad DSN")
	assert.Nil(t, store, "store must be nil on failure")
	assert.Contains(t, err.Error(), "ping database", "error should mention ping failure")
}

func TestInitConversationStore_DefaultPoolSettings(t *testing.T) {
	cfg := config.ConversationConfig{
		Store:  "sql",
		Driver: "sqlite3",
		DSN:    ":memory:",
	}
	store, backend, err := initConversationStore(cfg, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, store)
	assert.Equal(t, "sql", backend)
	_ = store.Close()
}

func TestInitConversationStore_CustomPoolSettings(t *testing.T) {
	cfg := config.ConversationConfig{
		Store:           "sql",
		Driver:          "sqlite3",
		DSN:             ":memory:",
		MaxOpenConns:    10,
		MaxIdleConns:    2,
		ConnMaxLifetime: "10m",
		ConnMaxIdleTime: "2m",
	}
	store, backend, err := initConversationStore(cfg, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, store)
	assert.Equal(t, "sql", backend)
	_ = store.Close()
}
