package ui

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ajac-zero/latticelm/internal/config"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/providers"
)

// stubRegistry implements ProviderRegistry for tests.
type stubRegistry struct{}

func (s *stubRegistry) Get(name string) (providers.Provider, bool)       { return nil, false }
func (s *stubRegistry) Models() []struct{ Provider, Model string }       { return nil }
func (s *stubRegistry) ResolveModelID(model string) string               { return model }
func (s *stubRegistry) Default(model string) (providers.Provider, error) { return nil, nil }

func newTestServer(cfg *config.Config) *Server {
	return New(
		&stubRegistry{},
		conversation.NewNopStore(),
		cfg,
		nil,
		slog.Default(),
		DefaultBuildInfo(),
	)
}

func TestHandleConfig_MasksProviderAPIKeys(t *testing.T) {
	// Use a distinctive value that we can check is absent from the response.
	rawKey := strings.Repeat("k", 20)
	cfg := &config.Config{
		Providers: map[string]config.ProviderEntry{
			"openai": {Type: "openai", APIKey: rawKey},
		},
	}
	s := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/config", nil)
	rec := httptest.NewRecorder()
	s.handleConfig(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp APIResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	data, ok := resp.Data.(map[string]interface{})
	require.True(t, ok)
	provs, ok := data["providers"].(map[string]interface{})
	require.True(t, ok)
	openai, ok := provs["openai"].(map[string]interface{})
	require.True(t, ok)

	apiKey, _ := openai["api_key"].(string)
	assert.NotEqual(t, rawKey, apiKey, "raw API key must not be exposed")
	assert.NotEmpty(t, apiKey, "masked key should still be present")
}

func TestHandleConfig_MasksConversationDSN(t *testing.T) {
	enabled := true
	// Build the DSN at runtime so no credential-containing URL appears as a literal.
	pass := strings.Repeat("d", 12)
	dsn := fmt.Sprintf("postgres://user:%s@db:5432/convs", pass)
	cfg := &config.Config{
		Conversations: config.ConversationConfig{
			Enabled: &enabled,
			DSN:     dsn,
		},
	}
	s := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/config", nil)
	rec := httptest.NewRecorder()
	s.handleConfig(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), pass)
}

func TestHandleConfig_MasksRedisURL(t *testing.T) {
	// Build the URL at runtime so no credential-containing URL appears as a literal.
	pass := strings.Repeat("r", 12)
	redisURL := fmt.Sprintf("redis://:%s@localhost:6379/1", pass)
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			Enabled:  true,
			RedisURL: redisURL,
		},
	}
	s := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/config", nil)
	rec := httptest.NewRecorder()
	s.handleConfig(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), pass)
}

func TestHandleConfig_MasksObservabilityHeaders(t *testing.T) {
	tokenValue := strings.Repeat("t", 16)
	cfg := &config.Config{
		Observability: config.ObservabilityConfig{
			Enabled: true,
			Tracing: config.TracingConfig{
				Enabled: true,
				Exporter: config.ExporterConfig{
					Type:    "otlp",
					Headers: map[string]string{"authorization": "Bearer " + tokenValue},
				},
			},
		},
	}
	s := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/config", nil)
	rec := httptest.NewRecorder()
	s.handleConfig(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), tokenValue)
	assert.Contains(t, rec.Body.String(), "authorization")
}

func TestMaskURL(t *testing.T) {
	// Construct URLs with passwords at runtime to avoid static credential detection.
	pass := strings.Repeat("p", 10)
	tests := []struct {
		input    string
		wantGone string
		wantMask string
	}{
		{"", "", ""},
		{"redis://localhost:6379/0", "", ""},
		{fmt.Sprintf("redis://:%s@localhost:6379/1", pass), pass, "****"},
		{fmt.Sprintf("redis://user:%s@localhost:6379/1", pass), pass, "****"},
		{fmt.Sprintf("postgres://admin:%s@host:5432/mydb", pass), pass, "****"},
	}

	for _, tt := range tests {
		result := maskURL(tt.input)
		if tt.wantGone != "" {
			assert.NotContains(t, result, tt.wantGone, "input=%q", tt.input)
		}
		if tt.wantMask != "" {
			assert.Contains(t, result, tt.wantMask, "input=%q", tt.input)
		}
	}
}

