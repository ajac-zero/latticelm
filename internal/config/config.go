package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config describes the full gateway configuration file.
type Config struct {
	Server        ServerConfig             `yaml:"server"`
	Providers     map[string]ProviderEntry `yaml:"providers"`
	Models        []ModelEntry             `yaml:"models"`
	Auth          AuthConfig               `yaml:"auth"`
	Conversations ConversationConfig       `yaml:"conversations"`
	Logging       LoggingConfig            `yaml:"logging"`
	RateLimit     RateLimitConfig          `yaml:"rate_limit"`
	Observability ObservabilityConfig      `yaml:"observability"`
	Admin         AdminConfig              `yaml:"admin"`
}

// ConversationConfig controls conversation storage.
type ConversationConfig struct {
	// Enabled controls whether conversation persistence is active. Defaults to false.
	Enabled *bool `yaml:"enabled"`
	// StoreByDefault controls whether requests without an explicit store field
	// are persisted. Defaults to false (no-store unless client opts in).
	StoreByDefault bool `yaml:"store_by_default"`
	// Store is the storage backend: "memory" (default), "sql", or "redis".
	Store string `yaml:"store"`
	// TTL is the conversation expiration duration (e.g. "1h", "30m"). Defaults to "1h".
	TTL string `yaml:"ttl"`
	// MaxTTL is the maximum allowed TTL for conversations. If TTL exceeds this
	// value it is clamped. Zero means no upper limit.
	MaxTTL string `yaml:"max_ttl"`
	// DSN is the database/Redis connection string, required when store is "sql" or "redis".
	// Examples: "conversations.db" (SQLite), "postgres://user:pass@host/db", "redis://:password@localhost:6379/0".
	DSN string `yaml:"dsn"`
	// Driver is the SQL driver name, required when store is "sql".
	// Examples: "sqlite3", "postgres", "mysql".
	Driver string `yaml:"driver"`
}

// IsEnabled returns whether conversation persistence is enabled.
// Defaults to false when not explicitly set.
func (c ConversationConfig) IsEnabled() bool {
	if c.Enabled != nil {
		return *c.Enabled
	}
	return false
}

// LoggingConfig controls logging format and level.
type LoggingConfig struct {
	// Format is the log output format: "json" (default) or "text".
	Format string `yaml:"format"`
	// Level is the minimum log level: "debug", "info" (default), "warn", or "error".
	Level string `yaml:"level"`
}

// RateLimitConfig controls distributed identity-based rate limiting.
type RateLimitConfig struct {
	// Enabled controls whether rate limiting is active.
	Enabled bool `yaml:"enabled"`
	// RedisURL is the Redis connection URL for distributed state (required when enabled).
	RedisURL string `yaml:"redis_url"`
	// TrustedProxyCIDRs is a list of CIDRs whose X-Forwarded-For headers are trusted.
	TrustedProxyCIDRs []string `yaml:"trusted_proxy_cidrs"`
	// RequestsPerSecond is the number of requests allowed per second per identity.
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	// Burst is the maximum burst size allowed.
	Burst int `yaml:"burst"`
	// MaxPromptTokens is the maximum prompt tokens allowed per request.
	MaxPromptTokens int `yaml:"max_prompt_tokens"`
	// MaxOutputTokens is the maximum output tokens allowed per request.
	MaxOutputTokens int `yaml:"max_output_tokens"`
	// MaxConcurrentRequests is the maximum concurrent requests per tenant.
	MaxConcurrentRequests int `yaml:"max_concurrent_requests"`
	// DailyTokenQuota is the per-tenant daily token budget.
	DailyTokenQuota int64 `yaml:"daily_token_quota"`
}

// ObservabilityConfig controls observability features.
type ObservabilityConfig struct {
	Enabled bool          `yaml:"enabled"`
	Metrics MetricsConfig `yaml:"metrics"`
	Tracing TracingConfig `yaml:"tracing"`
}

// MetricsConfig controls Prometheus metrics.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"` // default: "/metrics"
}

// TracingConfig controls OpenTelemetry tracing.
type TracingConfig struct {
	Enabled     bool           `yaml:"enabled"`
	ServiceName string         `yaml:"service_name"` // default: "llm-gateway"
	Sampler     SamplerConfig  `yaml:"sampler"`
	Exporter    ExporterConfig `yaml:"exporter"`
}

// SamplerConfig controls trace sampling.
type SamplerConfig struct {
	Type string  `yaml:"type"` // "always", "never", "probability"
	Rate float64 `yaml:"rate"` // 0.0 to 1.0
}

// ExporterConfig controls trace exporters.
type ExporterConfig struct {
	Type     string            `yaml:"type"` // "otlp", "stdout"
	Endpoint string            `yaml:"endpoint"`
	Insecure bool              `yaml:"insecure"`
	Headers  map[string]string `yaml:"headers"`
}

