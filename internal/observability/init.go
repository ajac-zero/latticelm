package observability

import (
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/providers"
	"github.com/prometheus/client_golang/prometheus"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ProviderRegistry defines the interface for provider registries.
// This matches the interface expected by the server.
type ProviderRegistry interface {
	Get(name string) (providers.Provider, bool)
	Models() []struct{ Provider, Model string }
	ResolveModelID(model string) string
	Default(model string) (providers.Provider, error)
}

// WrapProviderRegistry wraps all providers in a registry with observability.
func WrapProviderRegistry(registry ProviderRegistry, metricsRegistry *prometheus.Registry, tp *sdktrace.TracerProvider) ProviderRegistry {
	if registry == nil {
		return nil
	}

	// We can't directly modify the registry's internal map, so we'll need to
	// wrap providers as they're retrieved. Instead, create a new instrumented registry.
	return &InstrumentedRegistry{
		base:             registry,
		metrics:          metricsRegistry,
		tracer:           tp,
		wrappedProviders: make(map[string]providers.Provider),
	}
}

// InstrumentedRegistry wraps a provider registry to return instrumented providers.
type InstrumentedRegistry struct {
	base             ProviderRegistry
	metrics          *prometheus.Registry
	tracer           *sdktrace.TracerProvider
	wrappedProviders map[string]providers.Provider
}

// Get returns an instrumented provider by entry name.
func (r *InstrumentedRegistry) Get(name string) (providers.Provider, bool) {
	// Check if we've already wrapped this provider
	if wrapped, ok := r.wrappedProviders[name]; ok {
		return wrapped, true
	}

	// Get the base provider
	p, ok := r.base.Get(name)
	if !ok {
		return nil, false
	}

	// Wrap it
	wrapped := NewInstrumentedProvider(p, r.metrics, r.tracer)
	r.wrappedProviders[name] = wrapped
	return wrapped, true
}

// Default returns the instrumented provider for the given model name.
func (r *InstrumentedRegistry) Default(model string) (providers.Provider, error) {
	p, err := r.base.Default(model)
	if err != nil {
		return nil, err
	}

	// Check if we've already wrapped this provider
	name := p.Name()
	if wrapped, ok := r.wrappedProviders[name]; ok {
		return wrapped, nil
	}

	// Wrap it
	wrapped := NewInstrumentedProvider(p, r.metrics, r.tracer)
	r.wrappedProviders[name] = wrapped
	return wrapped, nil
}

// Models returns the list of configured models and their provider entry names.
func (r *InstrumentedRegistry) Models() []struct{ Provider, Model string } {
	return r.base.Models()
}

// ResolveModelID returns the provider_model_id for a model.
func (r *InstrumentedRegistry) ResolveModelID(model string) string {
	return r.base.ResolveModelID(model)
}

// WrapConversationStore wraps a conversation store with observability.
func WrapConversationStore(store conversation.Store, backend string, metricsRegistry *prometheus.Registry, tp *sdktrace.TracerProvider) conversation.Store {
	if store == nil {
		return nil
	}

	return NewInstrumentedStore(store, backend, metricsRegistry, tp)
}
