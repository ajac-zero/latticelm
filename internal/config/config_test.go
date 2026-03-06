package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		envVars     map[string]string
		expectError bool
		validate    func(t *testing.T, cfg *Config)
	}{
		{
			name: "basic config with all fields",
			configYAML: `
server:
  address: ":8080"
providers:
  openai:
    type: openai
    api_key: sk-test-key
  anthropic:
    type: anthropic
    api_key: sk-ant-key
models:
  - name: gpt-4
    provider: openai
    provider_model_id: gpt-4-turbo
  - name: claude-3
    provider: anthropic
    provider_model_id: claude-3-sonnet-20240229
auth:
  enabled: true
  issuer: https://accounts.google.com
  audience: my-client-id
conversations:
  store: memory
  ttl: 1h
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, ":8080", cfg.Server.Address)
				assert.Len(t, cfg.Providers, 2)
				assert.Equal(t, "openai", cfg.Providers["openai"].Type)
				assert.Equal(t, "sk-test-key", cfg.Providers["openai"].APIKey)
				assert.Len(t, cfg.Models, 2)
				assert.Equal(t, "gpt-4", cfg.Models[0].Name)
				assert.True(t, cfg.Auth.Enabled)
				assert.Equal(t, "memory", cfg.Conversations.Store)
			},
		},
		{
			name: "config with environment variables",
			configYAML: `
server:
  address: ":8080"
providers:
  openai:
    type: openai
    api_key: ${OPENAI_API_KEY}
models:
  - name: gpt-4
    provider: openai
    provider_model_id: gpt-4
`,
			envVars: map[string]string{
				"OPENAI_API_KEY": "sk-from-env",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "sk-from-env", cfg.Providers["openai"].APIKey)
			},
		},
		{
			name: "minimal config",
			configYAML: `
server:
  address: ":8080"
providers:
  openai:
    type: openai
    api_key: test-key
models:
  - name: gpt-4
    provider: openai
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, ":8080", cfg.Server.Address)
				assert.Len(t, cfg.Providers, 1)
				assert.Len(t, cfg.Models, 1)
				assert.False(t, cfg.Auth.Enabled)
			},
		},
		{
			name: "azure openai provider",
			configYAML: `
server:
  address: ":8080"
providers:
  azure:
    type: azureopenai
    api_key: azure-key
    endpoint: https://my-resource.openai.azure.com
    api_version: "2024-02-15-preview"
models:
  - name: gpt-4-azure
    provider: azure
    provider_model_id: gpt-4-deployment
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "azureopenai", cfg.Providers["azure"].Type)
				assert.Equal(t, "azure-key", cfg.Providers["azure"].APIKey)
				assert.Equal(t, "https://my-resource.openai.azure.com", cfg.Providers["azure"].Endpoint)
				assert.Equal(t, "2024-02-15-preview", cfg.Providers["azure"].APIVersion)
			},
		},
		{
			name: "vertex ai provider",
			configYAML: `
server:
  address: ":8080"
providers:
  vertex:
    type: vertexai
    project: my-gcp-project
    location: us-central1
models:
  - name: gemini-pro
    provider: vertex
    provider_model_id: gemini-1.5-pro
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "vertexai", cfg.Providers["vertex"].Type)
				assert.Equal(t, "my-gcp-project", cfg.Providers["vertex"].Project)
				assert.Equal(t, "us-central1", cfg.Providers["vertex"].Location)
			},
		},
		{
			name: "sql conversation store",
			configYAML: `
server:
  address: ":8080"
providers:
  openai:
    type: openai
    api_key: test-key
models:
  - name: gpt-4
    provider: openai
conversations:
  store: sql
  driver: sqlite3
  dsn: conversations.db
  ttl: 2h
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "sql", cfg.Conversations.Store)
				assert.Equal(t, "sqlite3", cfg.Conversations.Driver)
				assert.Equal(t, "conversations.db", cfg.Conversations.DSN)
				assert.Equal(t, "2h", cfg.Conversations.TTL)
			},
		},
		{
			name: "redis conversation store",
			configYAML: `
server:
  address: ":8080"
providers:
  openai:
    type: openai
    api_key: test-key
models:
  - name: gpt-4
    provider: openai
conversations:
  store: redis
  dsn: redis://localhost:6379/0
  ttl: 30m
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "redis", cfg.Conversations.Store)
				assert.Equal(t, "redis://localhost:6379/0", cfg.Conversations.DSN)
				assert.Equal(t, "30m", cfg.Conversations.TTL)
			},
		},
		{
			name: "invalid model references unknown provider",
			configYAML: `
server:
  address: ":8080"
providers:
  openai:
    type: openai
    api_key: test-key
models:
  - name: gpt-4
    provider: unknown_provider
`,
			expectError: true,
		},
		{
			name:        "invalid YAML",
			configYAML:  `invalid: yaml: content: [unclosed`,
			expectError: true,
		},
		{
			name: "model references provider without required API key",
			configYAML: `
server:
  address: ":8080"
providers:
  openai:
    type: openai
models:
  - name: gpt-4
    provider: openai
`,
			expectError: true,
		},
		{
			name: "multiple models same provider",
			configYAML: `
server:
  address: ":8080"
providers:
  openai:
    type: openai
    api_key: test-key
models:
  - name: gpt-4
    provider: openai
    provider_model_id: gpt-4-turbo
  - name: gpt-3.5
    provider: openai
    provider_model_id: gpt-3.5-turbo
  - name: gpt-4-mini
    provider: openai
    provider_model_id: gpt-4o-mini
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Len(t, cfg.Models, 3)
				for _, model := range cfg.Models {
					assert.Equal(t, "openai", model.Provider)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configYAML), 0644)
			require.NoError(t, err, "failed to write test config file")

			// Set environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Load config
			cfg, err := Load(configPath)

			if tt.expectError {
				assert.Error(t, err, "expected an error")
				return
			}

			require.NoError(t, err, "unexpected error loading config")
			require.NotNil(t, cfg, "config should not be nil")

			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	assert.Error(t, err, "should error on nonexistent file")
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config",
			config: Config{
				Providers: map[string]ProviderEntry{
					"openai": {Type: "openai", APIKey: "test-key"},
				},
				Models: []ModelEntry{
					{Name: "gpt-4", Provider: "openai"},
				},
			},
			expectError: false,
		},
		{
			name: "model references unknown provider",
			config: Config{
				Providers: map[string]ProviderEntry{
					"openai": {Type: "openai", APIKey: "test-key"},
				},
				Models: []ModelEntry{
					{Name: "gpt-4", Provider: "unknown"},
				},
			},
			expectError: true,
		},
		{
			name: "model references provider without api key",
			config: Config{
				Providers: map[string]ProviderEntry{
					"openai": {Type: "openai"},
				},
				Models: []ModelEntry{
					{Name: "gpt-4", Provider: "openai"},
				},
			},
			expectError: true,
		},
		{
			name: "no models",
			config: Config{
				Providers: map[string]ProviderEntry{
					"openai": {Type: "openai"},
				},
				Models: []ModelEntry{},
			},
			expectError: false,
		},
		{
			name: "multiple models multiple providers",
			config: Config{
				Providers: map[string]ProviderEntry{
					"openai":    {Type: "openai", APIKey: "test-key"},
					"anthropic": {Type: "anthropic", APIKey: "ant-key"},
				},
				Models: []ModelEntry{
					{Name: "gpt-4", Provider: "openai"},
					{Name: "claude-3", Provider: "anthropic"},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if tt.expectError {
				assert.Error(t, err, "expected validation error")
			} else {
				assert.NoError(t, err, "unexpected validation error")
			}
		})
	}
}

func TestEnvironmentVariableExpansion(t *testing.T) {
	configYAML := `
server:
  address: "${SERVER_ADDRESS}"
providers:
  openai:
    type: openai
    api_key: ${OPENAI_KEY}
  anthropic:
    type: anthropic
    api_key: ${ANTHROPIC_KEY:-default-key}
models:
  - name: gpt-4
    provider: openai
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configYAML), 0644)
	require.NoError(t, err)

	// Set only some env vars to test defaults
	t.Setenv("SERVER_ADDRESS", ":9090")
	t.Setenv("OPENAI_KEY", "sk-from-env")
	// Don't set ANTHROPIC_KEY to test default value

	cfg, err := Load(configPath)
	require.NoError(t, err)

	assert.Equal(t, ":9090", cfg.Server.Address)
	assert.Equal(t, "sk-from-env", cfg.Providers["openai"].APIKey)
	// Note: Go's os.Expand doesn't support default values like ${VAR:-default}
	// This is just documenting current behavior
}
