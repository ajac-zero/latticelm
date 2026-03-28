package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPanicRecoveryMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		handler        http.HandlerFunc
		expectPanic    bool
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "no panic - request succeeds",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("success"))
			},
			expectPanic:    false,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
		{
			name: "panic with string - recovers gracefully",
			handler: func(w http.ResponseWriter, r *http.Request) {
				panic("something went wrong")
			},
			expectPanic:    true,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Internal Server Error\n",
		},
		{
			name: "panic with error - recovers gracefully",
			handler: func(w http.ResponseWriter, r *http.Request) {
				panic(io.ErrUnexpectedEOF)
			},
			expectPanic:    true,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Internal Server Error\n",
		},
		{
			name: "panic with struct - recovers gracefully",
			handler: func(w http.ResponseWriter, r *http.Request) {
				panic(struct{ msg string }{msg: "bad things"})
			},
			expectPanic:    true,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Internal Server Error\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture logs
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))

			// Wrap the handler with panic recovery
			wrapped := PanicRecoveryMiddleware(tt.handler, logger)

			// Create request and recorder
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			// Execute the handler (should not panic even if inner handler does)
			wrapped.ServeHTTP(rec, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, rec.Code)
			assert.Equal(t, tt.expectedBody, rec.Body.String())

			// Verify logging if panic was expected
			if tt.expectPanic {
				logOutput := buf.String()
				assert.Contains(t, logOutput, "panic recovered in HTTP handler")
				assert.Contains(t, logOutput, "stack")
			}
		})
	}
}

func TestRequestSizeLimitMiddleware(t *testing.T) {
	const maxSize = 100 // 100 bytes for testing

	tests := []struct {
		name           string
		method         string
		bodySize       int
		expectedStatus int
		shouldSucceed  bool
	}{
		{
			name:           "small POST request - succeeds",
			method:         http.MethodPost,
			bodySize:       50,
			expectedStatus: http.StatusOK,
			shouldSucceed:  true,
		},
		{
			name:           "exact size POST request - succeeds",
			method:         http.MethodPost,
			bodySize:       maxSize,
			expectedStatus: http.StatusOK,
			shouldSucceed:  true,
		},
		{
			name:           "oversized POST request - fails",
			method:         http.MethodPost,
			bodySize:       maxSize + 1,
			expectedStatus: http.StatusBadRequest,
			shouldSucceed:  false,
		},
		{
			name:           "large POST request - fails",
			method:         http.MethodPost,
			bodySize:       maxSize * 2,
			expectedStatus: http.StatusBadRequest,
			shouldSucceed:  false,
		},
		{
			name:           "oversized PUT request - fails",
			method:         http.MethodPut,
			bodySize:       maxSize + 1,
			expectedStatus: http.StatusBadRequest,
			shouldSucceed:  false,
		},
		{
			name:           "oversized PATCH request - fails",
			method:         http.MethodPatch,
			bodySize:       maxSize + 1,
			expectedStatus: http.StatusBadRequest,
			shouldSucceed:  false,
		},
		{
			name:           "GET request - no size limit applied",
			method:         http.MethodGet,
			bodySize:       maxSize + 1,
			expectedStatus: http.StatusOK,
			shouldSucceed:  true,
		},
		{
			name:           "DELETE request - no size limit applied",
			method:         http.MethodDelete,
			bodySize:       maxSize + 1,
			expectedStatus: http.StatusOK,
			shouldSucceed:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a handler that tries to read the body
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, "read %d bytes", len(body))
			})

			// Wrap with size limit middleware
			wrapped := RequestSizeLimitMiddleware(handler, maxSize)

			// Create request with body of specified size
			bodyContent := strings.Repeat("a", tt.bodySize)
			req := httptest.NewRequest(tt.method, "/test", strings.NewReader(bodyContent))
			rec := httptest.NewRecorder()

			// Execute
			wrapped.ServeHTTP(rec, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.shouldSucceed {
				assert.Contains(t, rec.Body.String(), "read")
			} else {
				// For methods with body, should get an error
				assert.NotContains(t, rec.Body.String(), "read")
			}
		})
	}
}

func TestRequestSizeLimitMiddleware_WithJSONDecoding(t *testing.T) {
	const maxSize = 1024 // 1KB

	tests := []struct {
		name           string
		payload        interface{}
		expectedStatus int
		shouldDecode   bool
	}{
		{
			name: "small JSON payload - succeeds",
			payload: map[string]string{
				"message": "hello",
			},
			expectedStatus: http.StatusOK,
			shouldDecode:   true,
		},
		{
			name: "large JSON payload - fails",
			payload: map[string]string{
				"message": strings.Repeat("x", maxSize+100),
			},
			expectedStatus: http.StatusBadRequest,
			shouldDecode:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a handler that decodes JSON
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var data map[string]string
				if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "decoded"})
			})

			// Wrap with size limit middleware
			wrapped := RequestSizeLimitMiddleware(handler, maxSize)

			// Create request
			body, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			// Execute
			wrapped.ServeHTTP(rec, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.shouldDecode {
				assert.Contains(t, rec.Body.String(), "decoded")
			}
		})
	}
}

func TestWriteJSONError(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		statusCode   int
		expectedBody string
	}{
		{
			name:         "simple error message",
			message:      "something went wrong",
			statusCode:   http.StatusBadRequest,
			expectedBody: `{"error":{"message":"something went wrong","type":"invalid_request"}}`,
		},
		{
			name:         "internal server error",
			message:      "internal error",
			statusCode:   http.StatusInternalServerError,
			expectedBody: `{"error":{"message":"internal error","type":"server_error"}}`,
		},
		{
			name:         "unauthorized error",
			message:      "unauthorized",
			statusCode:   http.StatusUnauthorized,
			expectedBody: `{"error":{"message":"unauthorized","type":"server_error"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))

			rec := httptest.NewRecorder()
			WriteJSONError(rec, logger, tt.message, tt.statusCode)

			assert.Equal(t, tt.statusCode, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.Equal(t, tt.expectedBody, rec.Body.String())
		})
	}
}

func TestPanicRecoveryMiddleware_Integration(t *testing.T) {
	// Test that panic recovery works in a more realistic scenario
	// with multiple middleware layers
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	// Create a chain of middleware
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a panic deep in the stack
		panic("unexpected error in business logic")
	})

	// Wrap with multiple middleware layers
	wrapped := PanicRecoveryMiddleware(
		RequestSizeLimitMiddleware(
			finalHandler,
			1024,
		),
		logger,
	)

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("test"))
	rec := httptest.NewRecorder()

	// Should not panic
	wrapped.ServeHTTP(rec, req)

	// Should return 500
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "Internal Server Error\n", rec.Body.String())

	// Should log the panic
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "panic recovered")
	assert.Contains(t, logOutput, "unexpected error in business logic")
}
