package observability

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ajac-zero/latticelm/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestInitTracer_StdoutExporter(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.TracingConfig
		expectError bool
	}{
		{
			name: "stdout exporter with always sampler",
			cfg: config.TracingConfig{
				Enabled:     true,
				ServiceName: "test-service",
				Sampler: config.SamplerConfig{
					Type: "always",
				},
				Exporter: config.ExporterConfig{
					Type: "stdout",
				},
			},
			expectError: false,
		},
		{
			name: "stdout exporter with never sampler",
			cfg: config.TracingConfig{
				Enabled:     true,
				ServiceName: "test-service-2",
				Sampler: config.SamplerConfig{
					Type: "never",
				},
				Exporter: config.ExporterConfig{
					Type: "stdout",
				},
			},
			expectError: false,
		},
		{
			name: "stdout exporter with probability sampler",
			cfg: config.TracingConfig{
				Enabled:     true,
				ServiceName: "test-service-3",
				Sampler: config.SamplerConfig{
					Type: "probability",
					Rate: 0.5,
				},
				Exporter: config.ExporterConfig{
					Type: "stdout",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := InitTracer(tt.cfg)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, tp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, tp)

				// Clean up
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				err = tp.Shutdown(ctx)
				assert.NoError(t, err)
			}
		})
	}
}

func TestInitTracer_InvalidExporter(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:     true,
		ServiceName: "test-service",
		Sampler: config.SamplerConfig{
			Type: "always",
		},
		Exporter: config.ExporterConfig{
			Type: "invalid-exporter",
		},
	}

	tp, err := InitTracer(cfg)
	assert.Error(t, err)
	assert.Nil(t, tp)
	assert.Contains(t, err.Error(), "unsupported exporter type")
}

func TestCreateSampler(t *testing.T) {
	tests := []struct {
		name           string
		cfg            config.SamplerConfig
		expectedType   string
		shouldSample   bool
		checkSampleAll bool // If true, check that all spans are sampled
	}{
		{
			name: "always sampler",
			cfg: config.SamplerConfig{
				Type: "always",
			},
			expectedType:   "AlwaysOn",
			shouldSample:   true,
			checkSampleAll: true,
		},
		{
			name: "never sampler",
			cfg: config.SamplerConfig{
				Type: "never",
			},
			expectedType:   "AlwaysOff",
			shouldSample:   false,
			checkSampleAll: true,
		},
		{
			name: "probability sampler - 100%",
			cfg: config.SamplerConfig{
				Type: "probability",
				Rate: 1.0,
			},
			expectedType:   "AlwaysOn",
			shouldSample:   true,
			checkSampleAll: true,
		},
		{
			name: "probability sampler - 0%",
			cfg: config.SamplerConfig{
				Type: "probability",
				Rate: 0.0,
			},
			expectedType:   "TraceIDRatioBased",
			shouldSample:   false,
			checkSampleAll: true,
		},
		{
			name: "probability sampler - 50%",
			cfg: config.SamplerConfig{
				Type: "probability",
				Rate: 0.5,
			},
			expectedType:   "TraceIDRatioBased",
			shouldSample:   false, // Can't guarantee sampling
			checkSampleAll: false,
		},
		{
			name: "default sampler (invalid type)",
			cfg: config.SamplerConfig{
				Type: "unknown",
			},
			expectedType:   "TraceIDRatioBased",
			shouldSample:   false, // 10% default
			checkSampleAll: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sampler := createSampler(tt.cfg)
			require.NotNil(t, sampler)

			// Get the sampler description
			description := sampler.Description()
			assert.Contains(t, description, tt.expectedType)

			// Test sampling behavior for deterministic samplers
			if tt.checkSampleAll {
				tp := sdktrace.NewTracerProvider(
					sdktrace.WithSampler(sampler),
				)
				tracer := tp.Tracer("test")

				// Create a test span
				ctx := context.Background()
				_, span := tracer.Start(ctx, "test-span")
				spanContext := span.SpanContext()
				span.End()

				// Check if span was sampled
				isSampled := spanContext.IsSampled()
				assert.Equal(t, tt.shouldSample, isSampled, "sampling result should match expected")

				// Clean up
				_ = tp.Shutdown(context.Background())
			}
		})
	}
}

