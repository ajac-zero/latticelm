//go:build integration

package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/config"
)

const (
	envAzureAnthropicAPIKey  = "AZURE_ANTHROPIC_API_KEY"
	envAzureAnthropicEndpoint = "AZURE_ANTHROPIC_ENDPOINT"
	envAzureAnthropicModel    = "AZURE_ANTHROPIC_MODEL"
)

type azureAnthropicTestEnv struct {
	apiKey   string
	endpoint string
	model    string
}

func loadAzureAnthropicEnv(t *testing.T) azureAnthropicTestEnv {
	t.Helper()
	return azureAnthropicTestEnv{
		apiKey:   os.Getenv(envAzureAnthropicAPIKey),
		endpoint: os.Getenv(envAzureAnthropicEndpoint),
		model:    os.Getenv(envAzureAnthropicModel),
	}
}

type azureAnthropicVCRResult struct {
	provider *Provider
	recorder *recorder.Recorder
	model    string
}

func newAzureAnthropicVCRProvider(t *testing.T, cassetteName string) azureAnthropicVCRResult {
	t.Helper()
	env := loadAzureAnthropicEnv(t)

	cassettePath := "testdata/cassettes/" + cassetteName
	hasCredentials := env.apiKey != "" && env.endpoint != ""
	hasCassette := cassetteFileExists(cassettePath)

	if !hasCredentials && !hasCassette {
		t.Skipf("no cassette at %s.yaml and no Azure Anthropic credentials set; skipping", cassettePath)
	}

	mode := recorder.ModeReplayOnly
	if hasCredentials {
		mode = recorder.ModeRecordOnly
	}

	rec, err := recorder.New(cassettePath,
		recorder.WithMode(mode),
		recorder.WithSkipRequestLatency(true),
		recorder.WithHook(scrubAzureAnthropicCassette, recorder.AfterCaptureHook),
		recorder.WithMatcher(matchByMethodAndBodyAnthropic),
	)
	require.NoError(t, err)

	httpClient := rec.GetDefaultClient()

	endpoint := env.endpoint
	apiKey := env.apiKey
	model := env.model

	if mode == recorder.ModeReplayOnly {
		apiKey = "replayed"
		if endpoint == "" {
			endpoint = "https://replayed.services.ai.azure.com/anthropic"
		}
		if model == "" {
			model = modelFromAnthropicCassette(t, cassettePath)
		}
	}

	if model == "" {
		model = "claude-3-5-sonnet"
	}

	client := anthropic.NewClient(
		option.WithBaseURL(endpoint),
		option.WithAPIKey("unused"),
		option.WithAuthToken(apiKey),
		option.WithHTTPClient(httpClient),
	)

	provider := &Provider{
		cfg: config.ProviderConfig{
			APIKey: apiKey,
			Model:  model,
		},
		client: &client,
		azure:  true,
	}

	return azureAnthropicVCRResult{
		provider: provider,
		recorder: rec,
		model:    model,
	}
}

func cassetteFileExists(path string) bool {
	_, err := os.Stat(path + ".yaml")
	return err == nil
}

func modelFromAnthropicCassette(t *testing.T, cassettePath string) string {
	t.Helper()
	c, err := cassette.Load(cassettePath)
	if err != nil || len(c.Interactions) == 0 {
		return ""
	}
	var body struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal([]byte(c.Interactions[0].Request.Body), &body); err != nil {
		return ""
	}
	return body.Model
}

func scrubAzureAnthropicCassette(i *cassette.Interaction) error {
	// Remove auth headers
	for key := range i.Request.Headers {
		if key == "Authorization" || key == "X-Api-Key" {
			delete(i.Request.Headers, key)
		}
	}

	// Replace endpoint hostname in URL
	if u, err := url.Parse(i.Request.URL); err == nil {
		u.Host = "scrubbed.services.ai.azure.com"
		i.Request.URL = u.String()
	}
	i.Request.Host = "scrubbed.services.ai.azure.com"

	return nil
}

