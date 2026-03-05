package anthropic

import (
	"context"
	"testing"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/config"
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
				APIKey: "sk-ant-test-key",
				Model:  "claude-3-opus",
			},
			validate: func(t *testing.T, p *Provider) {
				assert.NotNil(t, p)
				assert.NotNil(t, p.client)
				assert.Equal(t, "sk-ant-test-key", p.cfg.APIKey)
				assert.Equal(t, "claude-3-opus", p.cfg.Model)
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
		cfg      config.AzureAnthropicConfig
		validate func(t *testing.T, p *Provider)
	}{
		{
			name: "creates Azure provider with endpoint and API key",
			cfg: config.AzureAnthropicConfig{
				APIKey:   "azure-key",
				Endpoint: "https://test.services.ai.azure.com/anthropic",
				Model:    "claude-3-sonnet",
			},
			validate: func(t *testing.T, p *Provider) {
				assert.NotNil(t, p)
				assert.NotNil(t, p.client)
				assert.Equal(t, "azure-key", p.cfg.APIKey)
				assert.Equal(t, "claude-3-sonnet", p.cfg.Model)
				assert.True(t, p.azure)
			},
		},
		{
			name: "creates Azure provider without API key",
			cfg: config.AzureAnthropicConfig{
				APIKey:   "",
				Endpoint: "https://test.services.ai.azure.com/anthropic",
			},
			validate: func(t *testing.T, p *Provider) {
				assert.NotNil(t, p)
				assert.Nil(t, p.client)
				assert.True(t, p.azure)
			},
		},
		{
			name: "creates Azure provider without endpoint",
			cfg: config.AzureAnthropicConfig{
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
	assert.Equal(t, "anthropic", p.Name())
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
				Model: "claude-3-opus",
			},
			expectError: true,
			errorMsg:    "api key missing",
		},
		{
			name: "returns error when client not initialized",
			provider: &Provider{
				cfg:    config.ProviderConfig{APIKey: "sk-ant-test"},
				client: nil,
			},
			messages: []api.Message{
				{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
			},
			req: &api.ResponseRequest{
				Model: "claude-3-opus",
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
				Model: "claude-3-opus",
			},
			expectError: true,
			errorMsg:    "api key missing",
		},
		{
			name: "returns error when client not initialized",
			provider: &Provider{
				cfg:    config.ProviderConfig{APIKey: "sk-ant-test"},
				client: nil,
			},
			messages: []api.Message{
				{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
			},
			req: &api.ResponseRequest{
				Model: "claude-3-opus",
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
			requested:    "claude-3-opus",
			defaultModel: "claude-3-sonnet",
			expected:     "claude-3-opus",
		},
		{
			name:         "returns default model when requested is empty",
			requested:    "",
			defaultModel: "claude-3-sonnet",
			expected:     "claude-3-sonnet",
		},
		{
			name:         "returns fallback when both empty",
			requested:    "",
			defaultModel: "",
			expected:     "claude-3-5-sonnet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := chooseModel(tt.requested, tt.defaultModel)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractToolCalls(t *testing.T) {
	// Note: This function is already tested in convert_test.go
	// This is a placeholder for additional integration tests if needed
	t.Run("returns nil for empty content", func(t *testing.T) {
		result := extractToolCalls(nil)
		assert.Nil(t, result)
	})
}
