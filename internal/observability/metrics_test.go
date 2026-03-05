package observability

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitMetrics(t *testing.T) {
	// Test that InitMetrics returns a non-nil registry
	registry := InitMetrics()
	require.NotNil(t, registry, "InitMetrics should return a non-nil registry")

	// Test that we can gather metrics from the registry (may be empty if no metrics recorded)
	metricFamilies, err := registry.Gather()
	require.NoError(t, err, "Gathering metrics should not error")

	// Just verify that the registry is functional
	// We cannot test specific metrics as they are package-level variables that may already be registered elsewhere
	_ = metricFamilies
}

func TestRecordCircuitBreakerStateChange(t *testing.T) {
	tests := []struct {
		name          string
		provider      string
		from          string
		to            string
		expectedState float64
	}{
		{
			name:          "transition to closed",
			provider:      "openai",
			from:          "open",
			to:            "closed",
			expectedState: 0,
		},
		{
			name:          "transition to open",
			provider:      "anthropic",
			from:          "closed",
			to:            "open",
			expectedState: 1,
		},
		{
			name:          "transition to half-open",
			provider:      "google",
			from:          "open",
			to:            "half-open",
			expectedState: 2,
		},
		{
			name:          "closed to half-open",
			provider:      "openai",
			from:          "closed",
			to:            "half-open",
			expectedState: 2,
		},
		{
			name:          "half-open to closed",
			provider:      "anthropic",
			from:          "half-open",
			to:            "closed",
			expectedState: 0,
		},
		{
			name:          "half-open to open",
			provider:      "google",
			from:          "half-open",
			to:            "open",
			expectedState: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics for this test
			circuitBreakerStateTransitions.Reset()
			circuitBreakerState.Reset()

			// Record the state change
			RecordCircuitBreakerStateChange(tt.provider, tt.from, tt.to)

			// Verify the transition counter was incremented
			transitionMetric := circuitBreakerStateTransitions.WithLabelValues(tt.provider, tt.from, tt.to)
			value := testutil.ToFloat64(transitionMetric)
			assert.Equal(t, 1.0, value, "transition counter should be incremented")

			// Verify the state gauge was set correctly
			stateMetric := circuitBreakerState.WithLabelValues(tt.provider)
			stateValue := testutil.ToFloat64(stateMetric)
			assert.Equal(t, tt.expectedState, stateValue, "state gauge should reflect new state")
		})
	}
}

func TestMetricLabels(t *testing.T) {
	// Initialize a fresh registry for testing
	registry := prometheus.NewRegistry()

	// Create new metric for testing labels
	testCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_counter",
			Help: "Test counter for label verification",
		},
		[]string{"label1", "label2"},
	)
	registry.MustRegister(testCounter)

	tests := []struct {
		name   string
		label1 string
		label2 string
		incr   float64
	}{
		{
			name:   "basic labels",
			label1: "value1",
			label2: "value2",
			incr:   1.0,
		},
		{
			name:   "different labels",
			label1: "foo",
			label2: "bar",
			incr:   5.0,
		},
		{
			name:   "empty labels",
			label1: "",
			label2: "",
			incr:   2.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := testCounter.WithLabelValues(tt.label1, tt.label2)
			counter.Add(tt.incr)

			value := testutil.ToFloat64(counter)
			assert.Equal(t, tt.incr, value, "counter value should match increment")
		})
	}
}

