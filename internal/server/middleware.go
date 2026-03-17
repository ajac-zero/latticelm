package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/ajac-zero/latticelm/internal/logger"
)

// MaxRequestBodyBytes is the maximum size allowed for request bodies (10MB)
const MaxRequestBodyBytes = 10 * 1024 * 1024

// PanicRecoveryMiddleware recovers from panics in HTTP handlers and logs them
// instead of crashing the server. Returns 500 Internal Server Error to the client.
func PanicRecoveryMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Capture stack trace
				stack := debug.Stack()

				// Log the panic with full context
				log.ErrorContext(r.Context(), "panic recovered in HTTP handler",
					logger.LogAttrsWithTrace(r.Context(),
						slog.String("request_id", logger.FromContext(r.Context())),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.String("remote_addr", r.RemoteAddr),
						slog.Any("panic", err),
						slog.String("stack", string(stack)),
					)...,
				)

				// Return 500 to client
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// RequestSizeLimitMiddleware enforces a maximum request body size to prevent
// DoS attacks via oversized payloads. Requests exceeding the limit receive 413.
func RequestSizeLimitMiddleware(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only limit body size for requests that have a body
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			// Wrap the request body with a size limiter
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}

		next.ServeHTTP(w, r)
	})
}

// WriteJSONError writes a JSON error response, safely encoding the message.
func WriteJSONError(w http.ResponseWriter, log *slog.Logger, message string, statusCode int) {
	body, err := json.Marshal(struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Message string `json:"message"`
	}{Message: message}})
	if err != nil {
		log.Error("failed to marshal error response",
			slog.String("original_message", message),
			slog.Int("status_code", statusCode),
			slog.String("error", err.Error()),
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}
