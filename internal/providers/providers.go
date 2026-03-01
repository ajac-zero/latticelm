package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourusername/go-llm-gateway/internal/api"
	"github.com/yourusername/go-llm-gateway/internal/config"
	anthropicprovider "github.com/yourusername/go-llm-gateway/internal/providers/anthropic"
	googleprovider "github.com/yourusername/go-llm-gateway/internal/providers/google"
	openaiprovider "github.com/yourusername/go-llm-gateway/internal/providers/openai"
)

// Provider represents a unified interface that each LLM provider must implement.
type Provider interface {
	Name() string
	Generate(ctx context.Context, req *api.ResponseRequest) (*api.Response, error)
	GenerateStream(ctx context.Context, req *api.ResponseRequest) (<-chan *api.StreamChunk, <-chan error)
}

// Registry keeps track of registered providers by key (e.g. "openai").
type Registry struct {
	providers map[string]Provider
}

// NewRegistry constructs provider implementations from configuration.
func NewRegistry(cfg config.ProvidersConfig) (*Registry, error) {
	reg := &Registry{providers: make(map[string]Provider)}

	if cfg.Google.APIKey != "" {
		reg.providers[googleprovider.Name] = googleprovider.New(cfg.Google)
	}
	if cfg.AzureAnthropic.APIKey != "" && cfg.AzureAnthropic.Endpoint != "" {
		reg.providers[anthropicprovider.Name] = anthropicprovider.NewAzure(cfg.AzureAnthropic)
	} else if cfg.Anthropic.APIKey != "" {
		reg.providers[anthropicprovider.Name] = anthropicprovider.New(cfg.Anthropic)
	}
	if cfg.AzureOpenAI.APIKey != "" && cfg.AzureOpenAI.Endpoint != "" {
		reg.providers[openaiprovider.Name] = openaiprovider.NewAzure(cfg.AzureOpenAI)
	} else if cfg.OpenAI.APIKey != "" {
		reg.providers[openaiprovider.Name] = openaiprovider.New(cfg.OpenAI)
	}

	if len(reg.providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}

	return reg, nil
}

// Get returns provider by key.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Default returns provider based on inferred name.
func (r *Registry) Default(model string) (Provider, error) {
	if model != "" {
		switch {
		case strings.HasPrefix(model, "gpt") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3"):
			if p, ok := r.providers[openaiprovider.Name]; ok {
				return p, nil
			}
		case strings.HasPrefix(model, "claude"):
			if p, ok := r.providers[anthropicprovider.Name]; ok {
				return p, nil
			}
		case strings.HasPrefix(model, "gemini"):
			if p, ok := r.providers[googleprovider.Name]; ok {
				return p, nil
			}
		}
	}

	for _, p := range r.providers {
		return p, nil
	}

	return nil, fmt.Errorf("no providers available")
}
