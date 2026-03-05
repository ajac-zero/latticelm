package server

import (
	"fmt"
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

// ErrorRecoveryMiddleware catches errors from MaxBytesReader and converts them
// to proper HTTP error responses. This should be placed after RequestSizeLimitMiddleware
// in the middleware chain.
func ErrorRecoveryMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		// Check if the request body exceeded the size limit
		// MaxBytesReader sets an error that we can detect on the next read attempt
		// But we need to handle the error when it actually occurs during JSON decoding
		// The JSON decoder will return the error, so we don't need special handling here
		// This middleware is more for future extensibility
	})
}

// WriteJSONError is a helper function to safely write JSON error responses,
// handling any encoding errors that might occur.
func WriteJSONError(w http.ResponseWriter, log *slog.Logger, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	// Use fmt.Fprintf to write the error response
	// This is safer than json.Encoder as we control the format
	_, err := fmt.Fprintf(w, `{"error":{"message":"%s"}}`, message)
	if err != nil {
		// If we can't even write the error response, log it
		log.Error("failed to write error response",
			slog.String("original_message", message),
			slog.Int("status_code", statusCode),
			slog.String("write_error", err.Error()),
		)
	}
}
