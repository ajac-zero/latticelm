//go:build integration

package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/azure"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/config"
)

const (
	envAzureAPIKey     = "AZURE_OPENAI_API_KEY"
	envAzureEndpoint   = "AZURE_OPENAI_ENDPOINT"
	envAzureModel      = "AZURE_OPENAI_MODEL"
	envAzureAPIVersion = "AZURE_OPENAI_API_VERSION"

	defaultAzureAPIVersion = "2024-12-01-preview"
)

type azureTestEnv struct {
	apiKey     string
	endpoint   string
	model      string
	apiVersion string
}

func loadAzureEnv(t *testing.T) azureTestEnv {
	t.Helper()
	env := azureTestEnv{
		apiKey:     os.Getenv(envAzureAPIKey),
		endpoint:   os.Getenv(envAzureEndpoint),
		model:      os.Getenv(envAzureModel),
		apiVersion: os.Getenv(envAzureAPIVersion),
	}
	if env.apiVersion == "" {
		env.apiVersion = defaultAzureAPIVersion
	}
	return env
}

type azureVCRResult struct {
	provider *Provider
	recorder *recorder.Recorder
	model    string // model used (from env or extracted from cassette)
}

func newAzureVCRProvider(t *testing.T, cassetteName string) azureVCRResult {
	t.Helper()
	env := loadAzureEnv(t)

	cassettePath := "testdata/cassettes/" + cassetteName
	hasCredentials := env.apiKey != "" && env.endpoint != ""
	hasCassette := cassetteFileExists(cassettePath)

	if !hasCredentials && !hasCassette {
		t.Skipf("no cassette at %s.yaml and no Azure credentials set; skipping", cassettePath)
	}

	mode := recorder.ModeReplayOnly
	if hasCredentials {
		mode = recorder.ModeRecordOnly
	}

	rec, err := recorder.New(cassettePath,
		recorder.WithMode(mode),
		recorder.WithSkipRequestLatency(true),
		recorder.WithHook(scrubAzureCassette, recorder.AfterCaptureHook),
		recorder.WithMatcher(matchByMethodAndBody),
	)
	require.NoError(t, err)

	httpClient := rec.GetDefaultClient()

	endpoint := env.endpoint
	apiVersion := env.apiVersion
	apiKey := env.apiKey
	model := env.model

	if mode == recorder.ModeReplayOnly {
		apiKey = "replayed"
		if endpoint == "" {
			endpoint = "https://replayed.openai.azure.com"
		}
		if model == "" {
			model = modelFromCassette(t, cassettePath)
		}
	}

	if model == "" {
		model = "gpt-4o-mini"
	}

	c := openai.NewClient(
		azure.WithEndpoint(endpoint, apiVersion),
		azure.WithAPIKey(apiKey),
		option.WithHTTPClient(httpClient),
	)

	p := &Provider{
		cfg:    config.ProviderConfig{APIKey: apiKey},
		client: &c,
		azure:  true,
	}

	return azureVCRResult{provider: p, recorder: rec, model: model}
}

func cassetteFileExists(path string) bool {
	_, err := os.Stat(path + ".yaml")
	return err == nil
}

// modelFromCassette extracts the model from the first recorded request body.
func modelFromCassette(t *testing.T, path string) string {
	t.Helper()
	c, err := cassette.Load(path)
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

func scrubAzureCassette(i *cassette.Interaction) error {
	i.Request.Headers.Del("Api-Key")
	i.Request.Headers.Del("Authorization")

	if u, err := url.Parse(i.Request.URL); err == nil {
		u.Host = "scrubbed.openai.azure.com"
		i.Request.URL = u.String()
	}
	return nil
}

// matchByMethodAndBody matches requests by HTTP method and request body,
// ignoring the URL entirely. Azure OpenAI embeds the deployment/model name
// in the URL path, which differs between record and replay. The body
// (which contains the model field and messages) is the reliable discriminator.
func matchByMethodAndBody(r *http.Request, i cassette.Request) bool {
	if r.Method != i.Method {
		return false
	}

	// For POST requests, compare the URL path suffix after /deployments/<model>/
	// to distinguish between chat/completions endpoints.
	if r.Method == http.MethodPost {
		reqSuffix := pathSuffix(r.URL.Path)
		cassURL, err := url.Parse(i.URL)
		if err != nil {
			return false
		}
		cassSuffix := pathSuffix(cassURL.Path)
		if reqSuffix != cassSuffix {
			return false
		}

		// Compare request bodies (ignoring the model field which may differ
		// due to deployment naming).
		reqBody := readRequestBody(r)
		return normalizeBody(reqBody) == normalizeBody(i.Body)
	}

	return true
}

// pathSuffix returns the path after /deployments/<name>/, e.g. "chat/completions".
func pathSuffix(p string) string {
	const marker = "/deployments/"
	idx := strings.Index(p, marker)
	if idx == -1 {
		return p
	}
	rest := p[idx+len(marker):]
	// Skip the deployment name segment
	if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
		return rest[slashIdx+1:]
	}
	return rest
}

// readRequestBody reads the body from an http.Request and resets it.
func readRequestBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return string(data)
}

