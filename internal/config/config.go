package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config describes the full gateway configuration file.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Providers ProvidersConfig `yaml:"providers"`
	Auth      AuthConfig      `yaml:"auth"`
}

// AuthConfig holds OIDC authentication settings.
type AuthConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Issuer   string `yaml:"issuer"`
	Audience string `yaml:"audience"`
}

// ServerConfig controls HTTP server values.
type ServerConfig struct {
	Address string `yaml:"address"`
}

// ProvidersConfig wraps supported provider settings.
type ProvidersConfig struct {
	Google         ProviderConfig       `yaml:"google"`
	Anthropic      ProviderConfig       `yaml:"anthropic"`
	OpenAI         ProviderConfig       `yaml:"openai"`
	AzureOpenAI    AzureOpenAIConfig    `yaml:"azureopenai"`
	AzureAnthropic AzureAnthropicConfig `yaml:"azureanthropic"`
}

// AzureAnthropicConfig contains Azure-specific settings for Anthropic (Microsoft Foundry).
type AzureAnthropicConfig struct {
	APIKey   string `yaml:"api_key"`
	Endpoint string `yaml:"endpoint"`
	Model    string `yaml:"model"`
}

// ProviderConfig contains shared provider configuration fields.
type ProviderConfig struct {
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
	Endpoint string `yaml:"endpoint"`
}

// AzureOpenAIConfig contains Azure-specific settings.
type AzureOpenAIConfig struct {
	APIKey       string `yaml:"api_key"`
	Endpoint     string `yaml:"endpoint"`
	DeploymentID string `yaml:"deployment_id"`
	APIVersion   string `yaml:"api_version"`
}

// Load reads and parses a YAML configuration file and applies env overrides.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyEnvOverrides()
	return &cfg, nil
}

func (cfg *Config) applyEnvOverrides() {
	overrideAPIKey(&cfg.Providers.Google, "GOOGLE_API_KEY")
	overrideAPIKey(&cfg.Providers.Anthropic, "ANTHROPIC_API_KEY")
	overrideAPIKey(&cfg.Providers.OpenAI, "OPENAI_API_KEY")
	
	// Azure OpenAI overrides
	if v := os.Getenv("AZURE_OPENAI_API_KEY"); v != "" {
		cfg.Providers.AzureOpenAI.APIKey = v
	}
	if v := os.Getenv("AZURE_OPENAI_ENDPOINT"); v != "" {
		cfg.Providers.AzureOpenAI.Endpoint = v
	}
	if v := os.Getenv("AZURE_OPENAI_DEPLOYMENT_ID"); v != "" {
		cfg.Providers.AzureOpenAI.DeploymentID = v
	}
	if v := os.Getenv("AZURE_OPENAI_API_VERSION"); v != "" {
		cfg.Providers.AzureOpenAI.APIVersion = v
	}

	// Azure Anthropic (Microsoft Foundry) overrides
	if v := os.Getenv("AZURE_ANTHROPIC_API_KEY"); v != "" {
		cfg.Providers.AzureAnthropic.APIKey = v
	}
	if v := os.Getenv("AZURE_ANTHROPIC_ENDPOINT"); v != "" {
		cfg.Providers.AzureAnthropic.Endpoint = v
	}
	if v := os.Getenv("AZURE_ANTHROPIC_MODEL"); v != "" {
		cfg.Providers.AzureAnthropic.Model = v
	}
}

func overrideAPIKey(cfg *ProviderConfig, envKey string) {
	if cfg == nil {
		return
	}
	if v := os.Getenv(envKey); v != "" {
		cfg.APIKey = v
	}
}
