package admin

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

func newTestAdminServer(cfg *config.Config) *AdminServer {
	return New(
		&stubRegistry{},
		conversation.NewNopStore(),
		cfg,
		slog.Default(),
		DefaultBuildInfo(),
		nil, // auth middleware not needed for these tests
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
	s := newTestAdminServer(cfg)

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
	s := newTestAdminServer(cfg)

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
	s := newTestAdminServer(cfg)

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
	s := newTestAdminServer(cfg)

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
