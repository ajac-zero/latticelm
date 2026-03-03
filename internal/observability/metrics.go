package observability

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// HTTP Metrics
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"method", "path", "status"},
	)

	httpRequestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_size_bytes",
			Help:    "HTTP request size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
		},
		[]string{"method", "path"},
	)

	httpResponseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
		},
		[]string{"method", "path"},
	)

	// Provider Metrics
	providerRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "provider_requests_total",
			Help: "Total number of provider requests",
		},
		[]string{"provider", "model", "operation", "status"},
	)

	providerRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "provider_request_duration_seconds",
			Help:    "Provider request latency in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 20, 30, 60},
		},
		[]string{"provider", "model", "operation"},
	)

	providerTokensTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "provider_tokens_total",
			Help: "Total number of tokens processed",
		},
		[]string{"provider", "model", "type"}, // type: input, output
	)

	providerStreamTTFB = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "provider_stream_ttfb_seconds",
			Help:    "Time to first byte for streaming requests in seconds",
			Buckets: []float64{0.05, 0.1, 0.5, 1, 2, 5, 10},
		},
		[]string{"provider", "model"},
	)

	providerStreamChunks = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "provider_stream_chunks_total",
			Help: "Total number of stream chunks received",
		},
		[]string{"provider", "model"},
	)

	providerStreamDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "provider_stream_duration_seconds",
			Help:    "Total duration of streaming requests in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 20, 30, 60},
		},
		[]string{"provider", "model"},
	)

	// Conversation Store Metrics
	conversationOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "conversation_operations_total",
			Help: "Total number of conversation store operations",
		},
		[]string{"operation", "backend", "status"},
	)

	conversationOperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "conversation_operation_duration_seconds",
			Help:    "Conversation store operation latency in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		},
		[]string{"operation", "backend"},
	)

	conversationActiveCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "conversation_active_count",
			Help: "Number of active conversations",
		},
		[]string{"backend"},
	)
)

// InitMetrics registers all metrics with a new Prometheus registry.
func InitMetrics() *prometheus.Registry {
	registry := prometheus.NewRegistry()

	// Register HTTP metrics
	registry.MustRegister(httpRequestsTotal)
	registry.MustRegister(httpRequestDuration)
	registry.MustRegister(httpRequestSize)
	registry.MustRegister(httpResponseSize)

	// Register provider metrics
	registry.MustRegister(providerRequestsTotal)
	registry.MustRegister(providerRequestDuration)
	registry.MustRegister(providerTokensTotal)
	registry.MustRegister(providerStreamTTFB)
	registry.MustRegister(providerStreamChunks)
	registry.MustRegister(providerStreamDuration)

	// Register conversation store metrics
	registry.MustRegister(conversationOperationsTotal)
	registry.MustRegister(conversationOperationDuration)
	registry.MustRegister(conversationActiveCount)

	return registry
}
