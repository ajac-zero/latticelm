package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// fileConfig is the shape of a GATEWAY_CONFIG file. Only providers and models
// are read from it; all other settings come from environment variables.
type fileConfig struct {
	Providers map[string]ProviderEntry `yaml:"providers" json:"providers"`
	Models    []ModelEntry             `yaml:"models"    json:"models"`
}

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

	return fc.Providers, fc.Models, nil
}