// matchByMethodAndBodyAnthropic matches requests by HTTP method and request body,
// ignoring the URL. This is necessary because Azure Anthropic may use different
// endpoints during replay, but the body content is stable.
func matchByMethodAndBodyAnthropic(req *http.Request, rec cassette.Request) bool {
	if req.Method != rec.Method {
		return false
	}

	// Compare request bodies (ignoring the model field which may differ in replay)
	reqBody := normalizeBodyString(readRequestBodyString(req))
	recBody := normalizeBodyString(rec.Body)

	return reqBody == recBody
}

// readRequestBodyString reads the body from an http.Request and resets it.
func readRequestBodyString(body interface{}) string {
	switch v := body.(type) {
	case string:
		return v
	case *http.Request:
		if v.Body == nil {
			return ""
		}
		defer v.Body.Close()
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, v.Body)
		v.Body = io.NopCloser(&buf)
		return buf.String()
	default:
		return ""
	}
}

// normalizeBodyString strips the "model" field for comparison since Azure
// deployment names may not match the model field in the request.
func normalizeBodyString(body string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return body
	}
	delete(m, "model")
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return string(out)
}

// Test: Basic text generation (sync)
func TestAzureAnthropicIntegration_Generate_BasicText(t *testing.T) {
	vcr := newAzureAnthropicVCRProvider(t, "azure_anthropic_generate_basic_text")
	defer vcr.recorder.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := vcr.provider.Generate(ctx,
		[]api.Message{
			{
				Role:    "user",
				Content: []api.ContentBlock{{Type: "input_text", Text: "Say hello"}},
			},
		},
		&api.ResponseRequest{
			Model: vcr.model,
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Text)
	assert.Greater(t, result.Usage.OutputTokens, 0)
}

// Test: System prompt (sync)
func TestAzureAnthropicIntegration_Generate_WithSystemPrompt(t *testing.T) {
	vcr := newAzureAnthropicVCRProvider(t, "azure_anthropic_generate_system_prompt")
	defer vcr.recorder.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := vcr.provider.Generate(ctx,
		[]api.Message{
			{
				Role:    "developer",
				Content: []api.ContentBlock{{Type: "input_text", Text: "You are a helpful assistant"}},
			},
			{
				Role:    "user",
				Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}},
			},
		},
		&api.ResponseRequest{
			Model: vcr.model,
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Text)
}