// normalizeBody strips the "model" field for comparison since Azure
// deployment names may not match the model field in the request.
func normalizeBody(body string) string {
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

// --- Sync tests ---

func TestAzureIntegration_Generate_BasicText(t *testing.T) {
	vcr := newAzureVCRProvider(t, "azure_generate_basic_text")
	defer vcr.recorder.Stop()

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Say hello in exactly 3 words."}}},
	}
	req := &api.ResponseRequest{Model: vcr.model}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := vcr.provider.Generate(ctx, messages, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotEmpty(t, result.Text)
	assert.NotEmpty(t, result.Model)
	assert.Greater(t, result.Usage.InputTokens, 0)
	assert.Greater(t, result.Usage.OutputTokens, 0)
	assert.Greater(t, result.Usage.TotalTokens, 0)
}

func TestAzureIntegration_Generate_WithSystemPrompt(t *testing.T) {
	vcr := newAzureVCRProvider(t, "azure_generate_system_prompt")
	defer vcr.recorder.Stop()

	messages := []api.Message{
		{Role: "developer", Content: []api.ContentBlock{{Type: "input_text", Text: "Always respond with exactly one word."}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "What color is the sky?"}}},
	}
	req := &api.ResponseRequest{Model: vcr.model}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := vcr.provider.Generate(ctx, messages, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotEmpty(t, result.Text)
}

func TestAzureIntegration_Generate_ToolCalling(t *testing.T) {
	vcr := newAzureVCRProvider(t, "azure_generate_tool_calling")
	defer vcr.recorder.Stop()

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "What is the weather in San Francisco?"}}},
	}
	toolsJSON := `[{
		"type": "function",
		"name": "get_weather",
		"description": "Get the current weather for a location",
		"parameters": {
			"type": "object",
			"properties": {
				"location": {"type": "string", "description": "City and state"}
			},
			"required": ["location"]
		}
	}]`
	req := &api.ResponseRequest{
		Model: vcr.model,
		Tools: []byte(toolsJSON),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := vcr.provider.Generate(ctx, messages, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NotEmpty(t, result.ToolCalls, "expected at least one tool call")
	tc := result.ToolCalls[0]
	assert.Equal(t, "get_weather", tc.Name)
	assert.NotEmpty(t, tc.ID)
	assert.NotEmpty(t, tc.Arguments)
}

func TestAzureIntegration_Generate_MultiTurn(t *testing.T) {
	vcr := newAzureVCRProvider(t, "azure_generate_multi_turn")
	defer vcr.recorder.Stop()

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "My name is Alice."}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "output_text", Text: "Hello Alice! How can I help you?"}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "What is my name?"}}},
	}
	req := &api.ResponseRequest{Model: vcr.model}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := vcr.provider.Generate(ctx, messages, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.Text, "Alice")
}

// --- Streaming tests ---

func TestAzureIntegration_GenerateStream_BasicText(t *testing.T) {
	vcr := newAzureVCRProvider(t, "azure_stream_basic_text")
	defer vcr.recorder.Stop()

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Count from 1 to 5."}}},
	}
	req := &api.ResponseRequest{Model: vcr.model}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deltaChan, errChan := vcr.provider.GenerateStream(ctx, messages, req)

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

func TestAzureIntegration_GenerateStream_ToolCalling(t *testing.T) {
	vcr := newAzureVCRProvider(t, "azure_stream_tool_calling")
	defer vcr.recorder.Stop()

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "What is the weather in San Francisco?"}}},
	}
	toolsJSON := `[{
		"type": "function",
		"name": "get_weather",
		"description": "Get the current weather for a location",
		"parameters": {
			"type": "object",
			"properties": {
				"location": {"type": "string", "description": "City and state"}
			},
			"required": ["location"]
		}
	}]`
	req := &api.ResponseRequest{
		Model: vcr.model,
		Tools: []byte(toolsJSON),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deltaChan, errChan := vcr.provider.GenerateStream(ctx, messages, req)

	var gotToolDelta bool
	var gotDone bool
	var lastErr error
	toolArgs := make(map[int]string)

	for deltaChan != nil || errChan != nil {
		select {
		case delta, ok := <-deltaChan:
			if !ok {
				deltaChan = nil
				continue
			}
			if delta.Done {
				gotDone = true
			}
			if delta.ToolCallDelta != nil {
				gotToolDelta = true
				toolArgs[delta.ToolCallDelta.Index] += delta.ToolCallDelta.Arguments
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
	assert.True(t, gotToolDelta, "expected tool call deltas")
	assert.NotEmpty(t, toolArgs, "expected accumulated tool call arguments")
}

// --- Error case ---

func TestAzureIntegration_Generate_InvalidModel(t *testing.T) {
	vcr := newAzureVCRProvider(t, "azure_generate_invalid_model")
	defer vcr.recorder.Stop()

	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
	}
	req := &api.ResponseRequest{Model: "nonexistent-model-xyz"}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := vcr.provider.Generate(ctx, messages, req)
	assert.Error(t, err)
	assert.Nil(t, result)
}


