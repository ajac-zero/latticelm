package observability

import (
	"context"
	"fmt"

	"github.com/ajac-zero/latticelm/internal/config"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// InitTracer initializes the OpenTelemetry tracer provider.
func InitTracer(cfg config.TracingConfig) (*sdktrace.TracerProvider, error) {
	// Create resource with service information
	// Use NewSchemaless to avoid schema version conflicts
	res := resource.NewSchemaless(
		semconv.ServiceName(cfg.ServiceName),
	)

	// Create exporter
	var exporter sdktrace.SpanExporter
	var err error
	switch cfg.Exporter.Type {
	case "otlp":
		exporter, err = createOTLPExporter(cfg.Exporter)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
	case "stdout":
		exporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported exporter type: %s", cfg.Exporter.Type)
	}

	// Create sampler
	sampler := createSampler(cfg.Sampler)

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	return tp, nil
}

// createOTLPExporter creates an OTLP gRPC exporter.
func createOTLPExporter(cfg config.ExporterConfig) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()))
	}

	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(cfg.Headers))
	}

	// Add dial options to ensure connection
	opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithBlock()))

	return otlptracegrpc.New(context.Background(), opts...)
}

// createSampler creates a sampler based on the configuration.
func createSampler(cfg config.SamplerConfig) sdktrace.Sampler {
	switch cfg.Type {
	case "always":
		return sdktrace.AlwaysSample()
	case "never":
		return sdktrace.NeverSample()
	case "probability":
		return sdktrace.TraceIDRatioBased(cfg.Rate)
	default:
		// Default to 10% sampling
		return sdktrace.TraceIDRatioBased(0.1)
	}
}

// Shutdown gracefully shuts down the tracer provider.
func Shutdown(ctx context.Context, tp *sdktrace.TracerProvider) error {
	if tp == nil {
		return nil
	}
	return tp.Shutdown(ctx)
}
