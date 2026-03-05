package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ajac-zero/latticelm/internal/config"
)

func TestNewRegistry(t *testing.T) {
	tests := []struct {
		name        string
		entries     map[string]config.ProviderEntry
		models      []config.ModelEntry
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, reg *Registry)
	}{
		{
			name: "valid config with OpenAI",
			entries: map[string]config.ProviderEntry{
				"openai": {
					Type:   "openai",
					APIKey: "sk-test",
				},
			},
			models: []config.ModelEntry{
				{Name: "gpt-4", Provider: "openai"},
			},
			validate: func(t *testing.T, reg *Registry) {
				assert.Len(t, reg.providers, 1)
				assert.Contains(t, reg.providers, "openai")
				assert.Equal(t, "openai", reg.models["gpt-4"])
			},
		},
		{
			name: "valid config with multiple providers",
			entries: map[string]config.ProviderEntry{
				"openai": {
					Type:   "openai",
					APIKey: "sk-test",
				},
				"anthropic": {
					Type:   "anthropic",
					APIKey: "sk-ant-test",
				},
			},
			models: []config.ModelEntry{
				{Name: "gpt-4", Provider: "openai"},
				{Name: "claude-3", Provider: "anthropic"},
			},
			validate: func(t *testing.T, reg *Registry) {
				assert.Len(t, reg.providers, 2)
				assert.Contains(t, reg.providers, "openai")
				assert.Contains(t, reg.providers, "anthropic")
				assert.Equal(t, "openai", reg.models["gpt-4"])
				assert.Equal(t, "anthropic", reg.models["claude-3"])
			},
		},
		{
			name: "no providers returns error",
			entries: map[string]config.ProviderEntry{
				"openai": {
					Type:   "openai",
					APIKey: "", // Missing API key
				},
			},
			models:      []config.ModelEntry{},
			expectError: true,
			errorMsg:    "no providers configured",
		},
		{
			name: "Azure OpenAI without endpoint returns error",
			entries: map[string]config.ProviderEntry{
				"azure": {
					Type:   "azureopenai",
					APIKey: "test-key",
				},
			},
			models:      []config.ModelEntry{},
			expectError: true,
			errorMsg:    "endpoint is required",
		},
		{
			name: "Azure OpenAI with endpoint succeeds",
			entries: map[string]config.ProviderEntry{
				"azure": {
					Type:       "azureopenai",
					APIKey:     "test-key",
					Endpoint:   "https://test.openai.azure.com",
					APIVersion: "2024-02-15-preview",
				},
			},
			models: []config.ModelEntry{
				{Name: "gpt-4-azure", Provider: "azure"},
			},
			validate: func(t *testing.T, reg *Registry) {
				assert.Len(t, reg.providers, 1)
				assert.Contains(t, reg.providers, "azure")
			},
		},
		{
			name: "Azure Anthropic without endpoint returns error",
			entries: map[string]config.ProviderEntry{
				"azure-anthropic": {
					Type:   "azureanthropic",
					APIKey: "test-key",
				},
			},
			models:      []config.ModelEntry{},
			expectError: true,
			errorMsg:    "endpoint is required",
		},
		{
			name: "Azure Anthropic with endpoint succeeds",
			entries: map[string]config.ProviderEntry{
				"azure-anthropic": {
					Type:     "azureanthropic",
					APIKey:   "test-key",
					Endpoint: "https://test.anthropic.azure.com",
				},
			},
			models: []config.ModelEntry{
				{Name: "claude-3-azure", Provider: "azure-anthropic"},
			},
			validate: func(t *testing.T, reg *Registry) {
				assert.Len(t, reg.providers, 1)
				assert.Contains(t, reg.providers, "azure-anthropic")
			},
		},
		{
			name: "Google provider",
			entries: map[string]config.ProviderEntry{
				"google": {
					Type:   "google",
					APIKey: "test-key",
				},
			},
			models: []config.ModelEntry{
				{Name: "gemini-pro", Provider: "google"},
			},
			validate: func(t *testing.T, reg *Registry) {
				assert.Len(t, reg.providers, 1)
				assert.Contains(t, reg.providers, "google")
			},
		},
		{
			name: "Vertex AI without project/location returns error",
			entries: map[string]config.ProviderEntry{
				"vertex": {
					Type: "vertexai",
				},
			},
			models:      []config.ModelEntry{},
			expectError: true,
			errorMsg:    "project and location are required",
		},
		{
			name: "Vertex AI with project and location succeeds",
			entries: map[string]config.ProviderEntry{
				"vertex": {
					Type:     "vertexai",
					Project:  "my-project",
					Location: "us-central1",
				},
			},
			models: []config.ModelEntry{
				{Name: "gemini-pro-vertex", Provider: "vertex"},
			},
			validate: func(t *testing.T, reg *Registry) {
				assert.Len(t, reg.providers, 1)
				assert.Contains(t, reg.providers, "vertex")
			},
		},
		{
			name: "unknown provider type returns error",
			entries: map[string]config.ProviderEntry{
				"unknown": {
					Type:   "unknown-type",
					APIKey: "test-key",
				},
			},
			models:      []config.ModelEntry{},
			expectError: true,
			errorMsg:    "unknown provider type",
		},
		{
			name: "provider with no API key is skipped",
			entries: map[string]config.ProviderEntry{
				"openai-no-key": {
					Type:   "openai",
					APIKey: "",
				},
				"anthropic-with-key": {
					Type:   "anthropic",
					APIKey: "sk-ant-test",
				},
			},
			models: []config.ModelEntry{
				{Name: "claude-3", Provider: "anthropic-with-key"},
			},
			validate: func(t *testing.T, reg *Registry) {
				assert.Len(t, reg.providers, 1)
				assert.Contains(t, reg.providers, "anthropic-with-key")
				assert.NotContains(t, reg.providers, "openai-no-key")
			},
		},
		{
			name: "model with provider_model_id",
			entries: map[string]config.ProviderEntry{
				"azure": {
					Type:     "azureopenai",
					APIKey:   "test-key",
					Endpoint: "https://test.openai.azure.com",
				},
			},
			models: []config.ModelEntry{
				{
					Name:            "gpt-4",
					Provider:        "azure",
					ProviderModelID: "gpt-4-deployment-name",
				},
			},
			validate: func(t *testing.T, reg *Registry) {
				assert.Equal(t, "gpt-4-deployment-name", reg.providerModelIDs["gpt-4"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, err := NewRegistry(tt.entries, tt.models)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, reg)

			if tt.validate != nil {
				tt.validate(t, reg)
			}
		})
	}
}

