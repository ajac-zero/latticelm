package openai

import (
	"context"
	"testing"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/config"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.ProviderConfig
		validate func(t *testing.T, p *Provider)
	}{
		{
			name: "creates provider with API key",
			cfg: config.ProviderConfig{
				APIKey: "sk-test-key",
				Model:  "gpt-4o",
			},
			validate: func(t *testing.T, p *Provider) {
				assert.NotNil(t, p)
				assert.NotNil(t, p.client)
				assert.Equal(t, "sk-test-key", p.cfg.APIKey)
				assert.Equal(t, "gpt-4o", p.cfg.Model)
				assert.False(t, p.azure)
			},
		},
		{
			name: "creates provider without API key",
			cfg: config.ProviderConfig{
				APIKey: "",
			},
			validate: func(t *testing.T, p *Provider) {
				assert.NotNil(t, p)
				assert.Nil(t, p.client)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.cfg)
			tt.validate(t, p)
		})
	}
}

func TestNewAzure(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.AzureOpenAIConfig
		validate func(t *testing.T, p *Provider)
	}{
		{
			name: "creates Azure provider with endpoint and API key",
			cfg: config.AzureOpenAIConfig{
				APIKey:     "azure-key",
				Endpoint:   "https://test.openai.azure.com",
				APIVersion: "2024-02-15-preview",
			},
			validate: func(t *testing.T, p *Provider) {
				assert.NotNil(t, p)
				assert.NotNil(t, p.client)
				assert.Equal(t, "azure-key", p.cfg.APIKey)
				assert.True(t, p.azure)
			},
		},
		{
			name: "creates Azure provider with default API version",
			cfg: config.AzureOpenAIConfig{
				APIKey:     "azure-key",
				Endpoint:   "https://test.openai.azure.com",
				APIVersion: "",
			},
			validate: func(t *testing.T, p *Provider) {
				assert.NotNil(t, p)
				assert.NotNil(t, p.client)
				assert.True(t, p.azure)
			},
		},
		{
			name: "creates Azure provider without API key",
			cfg: config.AzureOpenAIConfig{
				APIKey:   "",
				Endpoint: "https://test.openai.azure.com",
			},
			validate: func(t *testing.T, p *Provider) {
				assert.NotNil(t, p)
				assert.Nil(t, p.client)
				assert.True(t, p.azure)
			},
		},
		{
			name: "creates Azure provider without endpoint",
			cfg: config.AzureOpenAIConfig{
				APIKey:   "azure-key",
				Endpoint: "",
			},
			validate: func(t *testing.T, p *Provider) {
				assert.NotNil(t, p)
				assert.Nil(t, p.client)
				assert.True(t, p.azure)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAzure(tt.cfg)
			tt.validate(t, p)
		})
	}
}

func TestProvider_Name(t *testing.T) {
	p := New(config.ProviderConfig{})
	assert.Equal(t, "openai", p.Name())
}

func TestProvider_Generate_Validation(t *testing.T) {
	tests := []struct {
		name        string
		provider    *Provider
		messages    []api.Message
		req         *api.ResponseRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "returns error when API key missing",
			provider: &Provider{
				cfg:    config.ProviderConfig{APIKey: ""},
				client: nil,
			},
			messages: []api.Message{
				{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
			},
			req: &api.ResponseRequest{
				Model: "gpt-4o",
			},
			expectError: true,
			errorMsg:    "api key missing",
		},
		{
			name: "returns error when client not initialized",
			provider: &Provider{
				cfg:    config.ProviderConfig{APIKey: "sk-test"},
				client: nil,
			},
			messages: []api.Message{
				{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
			},
			req: &api.ResponseRequest{
				Model: "gpt-4o",
			},
			expectError: true,
			errorMsg:    "client not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.provider.Generate(context.Background(), tt.messages, tt.req)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestProvider_GenerateStream_Validation(t *testing.T) {
	tests := []struct {
		name        string
		provider    *Provider
		messages    []api.Message
		req         *api.ResponseRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "returns error when API key missing",
			provider: &Provider{
				cfg:    config.ProviderConfig{APIKey: ""},
				client: nil,
			},
			messages: []api.Message{
				{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
			},
			req: &api.ResponseRequest{
				Model: "gpt-4o",
			},
			expectError: true,
			errorMsg:    "api key missing",
		},
		{
			name: "returns error when client not initialized",
			provider: &Provider{
				cfg:    config.ProviderConfig{APIKey: "sk-test"},
				client: nil,
			},
			messages: []api.Message{
				{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
			},
			req: &api.ResponseRequest{
				Model: "gpt-4o",
			},
			expectError: true,
			errorMsg:    "client not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deltaChan, errChan := tt.provider.GenerateStream(context.Background(), tt.messages, tt.req)

			// Read from channels
			var receivedError error
			for {
				select {
				case _, ok := <-deltaChan:
					if !ok {
						deltaChan = nil
					}
				case err, ok := <-errChan:
					if ok && err != nil {
						receivedError = err
					}
					errChan = nil
				}

				if deltaChan == nil && errChan == nil {
					break
				}
			}

			if tt.expectError {
				require.Error(t, receivedError)
				if tt.errorMsg != "" {
					assert.Contains(t, receivedError.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, receivedError)
			}
		})
	}
}

func TestChooseModel(t *testing.T) {
	tests := []struct {
		name         string
		requested    string
		defaultModel string
		expected     string
	}{
		{
			name:         "returns requested model when provided",
			requested:    "gpt-4o",
			defaultModel: "gpt-4o-mini",
			expected:     "gpt-4o",
		},
		{
			name:         "returns default model when requested is empty",
			requested:    "",
			defaultModel: "gpt-4o-mini",
			expected:     "gpt-4o-mini",
		},
		{
			name:         "returns fallback when both empty",
			requested:    "",
			defaultModel: "",
			expected:     "gpt-4o-mini",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := chooseModel(tt.requested, tt.defaultModel)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractToolCalls_Integration(t *testing.T) {
	// Additional integration tests for extractToolCalls beyond convert_test.go
	t.Run("handles empty message", func(t *testing.T) {
		msg := openai.ChatCompletionMessage{}
		result := extractToolCalls(msg)
		assert.Nil(t, result)
	})
}
