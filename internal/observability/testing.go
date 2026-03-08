package observability

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// NewTestRegistry creates a new isolated Prometheus registry for testing
func NewTestRegistry() *prometheus.Registry {
	return prometheus.NewRegistry()
}

// NewTestTracer creates a no-op tracer for testing
func NewTestTracer() (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	res := resource.NewSchemaless(
		semconv.ServiceNameKey.String("test-service"),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp, exporter
}

// GetMetricValue extracts a metric value from a registry
func GetMetricValue(registry *prometheus.Registry, metricName string) (float64, error) {
	metrics, err := registry.Gather()
	if err != nil {
		return 0, err
	}

	for _, mf := range metrics {
		if mf.GetName() == metricName {
			if len(mf.GetMetric()) > 0 {
				m := mf.GetMetric()[0]
				if m.GetCounter() != nil {
					return m.GetCounter().GetValue(), nil
				}
				if m.GetGauge() != nil {
					return m.GetGauge().GetValue(), nil
				}
				if m.GetHistogram() != nil {
					return float64(m.GetHistogram().GetSampleCount()), nil
				}
			}
		}
	}

	return 0, nil
}

// CountMetricsWithName counts how many metrics match the given name
func CountMetricsWithName(registry *prometheus.Registry, metricName string) (int, error) {
	metrics, err := registry.Gather()
	if err != nil {
		return 0, err
	}

	for _, mf := range metrics {
		if mf.GetName() == metricName {
			return len(mf.GetMetric()), nil
		}
	}

	return 0, nil
}

// GetCounterValue is a helper to get counter values using testutil
func GetCounterValue(counter prometheus.Counter) float64 {
	return testutil.ToFloat64(counter)
}

// NewNoOpTracerProvider creates a tracer provider that discards all spans
func NewNoOpTracerProvider() *sdktrace.TracerProvider {
	return sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(&noOpExporter{})),
	)
}

// noOpExporter is an exporter that discards all spans
type noOpExporter struct{}

func (e *noOpExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *noOpExporter) Shutdown(context.Context) error {
	return nil
}

// ShutdownTracer is a helper to safely shutdown a tracer provider
func ShutdownTracer(tp *sdktrace.TracerProvider) error {
	if tp != nil {
		return tp.Shutdown(context.Background())
	}
	return nil
}

// NewTestExporter creates a test exporter that writes to the provided writer
type TestExporter struct {
}

func (e *TestExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *TestExporter) Shutdown(ctx context.Context) error {
	return nil
}
