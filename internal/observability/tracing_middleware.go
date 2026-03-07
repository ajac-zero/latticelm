package observability

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TracingMiddleware creates a middleware that adds OpenTelemetry tracing to HTTP requests.
func TracingMiddleware(next http.Handler, tp *sdktrace.TracerProvider) http.Handler {
	if tp == nil {
		// If tracing is not enabled, pass through without modification
		return next
	}

	// Set up W3C Trace Context propagation
	otel.SetTextMapPropagator(propagation.TraceContext{})

	tracer := tp.Tracer("llm-gateway")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract trace context from incoming request headers
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Start a new span
		ctx, span := tracer.Start(ctx, "HTTP "+r.Method+" "+r.URL.Path,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", r.URL.Path),
				attribute.String("http.scheme", r.URL.Scheme),
				attribute.String("http.host", r.Host),
				attribute.String("http.user_agent", r.Header.Get("User-Agent")),
			),
		)
		defer span.End()

		// Add request ID to span if present
		if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
			span.SetAttributes(attribute.String("http.request_id", requestID))
		}

		// Create a response writer wrapper to capture status code
		wrapped := &statusResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Inject trace context into request for downstream services
		r = r.WithContext(ctx)

		// Call the next handler
		next.ServeHTTP(wrapped, r)

		// Record the status code in the span
		span.SetAttributes(attribute.Int("http.status_code", wrapped.statusCode))

		// Set span status based on HTTP status code
		if wrapped.statusCode >= 400 {
			span.SetStatus(codes.Error, http.StatusText(wrapped.statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}
	})
}

// statusResponseWriter wraps http.ResponseWriter to capture the status code.
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (w *statusResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.statusCode = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
