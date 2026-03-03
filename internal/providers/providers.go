package providers

import (
	"context"
	"fmt"

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
		if p != nil {
			reg.providers[name] = p
		}
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
	// Vertex AI doesn't require APIKey, so check for it separately
	if entry.Type != "vertexai" && entry.APIKey == "" {
		return nil, nil
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

// Default returns the provider for the given model name.
func (r *Registry) Default(model string) (Provider, error) {
	if model != "" {
		if providerName, ok := r.models[model]; ok {
			if p, ok := r.providers[providerName]; ok {
				return p, nil
			}
		}
	}

	for _, p := range r.providers {
		return p, nil
	}

	return nil, fmt.Errorf("no providers available")
}