// Test: Tool calling (sync)
func TestAzureAnthropicIntegration_Generate_ToolCalling(t *testing.T) {
	vcr := newAzureAnthropicVCRProvider(t, "azure_anthropic_generate_tool_calling")
	defer vcr.recorder.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	toolsJSON := `[{
		"type": "function",
		"name": "calculator",
		"description": "A simple calculator",
		"input_schema": {
			"type": "object",
			"properties": {
				"operation": {"type": "string", "description": "add, subtract, multiply, divide"},
				"a": {"type": "number", "description": "first number"},
				"b": {"type": "number", "description": "second number"}
			},
			"required": ["operation", "a", "b"]
		}
	}]`

	result, err := vcr.provider.Generate(ctx,
		[]api.Message{
			{
				Role:    "user",
				Content: []api.ContentBlock{{Type: "input_text", Text: "What is 2 + 2? Use the calculator tool."}},
			},
		},
		&api.ResponseRequest{
			Model: vcr.model,
			Tools: json.RawMessage(toolsJSON),
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Greater(t, len(result.ToolCalls), 0)
}

// Test: Multi-turn conversation (sync)
func TestAzureAnthropicIntegration_Generate_MultiTurn(t *testing.T) {
	vcr := newAzureAnthropicVCRProvider(t, "azure_anthropic_generate_multi_turn")
	defer vcr.recorder.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := vcr.provider.Generate(ctx,
		[]api.Message{
			{
				Role:    "user",
				Content: []api.ContentBlock{{Type: "input_text", Text: "My name is Alice."}},
			},
			{
				Role:    "assistant",
				Content: []api.ContentBlock{{Type: "output_text", Text: "Hello Alice! How can I help you?"}},
			},
			{
				Role:    "user",
				Content: []api.ContentBlock{{Type: "input_text", Text: "What is my name?"}},
			},
		},
		&api.ResponseRequest{
			Model: vcr.model,
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Text)
	assert.Contains(t, result.Text, "Alice")
}

// Test: Streaming text
func TestAzureAnthropicIntegration_GenerateStream_BasicText(t *testing.T) {
	vcr := newAzureAnthropicVCRProvider(t, "azure_anthropic_stream_basic_text")
	defer vcr.recorder.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deltaChan, errChan := vcr.provider.GenerateStream(ctx,
		[]api.Message{
			{
				Role:    "user",
				Content: []api.ContentBlock{{Type: "input_text", Text: "Count from 1 to 5."}},
			},
		},
		&api.ResponseRequest{
			Model: vcr.model,
		},
	)

	var fullText string
	var gotDone bool
	var streamUsage *api.Usage
	var lastErr error

	for deltaChan != nil || errChan != nil {
		select {
		case delta, ok := <-deltaChan:
			if !ok {
				deltaChan = nil
				continue
			}
			if delta.Done {
				gotDone = true
				streamUsage = delta.Usage
			} else {
				fullText += delta.Text
			}
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				continue
			}
			if err != nil {
				lastErr = err
			}
		}
	}

	require.NoError(t, lastErr)
	assert.True(t, gotDone, "expected a done delta")
	assert.NotEmpty(t, fullText)
	require.NotNil(t, streamUsage, "expected usage in final delta")
	assert.Greater(t, streamUsage.InputTokens, 0)
	assert.Greater(t, streamUsage.OutputTokens, 0)
}

// Test: Streaming tool calling
func TestAzureAnthropicIntegration_GenerateStream_ToolCalling(t *testing.T) {
	vcr := newAzureAnthropicVCRProvider(t, "azure_anthropic_stream_tool_calling")
	defer vcr.recorder.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	toolsJSON := `[{
		"type": "function",
		"name": "calculator",
		"description": "A simple calculator",
		"input_schema": {
			"type": "object",
			"properties": {
				"operation": {"type": "string", "description": "add, subtract, multiply, divide"},
				"a": {"type": "number", "description": "first number"},
				"b": {"type": "number", "description": "second number"}
			},
			"required": ["operation", "a", "b"]
		}
	}]`

	deltaChan, errChan := vcr.provider.GenerateStream(ctx,
		[]api.Message{
			{
				Role:    "user",
				Content: []api.ContentBlock{{Type: "input_text", Text: "What is 5 * 3? Use the calculator tool."}},
			},
		},
		&api.ResponseRequest{
			Model: vcr.model,
			Tools: json.RawMessage(toolsJSON),
		},
	)

	var toolCallDeltasCount int
	var lastErr error

	for deltaChan != nil || errChan != nil {
		select {
		case delta, ok := <-deltaChan:
			if !ok {
				deltaChan = nil
				continue
			}
			if delta.ToolCallDelta != nil {
				toolCallDeltasCount++
			}
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				continue
			}
			if err != nil {
				lastErr = err
			}
		}
	}

	require.NoError(t, lastErr)
	assert.Greater(t, toolCallDeltasCount, 0)
}

// Test: Invalid model error
func TestAzureAnthropicIntegration_Generate_InvalidModel(t *testing.T) {
	vcr := newAzureAnthropicVCRProvider(t, "azure_anthropic_generate_invalid_model")
	defer vcr.recorder.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := vcr.provider.Generate(ctx,
		[]api.Message{
			{
				Role:    "user",
				Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}},
			},
		},
		&api.ResponseRequest{
			Model: "nonexistent-model",
		},
	)

	assert.Error(t, err)
	assert.Nil(t, result)
}
