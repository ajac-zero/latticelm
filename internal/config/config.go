package config

import (
	"fmt"
)

// Config describes the full gateway configuration.
type Config struct {
	Server        ServerConfig             `json:"server"`
	Providers     map[string]ProviderEntry `json:"providers"`
	Models        []ModelEntry             `json:"models"`
	Auth          AuthConfig               `json:"auth"`
	APIKeys       APIKeysConfig            `json:"api_keys"`
	Conversations ConversationConfig       `json:"conversations"`
	Logging       LoggingConfig            `json:"logging"`
	RateLimit     RateLimitConfig          `json:"rate_limit"`
	Session       SessionConfig            `json:"session"`
	Observability ObservabilityConfig      `json:"observability"`
	Usage         UsageConfig              `json:"usage"`
	UI            UIConfig                 `json:"ui"`
}

// APIKeysConfig controls API key authentication.
type APIKeysConfig struct {
	Enabled        bool `json:"enabled"`
	MaxKeysPerUser int  `json:"max_keys_per_user"` // 0 = unlimited
}

// ConversationConfig controls conversation storage.
type ConversationConfig struct {
	Enabled         *bool  `json:"enabled"`
	StoreByDefault  bool   `json:"store_by_default"`
	TTL             string `json:"ttl"`
	MaxTTL          string `json:"max_ttl"`
	MaxOpenConns    int    `json:"max_open_conns"`
	MaxIdleConns    int    `json:"max_idle_conns"`
	ConnMaxLifetime string `json:"conn_max_lifetime"`
	ConnMaxIdleTime string `json:"conn_max_idle_time"`
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
	Format string `json:"format"`
	Level  string `json:"level"`
}

// RateLimitConfig controls distributed identity-based rate limiting.
type RateLimitConfig struct {
	Enabled               bool     `json:"enabled"`
	RedisURL              string   `json:"redis_url"`
	TrustedProxyCIDRs     []string `json:"trusted_proxy_cidrs"`
	RequestsPerSecond     float64  `json:"requests_per_second"`
	Burst                 int      `json:"burst"`
	MaxPromptTokens       int      `json:"max_prompt_tokens"`
	MaxOutputTokens       int      `json:"max_output_tokens"`
	MaxConcurrentRequests int      `json:"max_concurrent_requests"`
	DailyTokenQuota       int64    `json:"daily_token_quota"`
}

// SessionConfig controls session storage for OIDC authentication.
type SessionConfig struct {
	// RedisURL enables Redis-backed session storage for distributed deployments.
	// If empty, in-memory storage is used (single instance only).
	RedisURL string `json:"redis_url"`
	// TTL is the session lifetime. Defaults to 24 hours.
	TTL string `json:"ttl"`
}

// ObservabilityConfig controls observability features.
type ObservabilityConfig struct {
	Enabled bool          `json:"enabled"`
	Metrics MetricsConfig `json:"metrics"`
	Tracing TracingConfig `json:"tracing"`
}

// MetricsConfig controls Prometheus metrics.
type MetricsConfig struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

// TracingConfig controls OpenTelemetry tracing.
type TracingConfig struct {
	Enabled     bool           `json:"enabled"`
	ServiceName string         `json:"service_name"`
	Sampler     SamplerConfig  `json:"sampler"`
	Exporter    ExporterConfig `json:"exporter"`
}

// SamplerConfig controls trace sampling.
type SamplerConfig struct {
	Type string  `json:"type"`
	Rate float64 `json:"rate"`
}

// ExporterConfig controls trace exporters.
type ExporterConfig struct {
	Type     string            `json:"type"`
	Endpoint string            `json:"endpoint"`
	Insecure bool              `json:"insecure"`
	Headers  map[string]string `json:"headers"`
}

// UsageConfig controls persistent token usage tracking.
type UsageConfig struct {
	Enabled       bool   `json:"enabled"`
	BufferSize    int    `json:"buffer_size"`
	FlushInterval string `json:"flush_interval"`
}

