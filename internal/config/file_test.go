package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromFile_YAML(t *testing.T) {
	content := `
providers:
  openai:
    type: openai
    api_key: sk-test
  azure:
    type: azureopenai
    api_key: az-key
    endpoint: https://my.openai.azure.com
models:
  - name: gpt-4o
    provider: openai
  - name: gpt-4o-azure
    provider: azure
    provider_model_id: gpt-4o
`
	path := writeTemp(t, "config.yaml", content)
	providers, models, err := LoadFromFile(path)
	require.NoError(t, err)

	require.Len(t, providers, 2)
	assert.Equal(t, "openai", providers["openai"].Type)
	assert.Equal(t, "sk-test", providers["openai"].APIKey)
	assert.Equal(t, "azureopenai", providers["azure"].Type)
	assert.Equal(t, "https://my.openai.azure.com", providers["azure"].Endpoint)

	require.Len(t, models, 2)
	assert.Equal(t, "gpt-4o", models[0].Name)
	assert.Equal(t, "openai", models[0].Provider)
	assert.Equal(t, "gpt-4o-azure", models[1].Name)
	assert.Equal(t, "gpt-4o", models[1].ProviderModelID)
}

func TestLoadFromFile_YML(t *testing.T) {
	content := `
providers:
  anthropic:
    type: anthropic
    api_key: ant-key
models:
  - name: claude-3-5-sonnet
    provider: anthropic
`
	path := writeTemp(t, "config.yml", content)
	providers, models, err := LoadFromFile(path)
	require.NoError(t, err)
	assert.Len(t, providers, 1)
	assert.Len(t, models, 1)
}

func TestLoadFromFile_JSON(t *testing.T) {
	content := `{
  "providers": {
    "google": {"type": "google", "api_key": "goog-key"}
  },
  "models": [
    {"name": "gemini-2.0-flash", "provider": "google"}
  ]
}`
	path := writeTemp(t, "config.json", content)
	providers, models, err := LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "google", providers["google"].Type)
	assert.Equal(t, "gemini-2.0-flash", models[0].Name)
}

func TestLoadFromFile_EmptyProviders(t *testing.T) {
	path := writeTemp(t, "config.yaml", "providers: {}\nmodels: []\n")
	providers, models, err := LoadFromFile(path)
	require.NoError(t, err)
	assert.Empty(t, providers)
	assert.Empty(t, models)
}

func TestLoadFromFile_UnsupportedExtension(t *testing.T) {
	path := writeTemp(t, "config.toml", "")
	_, _, err := LoadFromFile(path)
	assert.ErrorContains(t, err, "unsupported config file format")
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, _, err := LoadFromFile("/nonexistent/path/config.yaml")
	assert.ErrorContains(t, err, "read config file")
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	path := writeTemp(t, "config.yaml", "providers: ][invalid")
	_, _, err := LoadFromFile(path)
	assert.ErrorContains(t, err, "parse yaml config")
}

func TestLoadFromFile_InvalidJSON(t *testing.T) {
	path := writeTemp(t, "config.json", "{invalid json}")
	_, _, err := LoadFromFile(path)
	assert.ErrorContains(t, err, "parse json config")
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
