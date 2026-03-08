package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/config"
	anthropicprovider "github.com/ajac-zero/latticelm/internal/providers/anthropic"
	googleprovider "github.com/ajac-zero/latticelm/internal/providers/google"
	openaiprovider "github.com/ajac-zero/latticelm/internal/providers/openai"
)

// Provider represents a unified interface that each LLM provider must implement.
type Provider interface {
	Name() string
	Generate(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error)
	GenerateStream(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error)
}

// Registry keeps track of registered providers and model-to-provider mappings.
type Registry struct {
	providers        map[string]Provider
	models           map[string]string // model name -> provider entry name
	providerModelIDs map[string]string // model name -> provider model ID
	modelList        []config.ModelEntry
}

// NewRegistry constructs provider implementations from configuration.
func NewRegistry(entries map[string]config.ProviderEntry, models []config.ModelEntry) (*Registry, error) {
	return NewRegistryWithCircuitBreaker(entries, models, nil)
}

// NewRegistryWithCircuitBreaker constructs provider implementations with circuit breaker support.
// The onStateChange callback is invoked when circuit breaker state changes.
func NewRegistryWithCircuitBreaker(
	entries map[string]config.ProviderEntry,
	models []config.ModelEntry,
	onStateChange func(provider, from, to string),
) (*Registry, error) {
	reg := &Registry{
		providers:        make(map[string]Provider),
		models:           make(map[string]string),
		providerModelIDs: make(map[string]string),
		modelList:        models,
	}

	for name, entry := range entries {
		p, err := buildProvider(entry)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
		cbConfig := circuitBreakerConfigFromEntry(entry, onStateChange)
		reg.providers[name] = NewCircuitBreakerProvider(p, cbConfig)
	}

	for _, m := range models {
		reg.models[m.Name] = m.Provider
		if m.ProviderModelID != "" {
			reg.providerModelIDs[m.Name] = m.ProviderModelID
		}
	}

	if len(reg.providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}

	return reg, nil
}

func buildProvider(entry config.ProviderEntry) (Provider, error) {
	// Vertex AI uses Application Default Credentials, not an API key.
	if entry.Type != "vertexai" && entry.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for provider type %q", entry.Type)
	}

	switch entry.Type {
	case "openai":
		return openaiprovider.New(config.ProviderConfig{
			APIKey:   entry.APIKey,
			Endpoint: entry.Endpoint,
		}), nil
	case "azureopenai":
		if entry.Endpoint == "" {
			return nil, fmt.Errorf("endpoint is required for azureopenai")
		}
		return openaiprovider.NewAzure(config.AzureOpenAIConfig{
			APIKey:     entry.APIKey,
			Endpoint:   entry.Endpoint,
			APIVersion: entry.APIVersion,
		}), nil
	case "anthropic":
		return anthropicprovider.New(config.ProviderConfig{
			APIKey:   entry.APIKey,
			Endpoint: entry.Endpoint,
		}), nil
	case "azureanthropic":
		if entry.Endpoint == "" {
			return nil, fmt.Errorf("endpoint is required for azureanthropic")
		}
		return anthropicprovider.NewAzure(config.AzureAnthropicConfig{
			APIKey:   entry.APIKey,
			Endpoint: entry.Endpoint,
		}), nil
	case "google":
		return googleprovider.New(config.ProviderConfig{
			APIKey:   entry.APIKey,
			Endpoint: entry.Endpoint,
		})
	case "vertexai":
		if entry.Project == "" || entry.Location == "" {
			return nil, fmt.Errorf("project and location are required for vertexai")
		}
		return googleprovider.NewVertexAI(config.VertexAIConfig{
			Project:  entry.Project,
			Location: entry.Location,
		})
	default:
		return nil, fmt.Errorf("unknown provider type %q", entry.Type)
	}
}

// Get returns provider by entry name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Models returns the list of configured models and their provider entry names.
func (r *Registry) Models() []struct{ Provider, Model string } {
	var out []struct{ Provider, Model string }
	for _, m := range r.modelList {
		if _, ok := r.providers[m.Provider]; !ok {
			continue
		}
		out = append(out, struct{ Provider, Model string }{Provider: m.Provider, Model: m.Name})
	}
	return out
}

// ResolveModelID returns the provider_model_id for a model, falling back to the model name itself.
func (r *Registry) ResolveModelID(model string) string {
	if id, ok := r.providerModelIDs[model]; ok {
		return id
	}
	return model
}

// circuitBreakerConfigFromEntry builds a CircuitBreakerConfig from a ProviderEntry,
// applying per-provider overrides on top of the defaults.
func circuitBreakerConfigFromEntry(entry config.ProviderEntry, onStateChange func(provider, from, to string)) CircuitBreakerConfig {
	cfg := DefaultCircuitBreakerConfig()
	cfg.OnStateChange = onStateChange

	cb := entry.CircuitBreaker
	if cb == nil {
		return cfg
	}
	if cb.MaxRequests != 0 {
		cfg.MaxRequests = cb.MaxRequests
	}
	if cb.MinRequests != 0 {
		cfg.MinRequests = cb.MinRequests
	}
	if cb.FailureRatio != 0 {
		cfg.FailureRatio = cb.FailureRatio
	}
	if cb.Interval != "" {
		if d, err := time.ParseDuration(cb.Interval); err == nil {
			cfg.Interval = d
		}
	}
	if cb.Timeout != "" {
		if d, err := time.ParseDuration(cb.Timeout); err == nil {
			cfg.Timeout = d
		}
	}
	return cfg
}

// Default returns the provider for the given model name.
func (r *Registry) Default(model string) (Provider, error) {
	if model != "" {
		if providerName, ok := r.models[model]; ok {
			if p, ok := r.providers[providerName]; ok {
				return p, nil
			}
			return nil, fmt.Errorf("model %q is mapped to provider %q, but that provider is not available", model, providerName)
		}
		return nil, fmt.Errorf("model %q not configured", model)
	}

	for _, p := range r.providers {
		return p, nil
	}

	return nil, fmt.Errorf("no providers available")
}
