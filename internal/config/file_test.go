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

func TestLoadFromFile_ExpandsEnvPlaceholders(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	t.Setenv("OPENAI_MODEL_ID", "gpt-4o")
	content := `
providers:
  openai:
    type: openai
    api_key: ${OPENAI_API_KEY}
models:
  - name: ${OPENAI_MODEL_ID}
    provider: openai
`
	path := writeTemp(t, "config.yaml", content)
	providers, models, err := LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "sk-from-env", providers["openai"].APIKey)
	assert.Equal(t, "gpt-4o", models[0].Name)
}

func TestLoadFromFile_MissingEnvPlaceholder(t *testing.T) {
	content := `
providers:
  openai:
    type: openai
    api_key: ${OPENAI_API_KEY}
models:
  - name: gpt-4o
    provider: openai
`
	path := writeTemp(t, "config.yaml", content)
	_, _, err := LoadFromFile(path)
	assert.ErrorContains(t, err, `environment variable "OPENAI_API_KEY" is not set`)
}

func TestLoadFromFile_RequiresProvidersAndModels(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "missing providers",
			content: `
providers: {}
models:
  - name: gpt-4o
    provider: openai
`,
			want: "at least one provider",
		},
		{
			name: "missing models",
			content: `
providers:
  openai:
    type: openai
    api_key: sk-test
models: []
`,
			want: "at least one model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, "config.yaml", tt.content)
			_, _, err := LoadFromFile(path)
			assert.ErrorContains(t, err, tt.want)
		})
	}
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
