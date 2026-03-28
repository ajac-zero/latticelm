package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// fileConfig is the shape of a GATEWAY_CONFIG file. Only providers and models
// are read from it; all other settings come from environment variables.
type fileConfig struct {
	Providers map[string]ProviderEntry `yaml:"providers" json:"providers"`
	Models    []ModelEntry             `yaml:"models"    json:"models"`
}

var envPlaceholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// LoadFromFile reads a YAML or JSON config file (detected by extension) and
// returns the providers and models defined in it.
func LoadFromFile(path string) (map[string]ProviderEntry, []ModelEntry, error) {
	data, err := os.ReadFile(path) // #nosec G304 - path comes from GATEWAY_CONFIG env var, operator-controlled
	if err != nil {
		return nil, nil, fmt.Errorf("read config file: %w", err)
	}

	var fc fileConfig
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml"):
		if err := yaml.Unmarshal(data, &fc); err != nil {
			return nil, nil, fmt.Errorf("parse yaml config: %w", err)
		}
	case strings.HasSuffix(lower, ".json"):
		if err := json.Unmarshal(data, &fc); err != nil {
			return nil, nil, fmt.Errorf("parse json config: %w", err)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported config file format %q (must be .yaml, .yml, or .json)", path)
	}

	if len(fc.Providers) == 0 {
		return nil, nil, fmt.Errorf("config file must define at least one provider")
	}
	if len(fc.Models) == 0 {
		return nil, nil, fmt.Errorf("config file must define at least one model")
	}

	providers := make(map[string]ProviderEntry, len(fc.Providers))
	for name, entry := range fc.Providers {
		expanded, err := expandProviderEntry(name, entry)
		if err != nil {
			return nil, nil, err
		}
		providers[name] = expanded
	}

	models := make([]ModelEntry, len(fc.Models))
	for i, model := range fc.Models {
		expanded, err := expandModelEntry(model)
		if err != nil {
			return nil, nil, fmt.Errorf("model %q: %w", model.Name, err)
		}
		models[i] = expanded
	}

	return providers, models, nil
}

func expandProviderEntry(name string, entry ProviderEntry) (ProviderEntry, error) {
	var err error

	if entry.Type, err = expandEnvString(entry.Type); err != nil {
		return entry, fmt.Errorf("provider %q type: %w", name, err)
	}
	if entry.APIKey, err = expandEnvString(entry.APIKey); err != nil {
		return entry, fmt.Errorf("provider %q api_key: %w", name, err)
	}
	if entry.Endpoint, err = expandEnvString(entry.Endpoint); err != nil {
		return entry, fmt.Errorf("provider %q endpoint: %w", name, err)
	}
	if entry.APIVersion, err = expandEnvString(entry.APIVersion); err != nil {
		return entry, fmt.Errorf("provider %q api_version: %w", name, err)
	}
	if entry.Project, err = expandEnvString(entry.Project); err != nil {
		return entry, fmt.Errorf("provider %q project: %w", name, err)
	}
	if entry.Location, err = expandEnvString(entry.Location); err != nil {
		return entry, fmt.Errorf("provider %q location: %w", name, err)
	}
	if entry.CircuitBreaker != nil {
		if entry.CircuitBreaker.Interval, err = expandEnvString(entry.CircuitBreaker.Interval); err != nil {
			return entry, fmt.Errorf("provider %q circuit_breaker.interval: %w", name, err)
		}
		if entry.CircuitBreaker.Timeout, err = expandEnvString(entry.CircuitBreaker.Timeout); err != nil {
			return entry, fmt.Errorf("provider %q circuit_breaker.timeout: %w", name, err)
		}
	}

	return entry, nil
}

func expandModelEntry(model ModelEntry) (ModelEntry, error) {
	var err error

	if model.Name, err = expandEnvString(model.Name); err != nil {
		return model, fmt.Errorf("name: %w", err)
	}
	if model.Provider, err = expandEnvString(model.Provider); err != nil {
		return model, fmt.Errorf("provider: %w", err)
	}
	if model.ProviderModelID, err = expandEnvString(model.ProviderModelID); err != nil {
		return model, fmt.Errorf("provider_model_id: %w", err)
	}

	return model, nil
}

func expandEnvString(value string) (string, error) {
	matches := envPlaceholderPattern.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return value, nil
	}

	var builder strings.Builder
	last := 0
	for _, match := range matches {
		builder.WriteString(value[last:match[0]])
		key := value[match[2]:match[3]]
		envValue, ok := os.LookupEnv(key)
		if !ok {
			return "", fmt.Errorf("environment variable %q is not set", key)
		}
		builder.WriteString(envValue)
		last = match[1]
	}
	builder.WriteString(value[last:])

	return builder.String(), nil
}