func TestHTTPMetrics(t *testing.T) {
	// Reset metrics
	httpRequestsTotal.Reset()
	httpRequestDuration.Reset()
	httpRequestSize.Reset()
	httpResponseSize.Reset()

	tests := []struct {
		name   string
		method string
		path   string
		status string
	}{
		{
			name:   "GET request",
			method: "GET",
			path:   "/api/v1/chat",
			status: "200",
		},
		{
			name:   "POST request",
			method: "POST",
			path:   "/api/v1/generate",
			status: "201",
		},
		{
			name:   "error response",
			method: "POST",
			path:   "/api/v1/chat",
			status: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate recording HTTP metrics
			httpRequestsTotal.WithLabelValues(tt.method, tt.path, tt.status).Inc()
			httpRequestDuration.WithLabelValues(tt.method, tt.path, tt.status).Observe(0.5)
			httpRequestSize.WithLabelValues(tt.method, tt.path).Observe(1024)
			httpResponseSize.WithLabelValues(tt.method, tt.path).Observe(2048)

			// Verify counter
			counter := httpRequestsTotal.WithLabelValues(tt.method, tt.path, tt.status)
			value := testutil.ToFloat64(counter)
			assert.Greater(t, value, 0.0, "request counter should be incremented")
		})
	}
}

func TestProviderMetrics(t *testing.T) {
	// Reset metrics
	providerRequestsTotal.Reset()
	providerRequestDuration.Reset()
	providerTokensTotal.Reset()
	providerStreamTTFB.Reset()
	providerStreamChunks.Reset()
	providerStreamDuration.Reset()

	tests := []struct {
		name      string
		provider  string
		model     string
		operation string
		status    string
	}{
		{
			name:      "OpenAI generate success",
			provider:  "openai",
			model:     "gpt-4",
			operation: "generate",
			status:    "success",
		},
		{
			name:      "Anthropic stream success",
			provider:  "anthropic",
			model:     "claude-3-sonnet",
			operation: "stream",
			status:    "success",
		},
		{
			name:      "Google generate error",
			provider:  "google",
			model:     "gemini-pro",
			operation: "generate",
			status:    "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate recording provider metrics
			providerRequestsTotal.WithLabelValues(tt.provider, tt.model, tt.operation, tt.status).Inc()
			providerRequestDuration.WithLabelValues(tt.provider, tt.model, tt.operation).Observe(1.5)
			providerTokensTotal.WithLabelValues(tt.provider, tt.model, "input").Add(100)
			providerTokensTotal.WithLabelValues(tt.provider, tt.model, "output").Add(50)

			if tt.operation == "stream" {
				providerStreamTTFB.WithLabelValues(tt.provider, tt.model).Observe(0.2)
				providerStreamChunks.WithLabelValues(tt.provider, tt.model).Add(10)
				providerStreamDuration.WithLabelValues(tt.provider, tt.model).Observe(2.0)
			}

			// Verify counter
			counter := providerRequestsTotal.WithLabelValues(tt.provider, tt.model, tt.operation, tt.status)
			value := testutil.ToFloat64(counter)
			assert.Greater(t, value, 0.0, "request counter should be incremented")

			// Verify token counts
			inputTokens := providerTokensTotal.WithLabelValues(tt.provider, tt.model, "input")
			inputValue := testutil.ToFloat64(inputTokens)
			assert.Greater(t, inputValue, 0.0, "input tokens should be recorded")

			outputTokens := providerTokensTotal.WithLabelValues(tt.provider, tt.model, "output")
			outputValue := testutil.ToFloat64(outputTokens)
			assert.Greater(t, outputValue, 0.0, "output tokens should be recorded")
		})
	}
}

func TestConversationStoreMetrics(t *testing.T) {
	// Reset metrics
	conversationOperationsTotal.Reset()
	conversationOperationDuration.Reset()
	conversationActiveCount.Reset()

	tests := []struct {
		name      string
		operation string
		backend   string
		status    string
	}{
		{
			name:      "create success",
			operation: "create",
			backend:   "redis",
			status:    "success",
		},
		{
			name:      "get success",
			operation: "get",
			backend:   "sql",
			status:    "success",
		},
		{
			name:      "delete error",
			operation: "delete",
			backend:   "memory",
			status:    "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate recording store metrics
			conversationOperationsTotal.WithLabelValues(tt.operation, tt.backend, tt.status).Inc()
			conversationOperationDuration.WithLabelValues(tt.operation, tt.backend).Observe(0.01)

			if tt.operation == "create" {
				conversationActiveCount.WithLabelValues(tt.backend).Inc()
			} else if tt.operation == "delete" {
				conversationActiveCount.WithLabelValues(tt.backend).Dec()
			}

			// Verify counter
			counter := conversationOperationsTotal.WithLabelValues(tt.operation, tt.backend, tt.status)
			value := testutil.ToFloat64(counter)
			assert.Greater(t, value, 0.0, "operation counter should be incremented")
		})
	}
}