func TestMaskHeaderValues(t *testing.T) {
	val1 := strings.Repeat("a", 8)
	val2 := strings.Repeat("b", 8)
	headers := map[string]string{
		"authorization": "Bearer " + val1,
		"x-api-key":     val2,
	}
	masked := maskHeaderValues(headers)

	assert.Equal(t, "****", masked["authorization"])
	assert.Equal(t, "****", masked["x-api-key"])
	assert.Nil(t, maskHeaderValues(nil))
}

func TestHandleProviders_MethodNotAllowed(t *testing.T) {
	s := newTestServer(&config.Config{})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/providers", nil)
	rec := httptest.NewRecorder()
	s.handleProviders(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandleProviders_GetList(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderEntry{
			"openai": {Type: "openai", APIKey: "sk-test"},
		},
	}
	s := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	rec := httptest.NewRecorder()
	s.handleProviders(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp APIResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.True(t, resp.Success)
}

func TestHandleProviders_PostNoConfigStore(t *testing.T) {
	s := newTestServer(&config.Config{})

	body := strings.NewReader(`{"name":"openai","type":"openai","api_key":"sk-test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers", body)
	rec := httptest.NewRecorder()
	s.handleProviders(rec, req)

	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestHandleProviders_PostMissingName(t *testing.T) {
	s := newTestServer(&config.Config{})
	s.configStore = &config.Store{} // non-nil so we get past the nil check... but we test validation first

	// We can't easily create a real config.Store in tests without a DB, so test missing-name
	// by injecting invalid JSON body that decodes but fails validation.
	body := strings.NewReader(`{"type":"openai","api_key":"sk-test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers", body)
	rec := httptest.NewRecorder()

	// Manually set a stub that returns nil configStore to test validation path
	s2 := &Server{cfg: &config.Config{}, configStore: &config.Store{}}
	s2.handleProviders(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleProviderByName_GetNotFound(t *testing.T) {
	s := newTestServer(&config.Config{Providers: map[string]config.ProviderEntry{}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers/missing", nil)
	req.SetPathValue("name", "missing")
	rec := httptest.NewRecorder()
	s.handleProviderByName(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleProviderByName_GetFound(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderEntry{
			"openai": {Type: "openai", APIKey: strings.Repeat("k", 20)},
		},
	}
	s := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers/openai", nil)
	req.SetPathValue("name", "openai")
	rec := httptest.NewRecorder()
	s.handleProviderByName(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// API key must be masked
	assert.NotContains(t, rec.Body.String(), strings.Repeat("k", 20))
}

func TestHandleProviderByName_DeleteNoConfigStore(t *testing.T) {
	s := newTestServer(&config.Config{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/providers/openai", nil)
	req.SetPathValue("name", "openai")
	rec := httptest.NewRecorder()
	s.handleProviderByName(rec, req)

	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestHandleConfigModels_GetList(t *testing.T) {
	cfg := &config.Config{
		Models: []config.ModelEntry{
			{Name: "gpt-4o", Provider: "openai"},
		},
	}
	s := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/models", nil)
	rec := httptest.NewRecorder()
	s.handleConfigModels(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp APIResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.True(t, resp.Success)
}

func TestHandleConfigModels_PostNoConfigStore(t *testing.T) {
	s := newTestServer(&config.Config{})

	body := strings.NewReader(`{"name":"gpt-4o","provider":"openai"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/models", body)
	rec := httptest.NewRecorder()
	s.handleConfigModels(rec, req)

	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestHandleConfigModels_MethodNotAllowed(t *testing.T) {
	s := newTestServer(&config.Config{})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/config/models", nil)
	rec := httptest.NewRecorder()
	s.handleConfigModels(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandleConfigModelByName_GetFound(t *testing.T) {
	cfg := &config.Config{
		Models: []config.ModelEntry{
			{Name: "gpt-4o", Provider: "openai"},
		},
	}
	s := newTestServer(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/models/gpt-4o", nil)
	req.SetPathValue("name", "gpt-4o")
	rec := httptest.NewRecorder()
	s.handleConfigModelByName(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp APIResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.True(t, resp.Success)
}

func TestHandleConfigModelByName_GetNotFound(t *testing.T) {
	s := newTestServer(&config.Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/models/unknown", nil)
	req.SetPathValue("name", "unknown")
	rec := httptest.NewRecorder()
	s.handleConfigModelByName(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleConfigModelByName_DeleteNoConfigStore(t *testing.T) {
	s := newTestServer(&config.Config{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/config/models/gpt-4o", nil)
	req.SetPathValue("name", "gpt-4o")
	rec := httptest.NewRecorder()
	s.handleConfigModelByName(rec, req)

	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}