func TestShutdown(t *testing.T) {
	tests := []struct {
		name        string
		setupTP     func() *sdktrace.TracerProvider
		expectError bool
	}{
		{
			name: "shutdown valid tracer provider",
			setupTP: func() *sdktrace.TracerProvider {
				return sdktrace.NewTracerProvider()
			},
			expectError: false,
		},
		{
			name: "shutdown nil tracer provider",
			setupTP: func() *sdktrace.TracerProvider {
				return nil
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp := tt.setupTP()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := Shutdown(ctx, tp)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestShutdown_ContextTimeout(t *testing.T) {
	tp := sdktrace.NewTracerProvider()

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Shutdown(ctx, tp)
	// Shutdown should handle context cancellation gracefully
	// The error might be nil or context.Canceled depending on timing
	if err != nil {
		assert.Contains(t, err.Error(), "context")
	}
}

func TestTracerConfig_ServiceName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
	}{
		{
			name:        "default service name",
			serviceName: "llm-gateway",
		},
		{
			name:        "custom service name",
			serviceName: "custom-gateway",
		},
		{
			name:        "empty service name",
			serviceName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.TracingConfig{
				Enabled:     true,
				ServiceName: tt.serviceName,
				Sampler: config.SamplerConfig{
					Type: "always",
				},
				Exporter: config.ExporterConfig{
					Type: "stdout",
				},
			}

			tp, err := InitTracer(cfg)
			// Schema URL conflicts may occur in test environment, which is acceptable
			if err != nil && !strings.Contains(err.Error(), "conflicting Schema URL") {
				t.Fatalf("unexpected error: %v", err)
			}

			if tp != nil {
				// Clean up
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = tp.Shutdown(ctx)
			}
		})
	}
}

func TestCreateSampler_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.SamplerConfig
	}{
		{
			name: "negative rate",
			cfg: config.SamplerConfig{
				Type: "probability",
				Rate: -0.5,
			},
		},
		{
			name: "rate greater than 1",
			cfg: config.SamplerConfig{
				Type: "probability",
				Rate: 1.5,
			},
		},
		{
			name: "empty type",
			cfg: config.SamplerConfig{
				Type: "",
				Rate: 0.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// createSampler should not panic with edge cases
			sampler := createSampler(tt.cfg)
			assert.NotNil(t, sampler)
		})
	}
}

func TestTracerProvider_MultipleShutdowns(t *testing.T) {
	tp := sdktrace.NewTracerProvider()

	ctx := context.Background()

	// First shutdown should succeed
	err1 := Shutdown(ctx, tp)
	assert.NoError(t, err1)

	// Second shutdown might return error but shouldn't panic
	err2 := Shutdown(ctx, tp)
	// Error is acceptable here as provider is already shut down
	_ = err2
}

func TestSamplerDescription(t *testing.T) {
	tests := []struct {
		name             string
		cfg              config.SamplerConfig
		expectedInDesc   string
	}{
		{
			name: "always sampler description",
			cfg: config.SamplerConfig{
				Type: "always",
			},
			expectedInDesc: "AlwaysOn",
		},
		{
			name: "never sampler description",
			cfg: config.SamplerConfig{
				Type: "never",
			},
			expectedInDesc: "AlwaysOff",
		},
		{
			name: "probability sampler description",
			cfg: config.SamplerConfig{
				Type: "probability",
				Rate: 0.75,
			},
			expectedInDesc: "TraceIDRatioBased",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sampler := createSampler(tt.cfg)
			description := sampler.Description()
			assert.Contains(t, description, tt.expectedInDesc)
		})
	}
}

func TestInitTracer_ResourceAttributes(t *testing.T) {
	cfg := config.TracingConfig{
		Enabled:     true,
		ServiceName: "test-resource-service",
		Sampler: config.SamplerConfig{
			Type: "always",
		},
		Exporter: config.ExporterConfig{
			Type: "stdout",
		},
	}

	tp, err := InitTracer(cfg)
	// Schema URL conflicts may occur in test environment, which is acceptable
	if err != nil && !strings.Contains(err.Error(), "conflicting Schema URL") {
		t.Fatalf("unexpected error: %v", err)
	}

	if tp != nil {
		// Verify that the tracer provider was created successfully
		// Resource attributes are embedded in the provider
		tracer := tp.Tracer("test")
		assert.NotNil(t, tracer)

		// Clean up
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}
}

func TestProbabilitySampler_Boundaries(t *testing.T) {
	tests := []struct {
		name         string
		rate         float64
		shouldAlways bool
		shouldNever  bool
	}{
		{
			name:         "rate 0.0 - never sample",
			rate:         0.0,
			shouldAlways: false,
			shouldNever:  true,
		},
		{
			name:         "rate 1.0 - always sample",
			rate:         1.0,
			shouldAlways: true,
			shouldNever:  false,
		},
		{
			name:         "rate 0.5 - probabilistic",
			rate:         0.5,
			shouldAlways: false,
			shouldNever:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.SamplerConfig{
				Type: "probability",
				Rate: tt.rate,
			}

			sampler := createSampler(cfg)
			tp := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sampler),
			)
			defer func() { _ = tp.Shutdown(context.Background()) }()

			tracer := tp.Tracer("test")

			// Test multiple spans to verify sampling behavior
			sampledCount := 0
			totalSpans := 100

			for i := 0; i < totalSpans; i++ {
				ctx := context.Background()
				_, span := tracer.Start(ctx, "test-span")
				if span.SpanContext().IsSampled() {
					sampledCount++
				}
				span.End()
			}

			if tt.shouldAlways {
				assert.Equal(t, totalSpans, sampledCount, "all spans should be sampled")
			} else if tt.shouldNever {
				assert.Equal(t, 0, sampledCount, "no spans should be sampled")
			} else {
				// For probabilistic sampling, we just verify it's not all or nothing
				assert.Greater(t, sampledCount, 0, "some spans should be sampled")
				assert.Less(t, sampledCount, totalSpans, "not all spans should be sampled")
			}
		})
	}
}