// AuthConfig holds OIDC authentication settings.
type AuthConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Issuer   string `yaml:"issuer"`
	Audience string `yaml:"audience"`
}

// AdminConfig controls the admin UI.
type AdminConfig struct {
	Enabled       bool     `yaml:"enabled"`
	Claim         string   `yaml:"claim"`          // Optional admin authorization claim. Defaults to role/roles/groups lookup order.
	AllowedValues []string `yaml:"allowed_values"` // Allowed values for the admin claim. Defaults to ["admin"].
	// IPAllowlist is an optional list of CIDRs (e.g. "10.0.0.0/8") permitted to
	// reach admin routes. Empty means all source IPs are allowed.
	IPAllowlist []string `yaml:"ip_allowlist"`
}

// ServerConfig controls HTTP server values.
type ServerConfig struct {
	Address            string `yaml:"address"`
	MaxRequestBodySize int64  `yaml:"max_request_body_size"` // Maximum request body size in bytes (default: 10MB)
}

// CircuitBreakerEntry holds circuit breaker configuration overrides for a provider.
// Zero values mean "use the default".
type CircuitBreakerEntry struct {
	// MaxRequests is the max number of requests allowed through in half-open state. Default: 3.
	MaxRequests uint32 `yaml:"max_requests"`
	// Interval is the cyclic period for clearing counts in closed state (e.g. "30s"). Default: 30s.
	Interval string `yaml:"interval"`
	// Timeout is the open-state duration before transitioning to half-open (e.g. "60s"). Default: 60s.
	Timeout string `yaml:"timeout"`
	// MinRequests is the minimum number of requests before evaluating failure ratio. Default: 5.
	MinRequests uint32 `yaml:"min_requests"`
	// FailureRatio is the failure ratio threshold that trips the breaker (0.0–1.0). Default: 0.5.
	FailureRatio float64 `yaml:"failure_ratio"`
}

// ProviderEntry defines a named provider instance in the config file.
type ProviderEntry struct {
	Type           string               `yaml:"type"`
	APIKey         string               `yaml:"api_key"`
	Endpoint       string               `yaml:"endpoint"`
	APIVersion     string               `yaml:"api_version"`
	Project        string               `yaml:"project"`         // For Vertex AI
	Location       string               `yaml:"location"`        // For Vertex AI
	CircuitBreaker *CircuitBreakerEntry `yaml:"circuit_breaker"` // Optional per-provider CB overrides
}

// ModelEntry maps a model name to a provider entry.
type ModelEntry struct {
	Name            string `yaml:"name"`
	Provider        string `yaml:"provider"`
	ProviderModelID string `yaml:"provider_model_id"`
}

// ProviderConfig contains shared provider configuration fields used internally by providers.
type ProviderConfig struct {
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
	Endpoint string `yaml:"endpoint"`
}

// AzureOpenAIConfig contains Azure-specific settings used internally by the OpenAI provider.
type AzureOpenAIConfig struct {
	APIKey     string `yaml:"api_key"`
	Endpoint   string `yaml:"endpoint"`
	APIVersion string `yaml:"api_version"`
}

// AzureAnthropicConfig contains Azure-specific settings for Anthropic used internally.
type AzureAnthropicConfig struct {
	APIKey   string `yaml:"api_key"`
	Endpoint string `yaml:"endpoint"`
	Model    string `yaml:"model"`
}

// VertexAIConfig contains Vertex AI-specific settings used internally by the Google provider.
type VertexAIConfig struct {
	Project  string `yaml:"project"`
	Location string `yaml:"location"`
}

// Load reads and parses a YAML configuration file, expanding ${VAR} env references.
func Load(path string) (*Config, error) {
	// #nosec G304 -- Path is provided by application administrator, not end users
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.Expand(string(data), os.Getenv)

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg *Config) validate() error {
	for _, m := range cfg.Models {
		providerEntry, ok := cfg.Providers[m.Provider]
		if !ok {
			return fmt.Errorf("model %q references unknown provider %q", m.Name, m.Provider)
		}

		switch providerEntry.Type {
		case "openai", "anthropic", "google", "azureopenai", "azureanthropic":
			if providerEntry.APIKey == "" {
				return fmt.Errorf("model %q references provider %q (%s) without api_key", m.Name, m.Provider, providerEntry.Type)
			}
		}

		switch providerEntry.Type {
		case "azureopenai", "azureanthropic":
			if providerEntry.Endpoint == "" {
				return fmt.Errorf("model %q references provider %q (%s) without endpoint", m.Name, m.Provider, providerEntry.Type)
			}
		case "vertexai":
			if providerEntry.Project == "" || providerEntry.Location == "" {
				return fmt.Errorf("model %q references provider %q (vertexai) without project/location", m.Name, m.Provider)
			}
		case "openai", "anthropic", "google":
			// No additional required fields.
		default:
			return fmt.Errorf("model %q references provider %q with unknown type %q", m.Name, m.Provider, providerEntry.Type)
		}
	}
	return nil
}
