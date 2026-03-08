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
	// Store is the storage backend: "memory" (default), "sql", or "redis".
	Store string `yaml:"store"`
	// TTL is the conversation expiration duration (e.g. "1h", "30m"). Defaults to "1h".
	TTL string `yaml:"ttl"`
	// DSN is the database/Redis connection string, required when store is "sql" or "redis".
	// Examples: "conversations.db" (SQLite), "postgres://user:pass@host/db", "redis://:password@localhost:6379/0".
	DSN string `yaml:"dsn"`
	// Driver is the SQL driver name, required when store is "sql".
	// Examples: "sqlite3", "postgres", "mysql".
	Driver string `yaml:"driver"`
}

// LoggingConfig controls logging format and level.
type LoggingConfig struct {
	// Format is the log output format: "json" (default) or "text".
	Format string `yaml:"format"`
	// Level is the minimum log level: "debug", "info" (default), "warn", or "error".
	Level string `yaml:"level"`
}

// RateLimitConfig controls rate limiting behavior.
type RateLimitConfig struct {
	// Enabled controls whether rate limiting is active.
	Enabled bool `yaml:"enabled"`
	// RequestsPerSecond is the number of requests allowed per second per IP.
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	// Burst is the maximum burst size allowed.
	Burst int `yaml:"burst"`
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
}

// ServerConfig controls HTTP server values.
type ServerConfig struct {
	Address            string `yaml:"address"`
	MaxRequestBodySize int64  `yaml:"max_request_body_size"` // Maximum request body size in bytes (default: 10MB)
}

// ProviderEntry defines a named provider instance in the config file.
type ProviderEntry struct {
	Type       string `yaml:"type"`
	APIKey     string `yaml:"api_key"`
	Endpoint   string `yaml:"endpoint"`
	APIVersion string `yaml:"api_version"`
	Project    string `yaml:"project"`  // For Vertex AI
	Location   string `yaml:"location"` // For Vertex AI
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