func TestRegistry_Get(t *testing.T) {
	reg, err := NewRegistry(
		map[string]config.ProviderEntry{
			"openai": {
				Type:   "openai",
				APIKey: "sk-test",
			},
			"anthropic": {
				Type:   "anthropic",
				APIKey: "sk-ant-test",
			},
		},
		[]config.ModelEntry{
			{Name: "gpt-4", Provider: "openai"},
		},
	)
	require.NoError(t, err)

	tests := []struct {
		name        string
		providerKey string
		expectFound bool
		validate    func(t *testing.T, p Provider)
	}{
		{
			name:        "existing provider",
			providerKey: "openai",
			expectFound: true,
			validate: func(t *testing.T, p Provider) {
				assert.Equal(t, "openai", p.Name())
			},
		},
		{
			name:        "another existing provider",
			providerKey: "anthropic",
			expectFound: true,
			validate: func(t *testing.T, p Provider) {
				assert.Equal(t, "anthropic", p.Name())
			},
		},
		{
			name:        "nonexistent provider",
			providerKey: "nonexistent",
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, found := reg.Get(tt.providerKey)

			if tt.expectFound {
				assert.True(t, found)
				require.NotNil(t, p)
				if tt.validate != nil {
					tt.validate(t, p)
				}
			} else {
				assert.False(t, found)
				assert.Nil(t, p)
			}
		})
	}
}

