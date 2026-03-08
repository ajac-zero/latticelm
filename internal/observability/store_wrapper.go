package observability

import (
	"context"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// InstrumentedStore wraps a conversation store with metrics and tracing.
type InstrumentedStore struct {
	base     conversation.Store
	registry *prometheus.Registry
	tracer   trace.Tracer
	backend  string
}

// NewInstrumentedStore wraps a conversation store with observability.
func NewInstrumentedStore(s conversation.Store, backend string, registry *prometheus.Registry, tp *sdktrace.TracerProvider) conversation.Store {
	var tracer trace.Tracer
	if tp != nil {
		tracer = tp.Tracer("llm-gateway")
	}

	// Initialize gauge with current size
	if registry != nil {
		conversationActiveCount.WithLabelValues(backend).Set(float64(s.Size()))
	}

	return &InstrumentedStore{
		base:     s,
		registry: registry,
		tracer:   tracer,
		backend:  backend,
	}
}

// Get wraps the store's Get method with metrics and tracing.
func (s *InstrumentedStore) Get(ctx context.Context, id string) (*conversation.Conversation, error) {
	// Start span if tracing is enabled
	if s.tracer != nil {
		var span trace.Span
		ctx, span = s.tracer.Start(ctx, "conversation.get",
			trace.WithAttributes(
				attribute.String("conversation.id", id),
				attribute.String("conversation.backend", s.backend),
			),
		)
		defer span.End()
	}

	// Record start time
	start := time.Now()

	// Call underlying store
	conv, err := s.base.Get(ctx, id)

	// Record metrics
	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
		if s.tracer != nil {
			span := trace.SpanFromContext(ctx)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	} else {
		if s.tracer != nil {
			span := trace.SpanFromContext(ctx)
			if conv != nil {
				span.SetAttributes(
					attribute.Int("conversation.message_count", len(conv.Messages)),
					attribute.String("conversation.model", conv.Model),
				)
			}
			span.SetStatus(codes.Ok, "")
		}
	}

	if s.registry != nil {
		conversationOperationsTotal.WithLabelValues("get", s.backend, status).Inc()
		conversationOperationDuration.WithLabelValues("get", s.backend).Observe(duration)
	}

	return conv, err
}

// Create wraps the store's Create method with metrics and tracing.
func (s *InstrumentedStore) Create(ctx context.Context, id string, model string, messages []api.Message, owner conversation.OwnerInfo) (*conversation.Conversation, error) {
	// Start span if tracing is enabled
	if s.tracer != nil {
		var span trace.Span
		ctx, span = s.tracer.Start(ctx, "conversation.create",
			trace.WithAttributes(
				attribute.String("conversation.id", id),
				attribute.String("conversation.backend", s.backend),
				attribute.String("conversation.model", model),
				attribute.Int("conversation.initial_messages", len(messages)),
			),
		)
		defer span.End()
	}

	// Record start time
	start := time.Now()

	// Call underlying store
	conv, err := s.base.Create(ctx, id, model, messages, owner)

	// Record metrics
	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
		if s.tracer != nil {
			span := trace.SpanFromContext(ctx)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	} else {
		if s.tracer != nil {
			span := trace.SpanFromContext(ctx)
			span.SetStatus(codes.Ok, "")
		}
	}

	if s.registry != nil {
		conversationOperationsTotal.WithLabelValues("create", s.backend, status).Inc()
		conversationOperationDuration.WithLabelValues("create", s.backend).Observe(duration)
		if err == nil {
			conversationActiveCount.WithLabelValues(s.backend).Inc()
		}
	}

	return conv, err
}

// Append wraps the store's Append method with metrics and tracing.
func (s *InstrumentedStore) Append(ctx context.Context, id string, messages ...api.Message) (*conversation.Conversation, error) {
	// Start span if tracing is enabled
	if s.tracer != nil {
		var span trace.Span
		ctx, span = s.tracer.Start(ctx, "conversation.append",
			trace.WithAttributes(
				attribute.String("conversation.id", id),
				attribute.String("conversation.backend", s.backend),
				attribute.Int("conversation.appended_messages", len(messages)),
			),
		)
		defer span.End()
	}

	// Record start time
	start := time.Now()

	// Call underlying store
	conv, err := s.base.Append(ctx, id, messages...)

	// Record metrics
	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
		if s.tracer != nil {
			span := trace.SpanFromContext(ctx)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	} else {
		if s.tracer != nil {
			span := trace.SpanFromContext(ctx)
			if conv != nil {
				span.SetAttributes(
					attribute.Int("conversation.total_messages", len(conv.Messages)),
				)
			}
			span.SetStatus(codes.Ok, "")
		}
	}

	if s.registry != nil {
		conversationOperationsTotal.WithLabelValues("append", s.backend, status).Inc()
		conversationOperationDuration.WithLabelValues("append", s.backend).Observe(duration)
	}

	return conv, err
}

// Delete wraps the store's Delete method with metrics and tracing.
func (s *InstrumentedStore) Delete(ctx context.Context, id string) error {
	// Start span if tracing is enabled
	if s.tracer != nil {
		var span trace.Span
		ctx, span = s.tracer.Start(ctx, "conversation.delete",
			trace.WithAttributes(
				attribute.String("conversation.id", id),
				attribute.String("conversation.backend", s.backend),
			),
		)
		defer span.End()
	}

	// Record start time
	start := time.Now()

	// Call underlying store
	err := s.base.Delete(ctx, id)

	// Record metrics
	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
		if s.tracer != nil {
			span := trace.SpanFromContext(ctx)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	} else {
		if s.tracer != nil {
			span := trace.SpanFromContext(ctx)
			span.SetStatus(codes.Ok, "")
		}
	}

	if s.registry != nil {
		conversationOperationsTotal.WithLabelValues("delete", s.backend, status).Inc()
		conversationOperationDuration.WithLabelValues("delete", s.backend).Observe(duration)
		if err == nil {
			conversationActiveCount.WithLabelValues(s.backend).Dec()
		}
	}

	return err
}

// Size returns the size of the underlying store.
func (s *InstrumentedStore) Size() int {
	return s.base.Size()
}

// Close wraps the store's Close method.
func (s *InstrumentedStore) Close() error {
	return s.base.Close()
}
