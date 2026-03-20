package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		expectError bool
		validate    func(t *testing.T, cfg *Config)
	}{
		{
			name: "empty env produces zero-value config",
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "", cfg.Server.Address)
				assert.False(t, cfg.Auth.Enabled)
				assert.False(t, cfg.UI.Enabled)
				assert.Nil(t, cfg.Providers)
				assert.Nil(t, cfg.Models)
			},
		},
		{
			name: "server and logging",
			env: map[string]string{
				"SERVER_ADDRESS":             ":9090",
				"SERVER_MAX_REQUEST_BODY_SIZE": "5242880",
				"LOG_FORMAT":                 "text",
				"LOG_LEVEL":                  "debug",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, ":9090", cfg.Server.Address)
				assert.Equal(t, int64(5242880), cfg.Server.MaxRequestBodySize)
				assert.Equal(t, "text", cfg.Logging.Format)
				assert.Equal(t, "debug", cfg.Logging.Level)
			},
		},
		{
			name: "auth fields",
			env: map[string]string{
				"AUTH_ENABLED":      "true",
				"AUTH_ISSUER":       "https://accounts.google.com",
				"AUTH_CLIENT_ID":    "my-client-id",
				"AUTH_CLIENT_SECRET": "my-secret",
				"AUTH_REDIRECT_URI": "https://example.com/callback",
				"AUTH_ADMIN_EMAIL":  "admin@example.com",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.Auth.Enabled)
				assert.Equal(t, "https://accounts.google.com", cfg.Auth.Issuer)
				assert.Equal(t, "my-client-id", cfg.Auth.ClientID)
				assert.Equal(t, "my-secret", cfg.Auth.ClientSecret)
				assert.Equal(t, "https://example.com/callback", cfg.Auth.RedirectURI)
				assert.Equal(t, "admin@example.com", cfg.Auth.AdminEmail)
			},
		},
		{
			name: "audiences defaults to client_id when not set",
			env: map[string]string{
				"AUTH_CLIENT_ID": "my-client-id",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, []string{"my-client-id"}, cfg.Auth.Audiences)
			},
		},
		{
			name: "explicit audiences take precedence over client_id",
			env: map[string]string{
				"AUTH_CLIENT_ID": "my-client-id",
				"AUTH_AUDIENCE":  "explicit-audience",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, []string{"explicit-audience"}, cfg.Auth.Audiences)
			},
		},
		{
			name: "multiple audiences are parsed correctly",
			env: map[string]string{
				"AUTH_AUDIENCE": "audience1,audience2",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, []string{"audience1", "audience2"}, cfg.Auth.Audiences)
			},
		},
		{
			name: "ui fields with comma-separated slices",
			env: map[string]string{
				"UI_ENABLED":        "true",
				"UI_CLAIM":          "groups",
				"UI_ALLOWED_VALUES": "platform-admin, ops",
				"UI_IP_ALLOWLIST":   "10.0.0.0/8, 192.168.1.0/24",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.UI.Enabled)
				assert.Equal(t, "groups", cfg.UI.Claim)
				assert.Equal(t, []string{"platform-admin", "ops"}, cfg.UI.AllowedValues)
				assert.Equal(t, []string{"10.0.0.0/8", "192.168.1.0/24"}, cfg.UI.IPAllowlist)
			},
		},
		{
			name: "rate limit fields",
			env: map[string]string{
				"RATE_LIMIT_ENABLED":               "true",
				"RATE_LIMIT_REDIS_URL":             "redis://localhost:6379/1",
				"RATE_LIMIT_TRUSTED_PROXY_CIDRS":   "10.0.0.0/8,172.16.0.0/12",
				"RATE_LIMIT_REQUESTS_PER_SECOND":   "20.5",
				"RATE_LIMIT_BURST":                 "40",
				"RATE_LIMIT_MAX_PROMPT_TOKENS":      "50000",
				"RATE_LIMIT_MAX_OUTPUT_TOKENS":      "8192",
				"RATE_LIMIT_MAX_CONCURRENT_REQUESTS": "5",
				"RATE_LIMIT_DAILY_TOKEN_QUOTA":      "500000",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.RateLimit.Enabled)
				assert.Equal(t, "redis://localhost:6379/1", cfg.RateLimit.RedisURL)
				assert.Equal(t, []string{"10.0.0.0/8", "172.16.0.0/12"}, cfg.RateLimit.TrustedProxyCIDRs)
				assert.Equal(t, 20.5, cfg.RateLimit.RequestsPerSecond)
				assert.Equal(t, 40, cfg.RateLimit.Burst)
				assert.Equal(t, 50000, cfg.RateLimit.MaxPromptTokens)
				assert.Equal(t, 8192, cfg.RateLimit.MaxOutputTokens)
				assert.Equal(t, 5, cfg.RateLimit.MaxConcurrentRequests)
				assert.Equal(t, int64(500000), cfg.RateLimit.DailyTokenQuota)
			},
		},
		{
			name: "usage fields",
			env: map[string]string{
				"USAGE_ENABLED":        "true",
				"USAGE_ANALYTICS_MODE": "timescaledb",
				"USAGE_DSN":            "postgres://user:pass@host/db",
				"USAGE_BUFFER_SIZE":    "2000",
				"USAGE_FLUSH_INTERVAL": "10s",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.Usage.Enabled)
				assert.Equal(t, "timescaledb", cfg.Usage.AnalyticsMode)
				assert.Equal(t, "postgres://user:pass@host/db", cfg.Usage.DSN)
				assert.Equal(t, 2000, cfg.Usage.BufferSize)
				assert.Equal(t, "10s", cfg.Usage.FlushInterval)
			},
		},
		{
			name: "conversations fields including nullable bool",
			env: map[string]string{
				"CONVERSATIONS_ENABLED":          "true",
				"CONVERSATIONS_STORE_BY_DEFAULT": "true",
				"CONVERSATIONS_STORE":            "sql",
				"CONVERSATIONS_TTL":              "2h",
				"CONVERSATIONS_MAX_TTL":          "24h",
				"CONVERSATIONS_DSN":              "postgres://user:pass@host/db",
				"CONVERSATIONS_DRIVER":           "pgx",
				"CONVERSATIONS_MAX_OPEN_CONNS":   "30",
				"CONVERSATIONS_MAX_IDLE_CONNS":   "10",
				"CONVERSATIONS_CONN_MAX_LIFETIME": "10m",
				"CONVERSATIONS_CONN_MAX_IDLE_TIME": "2m",
			},
			validate: func(t *testing.T, cfg *Config) {
				require.NotNil(t, cfg.Conversations.Enabled)
				assert.True(t, *cfg.Conversations.Enabled)
				assert.True(t, cfg.Conversations.StoreByDefault)
				assert.Equal(t, "sql", cfg.Conversations.Store)
				assert.Equal(t, "2h", cfg.Conversations.TTL)
				assert.Equal(t, "24h", cfg.Conversations.MaxTTL)
				assert.Equal(t, "postgres://user:pass@host/db", cfg.Conversations.DSN)
				assert.Equal(t, "pgx", cfg.Conversations.Driver)
				assert.Equal(t, 30, cfg.Conversations.MaxOpenConns)
				assert.Equal(t, 10, cfg.Conversations.MaxIdleConns)
				assert.Equal(t, "10m", cfg.Conversations.ConnMaxLifetime)
				assert.Equal(t, "2m", cfg.Conversations.ConnMaxIdleTime)
			},
		},
		{
			name: "observability and tracing fields",
			env: map[string]string{
				"OBSERVABILITY_ENABLED":    "true",
				"METRICS_ENABLED":          "true",
				"METRICS_PATH":             "/prom",
				"TRACING_ENABLED":          "true",
				"TRACING_SERVICE_NAME":     "my-gateway",
				"TRACING_SAMPLER_TYPE":     "always",
				"TRACING_SAMPLER_RATE":     "1.0",
				"TRACING_EXPORTER_TYPE":    "otlp",
				"TRACING_EXPORTER_ENDPOINT": "collector:4317",
				"TRACING_EXPORTER_INSECURE": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.Observability.Enabled)
				assert.True(t, cfg.Observability.Metrics.Enabled)
				assert.Equal(t, "/prom", cfg.Observability.Metrics.Path)
				assert.True(t, cfg.Observability.Tracing.Enabled)
				assert.Equal(t, "my-gateway", cfg.Observability.Tracing.ServiceName)
				assert.Equal(t, "always", cfg.Observability.Tracing.Sampler.Type)
				assert.Equal(t, 1.0, cfg.Observability.Tracing.Sampler.Rate)
				assert.Equal(t, "otlp", cfg.Observability.Tracing.Exporter.Type)
				assert.Equal(t, "collector:4317", cfg.Observability.Tracing.Exporter.Endpoint)
				assert.True(t, cfg.Observability.Tracing.Exporter.Insecure)
			},
		},
		{
			name:        "invalid bool returns error",
			env:         map[string]string{"AUTH_ENABLED": "notabool"},
			expectError: true,
		},
		{
			name:        "invalid int returns error",
			env:         map[string]string{"RATE_LIMIT_BURST": "notanint"},
			expectError: true,
		},
		{
			name:        "invalid float returns error",
			env:         map[string]string{"RATE_LIMIT_REQUESTS_PER_SECOND": "notafloat"},
			expectError: true,
		},
		{
			name:        "invalid int64 returns error",
			env:         map[string]string{"SERVER_MAX_REQUEST_BODY_SIZE": "notanint"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := LoadFromEnv()

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestValidate(t *testing.T) {
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
		},
		{
			name: "no models passes",
			config: Config{
				Providers: map[string]ProviderEntry{
					"openai": {Type: "openai"},
				},
			},
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
			name: "model references provider without api_key",
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
			name: "azure requires endpoint",
			config: Config{
				Providers: map[string]ProviderEntry{
					"azure": {Type: "azureopenai", APIKey: "key"},
				},
				Models: []ModelEntry{
					{Name: "gpt-4", Provider: "azure"},
				},
			},
			expectError: true,
		},
		{
			name: "vertexai requires project and location",
			config: Config{
				Providers: map[string]ProviderEntry{
					"vertex": {Type: "vertexai", Project: "my-project"},
				},
				Models: []ModelEntry{
					{Name: "gemini", Provider: "vertex"},
				},
			},
			expectError: true,
		},
		{
			name: "multiple models multiple providers",
			config: Config{
				Providers: map[string]ProviderEntry{
					"openai":    {Type: "openai", APIKey: "key"},
					"anthropic": {Type: "anthropic", APIKey: "ant-key"},
				},
				Models: []ModelEntry{
					{Name: "gpt-4", Provider: "openai"},
					{Name: "claude-3", Provider: "anthropic"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