func TestMetricHelp(t *testing.T) {
	registry := InitMetrics()
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	// Verify that all metrics have help text
	for _, mf := range metricFamilies {
		assert.NotEmpty(t, mf.GetHelp(), "metric %s should have help text", mf.GetName())
	}
}

func TestMetricTypes(t *testing.T) {
	registry := InitMetrics()
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	metricTypes := make(map[string]string)
	for _, mf := range metricFamilies {
		metricTypes[mf.GetName()] = mf.GetType().String()
	}

	// Verify counter metrics
	counterMetrics := []string{
		"http_requests_total",
		"provider_requests_total",
		"provider_tokens_total",
		"provider_stream_chunks_total",
		"conversation_operations_total",
		"circuit_breaker_state_transitions_total",
	}
	for _, metric := range counterMetrics {
		assert.Equal(t, "COUNTER", metricTypes[metric], "metric %s should be a counter", metric)
	}

	// Verify histogram metrics
	histogramMetrics := []string{
		"http_request_duration_seconds",
		"http_request_size_bytes",
		"http_response_size_bytes",
		"provider_request_duration_seconds",
		"provider_stream_ttfb_seconds",
		"provider_stream_duration_seconds",
		"conversation_operation_duration_seconds",
	}
	for _, metric := range histogramMetrics {
		assert.Equal(t, "HISTOGRAM", metricTypes[metric], "metric %s should be a histogram", metric)
	}

	// Verify gauge metrics
	gaugeMetrics := []string{
		"conversation_active_count",
		"circuit_breaker_state",
	}
	for _, metric := range gaugeMetrics {
		assert.Equal(t, "GAUGE", metricTypes[metric], "metric %s should be a gauge", metric)
	}
}

func TestCircuitBreakerInvalidState(t *testing.T) {
	// Reset metrics
	circuitBreakerState.Reset()
	circuitBreakerStateTransitions.Reset()

	// Record a state change with an unknown target state
	RecordCircuitBreakerStateChange("test-provider", "closed", "unknown")

	// The transition should still be recorded
	transitionMetric := circuitBreakerStateTransitions.WithLabelValues("test-provider", "closed", "unknown")
	value := testutil.ToFloat64(transitionMetric)
	assert.Equal(t, 1.0, value, "transition should be recorded even for unknown state")

	// The state gauge should be 0 (default for unknown states)
	stateMetric := circuitBreakerState.WithLabelValues("test-provider")
	stateValue := testutil.ToFloat64(stateMetric)
	assert.Equal(t, 0.0, stateValue, "unknown state should default to 0")
}

func TestMetricNaming(t *testing.T) {
	registry := InitMetrics()
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	// Verify metric naming conventions
	for _, mf := range metricFamilies {
		name := mf.GetName()

		// Counter metrics should end with _total
		if strings.HasSuffix(name, "_total") {
			assert.Equal(t, "COUNTER", mf.GetType().String(), "metric %s ends with _total but is not a counter", name)
		}

		// Duration metrics should end with _seconds
		if strings.Contains(name, "duration") {
			assert.True(t, strings.HasSuffix(name, "_seconds"), "duration metric %s should end with _seconds", name)
		}

		// Size metrics should end with _bytes
		if strings.Contains(name, "size") {
			assert.True(t, strings.HasSuffix(name, "_bytes"), "size metric %s should end with _bytes", name)
		}
	}
}
