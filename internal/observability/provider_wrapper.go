package observability

import (
	"context"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/providers"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// InstrumentedProvider wraps a provider with metrics and tracing.
type InstrumentedProvider struct {
	base     providers.Provider
	registry *prometheus.Registry
	tracer   trace.Tracer
}

// NewInstrumentedProvider wraps a provider with observability.
func NewInstrumentedProvider(p providers.Provider, registry *prometheus.Registry, tp *sdktrace.TracerProvider) providers.Provider {
	var tracer trace.Tracer
	if tp != nil {
		tracer = tp.Tracer("llm-gateway")
	}

	return &InstrumentedProvider{
		base:     p,
		registry: registry,
		tracer:   tracer,
	}
}

// Name returns the name of the underlying provider.
func (p *InstrumentedProvider) Name() string {
	return p.base.Name()
}

// Generate wraps the provider's Generate method with metrics and tracing.
func (p *InstrumentedProvider) Generate(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
	// Start span if tracing is enabled
	if p.tracer != nil {
		var span trace.Span
		ctx, span = p.tracer.Start(ctx, "provider.generate",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("provider.name", p.base.Name()),
				attribute.String("provider.model", req.Model),
			),
		)
		defer span.End()
	}

	// Record start time
	start := time.Now()

	// Call underlying provider
	result, err := p.base.Generate(ctx, messages, req)

	// Record metrics
	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
		if p.tracer != nil {
			span := trace.SpanFromContext(ctx)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	} else if result != nil {
		// Add token attributes to span
		if p.tracer != nil {
			span := trace.SpanFromContext(ctx)
			span.SetAttributes(
				attribute.Int64("provider.input_tokens", int64(result.Usage.InputTokens)),
				attribute.Int64("provider.output_tokens", int64(result.Usage.OutputTokens)),
				attribute.Int64("provider.total_tokens", int64(result.Usage.TotalTokens)),
			)
			span.SetStatus(codes.Ok, "")
		}

		// Record token metrics
		if p.registry != nil {
			providerTokensTotal.WithLabelValues(p.base.Name(), req.Model, "input").Add(float64(result.Usage.InputTokens))
			providerTokensTotal.WithLabelValues(p.base.Name(), req.Model, "output").Add(float64(result.Usage.OutputTokens))
		}
	}

	// Record request metrics
	if p.registry != nil {
		providerRequestsTotal.WithLabelValues(p.base.Name(), req.Model, "generate", status).Inc()
		providerRequestDuration.WithLabelValues(p.base.Name(), req.Model, "generate").Observe(duration)
	}

	return result, err
}

// GenerateStream wraps the provider's GenerateStream method with metrics and tracing.
func (p *InstrumentedProvider) GenerateStream(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
	// Start span if tracing is enabled
	if p.tracer != nil {
		var span trace.Span
		ctx, span = p.tracer.Start(ctx, "provider.generate_stream",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(
				attribute.String("provider.name", p.base.Name()),
				attribute.String("provider.model", req.Model),
			),
		)
		defer span.End()
	}

	// Record start time
	start := time.Now()
	var ttfb time.Duration
	firstChunk := true

	// Create instrumented channels
	baseChan, baseErrChan := p.base.GenerateStream(ctx, messages, req)
	outChan := make(chan *api.ProviderStreamDelta)
	outErrChan := make(chan error, 1)

	// Metrics tracking
	var chunkCount int64
	var totalInputTokens, totalOutputTokens int64
	var streamErr error

	go func() {
		defer close(outChan)
		defer close(outErrChan)

		for {
			select {
			case delta, ok := <-baseChan:
				if !ok {
					// Stream finished - record final metrics
					duration := time.Since(start).Seconds()
					status := "success"
					if streamErr != nil {
						status = "error"
						if p.tracer != nil {
							span := trace.SpanFromContext(ctx)
							span.RecordError(streamErr)
							span.SetStatus(codes.Error, streamErr.Error())
						}
					} else {
						if p.tracer != nil {
							span := trace.SpanFromContext(ctx)
							span.SetAttributes(
								attribute.Int64("provider.input_tokens", totalInputTokens),
								attribute.Int64("provider.output_tokens", totalOutputTokens),
								attribute.Int64("provider.chunk_count", chunkCount),
								attribute.Float64("provider.ttfb_seconds", ttfb.Seconds()),
							)
							span.SetStatus(codes.Ok, "")
						}

						// Record token metrics
						if p.registry != nil && (totalInputTokens > 0 || totalOutputTokens > 0) {
							providerTokensTotal.WithLabelValues(p.base.Name(), req.Model, "input").Add(float64(totalInputTokens))
							providerTokensTotal.WithLabelValues(p.base.Name(), req.Model, "output").Add(float64(totalOutputTokens))
						}
					}

					// Record stream metrics
					if p.registry != nil {
						providerRequestsTotal.WithLabelValues(p.base.Name(), req.Model, "generate_stream", status).Inc()
						providerStreamDuration.WithLabelValues(p.base.Name(), req.Model).Observe(duration)
						providerStreamChunks.WithLabelValues(p.base.Name(), req.Model).Add(float64(chunkCount))
						if ttfb > 0 {
							providerStreamTTFB.WithLabelValues(p.base.Name(), req.Model).Observe(ttfb.Seconds())
						}
					}
					return
				}

				// Record TTFB on first chunk
				if firstChunk {
					ttfb = time.Since(start)
					firstChunk = false
				}

				chunkCount++

				// Track token usage
				if delta.Usage != nil {
					totalInputTokens = int64(delta.Usage.InputTokens)
					totalOutputTokens = int64(delta.Usage.OutputTokens)
				}

				// Forward the delta
				outChan <- delta

			case err, ok := <-baseErrChan:
				if ok && err != nil {
					streamErr = err
					outErrChan <- err
				}
				return
			}
		}
	}()

	return outChan, outErrChan
}