// AuthConfig holds OIDC authentication settings.
type AuthConfig struct {
	Enabled      bool     `json:"enabled"`
	Issuer       string   `json:"issuer"`
	DiscoveryURL string   `json:"discovery_url"` // optional; overrides the derived {issuer}/.well-known/openid-configuration URL
	Audiences    []string `json:"audiences"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURI  string   `json:"redirect_uri"`
	AdminEmail   string   `json:"admin_email"`
}

// UIConfig controls the web UI.
type UIConfig struct {
	Enabled       bool     `json:"enabled"`
	Claim         string   `json:"claim"`
	AllowedValues []string `json:"allowed_values"`
	IPAllowlist   []string `json:"ip_allowlist"`
}

// ServerConfig controls HTTP server values.
type ServerConfig struct {
	Address            string `json:"address"`
	MaxRequestBodySize int64  `json:"max_request_body_size"`
}

// CircuitBreakerEntry holds circuit breaker configuration overrides for a provider.
type CircuitBreakerEntry struct {
	MaxRequests  uint32  `json:"max_requests,omitempty"  yaml:"max_requests,omitempty"`
	Interval     string  `json:"interval,omitempty"      yaml:"interval,omitempty"`
	Timeout      string  `json:"timeout,omitempty"       yaml:"timeout,omitempty"`
	MinRequests  uint32  `json:"min_requests,omitempty"  yaml:"min_requests,omitempty"`
	FailureRatio float64 `json:"failure_ratio,omitempty" yaml:"failure_ratio,omitempty"`
}

// ProviderEntry defines a named provider instance.
type ProviderEntry struct {
	Type           string               `json:"type"                      yaml:"type"`
	APIKey         string               `json:"api_key,omitempty"         yaml:"api_key,omitempty"`
	Endpoint       string               `json:"endpoint,omitempty"        yaml:"endpoint,omitempty"`
	APIVersion     string               `json:"api_version,omitempty"     yaml:"api_version,omitempty"`
	Project        string               `json:"project,omitempty"         yaml:"project,omitempty"`
	Location       string               `json:"location,omitempty"        yaml:"location,omitempty"`
	CircuitBreaker *CircuitBreakerEntry `json:"circuit_breaker,omitempty" yaml:"circuit_breaker,omitempty"`
}

// ModelEntry maps a model name to a provider.
type ModelEntry struct {
	Name            string `json:"name"                       yaml:"name"`
	Provider        string `json:"provider"                   yaml:"provider"`
	ProviderModelID string `json:"provider_model_id,omitempty" yaml:"provider_model_id,omitempty"`
}

// ProviderConfig contains shared provider configuration fields used internally by providers.
type ProviderConfig struct {
	APIKey   string
	Model    string
	Endpoint string
}

// AzureOpenAIConfig contains Azure-specific settings used internally by the OpenAI provider.
type AzureOpenAIConfig struct {
	APIKey     string
	Endpoint   string
	APIVersion string
}

// AzureAnthropicConfig contains Azure-specific settings for Anthropic used internally.
type AzureAnthropicConfig struct {
	APIKey   string
	Endpoint string
	Model    string
}

// VertexAIConfig contains Vertex AI-specific settings used internally by the Google provider.
type VertexAIConfig struct {
	Project  string
	Location string
}

// LoadFromEnv builds a Config from environment variables.
// Providers and Models are not populated here; they require a database-backed
// config store (DATABASE_URL + ENCRYPTION_KEY) and are loaded by loadConfig in main.
func LoadFromEnv() (*Config, error) {
	var cfg Config
	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, err
	}
	if len(cfg.Auth.Audiences) == 0 && cfg.Auth.ClientID != "" {
		cfg.Auth.Audiences = []string{cfg.Auth.ClientID}
	}
	return &cfg, nil
}

// Validate checks that all model entries reference a known, correctly
// configured provider.
func (cfg *Config) Validate() error {
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