func TestRegistry_Models(t *testing.T) {
	tests := []struct {
		name     string
		models   []config.ModelEntry
		validate func(t *testing.T, models []struct{ Provider, Model string })
	}{
		{
			name: "single model",
			models: []config.ModelEntry{
				{Name: "gpt-4", Provider: "openai"},
			},
			validate: func(t *testing.T, models []struct{ Provider, Model string }) {
				require.Len(t, models, 1)
				assert.Equal(t, "gpt-4", models[0].Model)
				assert.Equal(t, "openai", models[0].Provider)
			},
		},
		{
			name: "multiple models",
			models: []config.ModelEntry{
				{Name: "gpt-4", Provider: "openai"},
				{Name: "claude-3", Provider: "anthropic"},
				{Name: "gemini-pro", Provider: "google"},
			},
			validate: func(t *testing.T, models []struct{ Provider, Model string }) {
				require.Len(t, models, 3)
				modelNames := make([]string, len(models))
				for i, m := range models {
					modelNames[i] = m.Model
				}
				assert.Contains(t, modelNames, "gpt-4")
				assert.Contains(t, modelNames, "claude-3")
				assert.Contains(t, modelNames, "gemini-pro")
			},
		},
		{
			name:   "no models",
			models: []config.ModelEntry{},
			validate: func(t *testing.T, models []struct{ Provider, Model string }) {
				assert.Len(t, models, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, err := NewRegistry(
				map[string]config.ProviderEntry{
					"openai": {
						Type:   "openai",
						APIKey: "sk-test",
					},
					"anthropic": {
						Type:   "anthropic",
						APIKey: "sk-ant-test",
					},
					"google": {
						Type:   "google",
						APIKey: "test-key",
					},
				},
				tt.models,
			)
			require.NoError(t, err)

			models := reg.Models()
			if tt.validate != nil {
				tt.validate(t, models)
			}
		})
	}
}

func TestRegistry_ResolveModelID(t *testing.T) {
	reg, err := NewRegistry(
		map[string]config.ProviderEntry{
			"openai": {
				Type:   "openai",
				APIKey: "sk-test",
			},
			"azure": {
				Type:     "azureopenai",
				APIKey:   "test-key",
				Endpoint: "https://test.openai.azure.com",
			},
		},
		[]config.ModelEntry{
			{Name: "gpt-4", Provider: "openai"},
			{Name: "gpt-4-azure", Provider: "azure", ProviderModelID: "gpt-4-deployment"},
		},
	)
	require.NoError(t, err)

	tests := []struct {
		name      string
		modelName string
		expected  string
	}{
		{
			name:      "model without provider_model_id returns model name",
			modelName: "gpt-4",
			expected:  "gpt-4",
		},
		{
			name:      "model with provider_model_id returns provider_model_id",
			modelName: "gpt-4-azure",
			expected:  "gpt-4-deployment",
		},
		{
			name:      "unknown model returns model name",
			modelName: "unknown-model",
			expected:  "unknown-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reg.ResolveModelID(tt.modelName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRegistry_Default(t *testing.T) {
	tests := []struct {
		name        string
		setupReg    func() *Registry
		modelName   string
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, p Provider)
	}{
		{
			name: "returns provider for known model",
			setupReg: func() *Registry {
				reg, _ := NewRegistry(
					map[string]config.ProviderEntry{
						"openai": {
							Type:   "openai",
							APIKey: "sk-test",
						},
						"anthropic": {
							Type:   "anthropic",
							APIKey: "sk-ant-test",
						},
					},
					[]config.ModelEntry{
						{Name: "gpt-4", Provider: "openai"},
						{Name: "claude-3", Provider: "anthropic"},
					},
				)
				return reg
			},
			modelName: "gpt-4",
			validate: func(t *testing.T, p Provider) {
				assert.Equal(t, "openai", p.Name())
			},
		},
		{
			name: "returns first provider for unknown model",
			setupReg: func() *Registry {
				reg, _ := NewRegistry(
					map[string]config.ProviderEntry{
						"openai": {
							Type:   "openai",
							APIKey: "sk-test",
						},
					},
					[]config.ModelEntry{
						{Name: "gpt-4", Provider: "openai"},
					},
				)
				return reg
			},
			modelName: "unknown-model",
			validate: func(t *testing.T, p Provider) {
				assert.NotNil(t, p)
				// Should return first available provider
			},
		},
		{
			name: "returns first provider for empty model name",
			setupReg: func() *Registry {
				reg, _ := NewRegistry(
					map[string]config.ProviderEntry{
						"openai": {
							Type:   "openai",
							APIKey: "sk-test",
						},
					},
					[]config.ModelEntry{
						{Name: "gpt-4", Provider: "openai"},
					},
				)
				return reg
			},
			modelName: "",
			validate: func(t *testing.T, p Provider) {
				assert.NotNil(t, p)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := tt.setupReg()
			p, err := reg.Default(tt.modelName)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, p)

			if tt.validate != nil {
				tt.validate(t, p)
			}
		})
	}
}

func TestBuildProvider(t *testing.T) {
	tests := []struct {
		name        string
		entry       config.ProviderEntry
		expectError bool
		errorMsg    string
		expectNil   bool
		validate    func(t *testing.T, p Provider)
	}{
		{
			name: "OpenAI provider",
			entry: config.ProviderEntry{
				Type:   "openai",
				APIKey: "sk-test",
			},
			validate: func(t *testing.T, p Provider) {
				assert.Equal(t, "openai", p.Name())
			},
		},
		{
			name: "OpenAI provider with custom endpoint",
			entry: config.ProviderEntry{
				Type:     "openai",
				APIKey:   "sk-test",
				Endpoint: "https://custom.openai.com",
			},
			validate: func(t *testing.T, p Provider) {
				assert.Equal(t, "openai", p.Name())
			},
		},
		{
			name: "Anthropic provider",
			entry: config.ProviderEntry{
				Type:   "anthropic",
				APIKey: "sk-ant-test",
			},
			validate: func(t *testing.T, p Provider) {
				assert.Equal(t, "anthropic", p.Name())
			},
		},
		{
			name: "Google provider",
			entry: config.ProviderEntry{
				Type:   "google",
				APIKey: "test-key",
			},
			validate: func(t *testing.T, p Provider) {
				assert.Equal(t, "google", p.Name())
			},
		},
		{
			name: "provider without API key returns nil",
			entry: config.ProviderEntry{
				Type:   "openai",
				APIKey: "",
			},
			expectNil: true,
		},
		{
			name: "unknown provider type",
			entry: config.ProviderEntry{
				Type:   "unknown",
				APIKey: "test-key",
			},
			expectError: true,
			errorMsg:    "unknown provider type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := buildProvider(tt.entry)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				return
			}

			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, p)
				return
			}

			require.NotNil(t, p)

			if tt.validate != nil {
				tt.validate(t, p)
			}
		})
	}
}
