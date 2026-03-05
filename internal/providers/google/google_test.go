package google

import (
	"context"
	"testing"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.ProviderConfig
		expectError bool
		validate    func(t *testing.T, p *Provider, err error)
	}{
		{
			name: "creates provider with API key",
			cfg: config.ProviderConfig{
				APIKey: "test-api-key",
				Model:  "gemini-2.0-flash",
			},
			expectError: false,
			validate: func(t *testing.T, p *Provider, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, p)
				assert.NotNil(t, p.client)
				assert.Equal(t, "test-api-key", p.cfg.APIKey)
				assert.Equal(t, "gemini-2.0-flash", p.cfg.Model)
			},
		},
		{
			name: "creates provider without API key",
			cfg: config.ProviderConfig{
				APIKey: "",
			},
			expectError: false,
			validate: func(t *testing.T, p *Provider, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, p)
				assert.Nil(t, p.client)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := New(tt.cfg)
			tt.validate(t, p, err)
		})
	}
}

func TestNewVertexAI(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.VertexAIConfig
		expectError bool
		validate    func(t *testing.T, p *Provider, err error)
	}{
		{
			name: "creates Vertex AI provider with project and location",
			cfg: config.VertexAIConfig{
				Project:  "my-gcp-project",
				Location: "us-central1",
			},
			expectError: false,
			validate: func(t *testing.T, p *Provider, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, p)
				// Client creation may fail without proper GCP credentials in test env
				// but provider should be created
			},
		},
		{
			name: "creates Vertex AI provider without project",
			cfg: config.VertexAIConfig{
				Project:  "",
				Location: "us-central1",
			},
			expectError: false,
			validate: func(t *testing.T, p *Provider, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, p)
				assert.Nil(t, p.client)
			},
		},
		{
			name: "creates Vertex AI provider without location",
			cfg: config.VertexAIConfig{
				Project:  "my-gcp-project",
				Location: "",
			},
			expectError: false,
			validate: func(t *testing.T, p *Provider, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, p)
				assert.Nil(t, p.client)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewVertexAI(tt.cfg)
			tt.validate(t, p, err)
		})
	}
}

func TestProvider_Name(t *testing.T) {
	p := &Provider{}
	assert.Equal(t, "google", p.Name())
}

func TestProvider_Generate_Validation(t *testing.T) {
	tests := []struct {
		name        string
		provider    *Provider
		messages    []api.Message
		req         *api.ResponseRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "returns error when client not initialized",
			provider: &Provider{
				client: nil,
			},
			messages: []api.Message{
				{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
			},
			req: &api.ResponseRequest{
				Model: "gemini-2.0-flash",
			},
			expectError: true,
			errorMsg:    "client not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.provider.Generate(context.Background(), tt.messages, tt.req)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestProvider_GenerateStream_Validation(t *testing.T) {
	tests := []struct {
		name        string
		provider    *Provider
		messages    []api.Message
		req         *api.ResponseRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "returns error when client not initialized",
			provider: &Provider{
				client: nil,
			},
			messages: []api.Message{
				{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
			},
			req: &api.ResponseRequest{
				Model: "gemini-2.0-flash",
			},
			expectError: true,
			errorMsg:    "client not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deltaChan, errChan := tt.provider.GenerateStream(context.Background(), tt.messages, tt.req)

			// Read from channels
			var receivedError error
			for {
				select {
				case _, ok := <-deltaChan:
					if !ok {
						deltaChan = nil
					}
				case err, ok := <-errChan:
					if ok && err != nil {
						receivedError = err
					}
					errChan = nil
				}

				if deltaChan == nil && errChan == nil {
					break
				}
			}

			if tt.expectError {
				require.Error(t, receivedError)
				if tt.errorMsg != "" {
					assert.Contains(t, receivedError.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, receivedError)
			}
		})
	}
}

func TestConvertMessages(t *testing.T) {
	tests := []struct {
		name             string
		messages         []api.Message
		expectedContents int
		expectedSystem   string
		validate         func(t *testing.T, contents []*genai.Content, systemText string)
	}{
		{
			name: "converts user message",
			messages: []api.Message{
				{
					Role: "user",
					Content: []api.ContentBlock{
						{Type: "input_text", Text: "Hello"},
					},
				},
			},
			expectedContents: 1,
			expectedSystem:   "",
			validate: func(t *testing.T, contents []*genai.Content, systemText string) {
				require.Len(t, contents, 1)
				assert.Equal(t, "user", contents[0].Role)
				assert.Equal(t, "", systemText)
			},
		},
		{
			name: "extracts system message",
			messages: []api.Message{
				{
					Role: "system",
					Content: []api.ContentBlock{
						{Type: "input_text", Text: "You are a helpful assistant"},
					},
				},
				{
					Role: "user",
					Content: []api.ContentBlock{
						{Type: "input_text", Text: "Hello"},
					},
				},
			},
			expectedContents: 1,
			expectedSystem:   "You are a helpful assistant",
			validate: func(t *testing.T, contents []*genai.Content, systemText string) {
				require.Len(t, contents, 1)
				assert.Equal(t, "You are a helpful assistant", systemText)
				assert.Equal(t, "user", contents[0].Role)
			},
		},
		{
			name: "converts assistant message with tool calls",
			messages: []api.Message{
				{
					Role: "assistant",
					Content: []api.ContentBlock{
						{Type: "output_text", Text: "Let me check the weather"},
					},
					ToolCalls: []api.ToolCall{
						{
							ID:        "call_123",
							Name:      "get_weather",
							Arguments: `{"location": "SF"}`,
						},
					},
				},
			},
			expectedContents: 1,
			validate: func(t *testing.T, contents []*genai.Content, systemText string) {
				require.Len(t, contents, 1)
				assert.Equal(t, "model", contents[0].Role)
				// Should have text part and function call part
				assert.GreaterOrEqual(t, len(contents[0].Parts), 1)
			},
		},
		{
			name: "converts tool result message",
			messages: []api.Message{
				{
					Role: "assistant",
					ToolCalls: []api.ToolCall{
						{ID: "call_123", Name: "get_weather", Arguments: "{}"},
					},
				},
				{
					Role:   "tool",
					CallID: "call_123",
					Name:   "get_weather",
					Content: []api.ContentBlock{
						{Type: "output_text", Text: `{"temp": 72}`},
					},
				},
			},
			expectedContents: 2,
			validate: func(t *testing.T, contents []*genai.Content, systemText string) {
				require.Len(t, contents, 2)
				// Tool result should be in user role
				assert.Equal(t, "user", contents[1].Role)
				require.Len(t, contents[1].Parts, 1)
				assert.NotNil(t, contents[1].Parts[0].FunctionResponse)
			},
		},
		{
			name: "handles developer message as system",
			messages: []api.Message{
				{
					Role: "developer",
					Content: []api.ContentBlock{
						{Type: "input_text", Text: "Developer instruction"},
					},
				},
			},
			expectedContents: 0,
			expectedSystem:   "Developer instruction",
			validate: func(t *testing.T, contents []*genai.Content, systemText string) {
				assert.Len(t, contents, 0)
				assert.Equal(t, "Developer instruction", systemText)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contents, systemText := convertMessages(tt.messages)
			assert.Len(t, contents, tt.expectedContents)
			assert.Equal(t, tt.expectedSystem, systemText)
			if tt.validate != nil {
				tt.validate(t, contents, systemText)
			}
		})
	}
}

func TestBuildConfig(t *testing.T) {
	tests := []struct {
		name       string
		systemText string
		req        *api.ResponseRequest
		tools      []*genai.Tool
		toolConfig *genai.ToolConfig
		expectNil  bool
		validate   func(t *testing.T, cfg *genai.GenerateContentConfig)
	}{
		{
			name:       "returns nil when no config needed",
			systemText: "",
			req:        &api.ResponseRequest{},
			tools:      nil,
			toolConfig: nil,
			expectNil:  true,
		},
		{
			name:       "creates config with system text",
			systemText: "You are helpful",
			req:        &api.ResponseRequest{},
			expectNil:  false,
			validate: func(t *testing.T, cfg *genai.GenerateContentConfig) {
				require.NotNil(t, cfg)
				require.NotNil(t, cfg.SystemInstruction)
				assert.Len(t, cfg.SystemInstruction.Parts, 1)
			},
		},
		{
			name:       "creates config with max tokens",
			systemText: "",
			req: &api.ResponseRequest{
				MaxOutputTokens: intPtr(1000),
			},
			expectNil: false,
			validate: func(t *testing.T, cfg *genai.GenerateContentConfig) {
				require.NotNil(t, cfg)
				assert.Equal(t, int32(1000), cfg.MaxOutputTokens)
			},
		},
		{
			name:       "creates config with temperature",
			systemText: "",
			req: &api.ResponseRequest{
				Temperature: float64Ptr(0.7),
			},
			expectNil: false,
			validate: func(t *testing.T, cfg *genai.GenerateContentConfig) {
				require.NotNil(t, cfg)
				require.NotNil(t, cfg.Temperature)
				assert.Equal(t, float32(0.7), *cfg.Temperature)
			},
		},
		{
			name:       "creates config with top_p",
			systemText: "",
			req: &api.ResponseRequest{
				TopP: float64Ptr(0.9),
			},
			expectNil: false,
			validate: func(t *testing.T, cfg *genai.GenerateContentConfig) {
				require.NotNil(t, cfg)
				require.NotNil(t, cfg.TopP)
				assert.Equal(t, float32(0.9), *cfg.TopP)
			},
		},
		{
			name:       "creates config with tools",
			systemText: "",
			req:        &api.ResponseRequest{},
			tools: []*genai.Tool{
				{
					FunctionDeclarations: []*genai.FunctionDeclaration{
						{Name: "get_weather"},
					},
				},
			},
			expectNil: false,
			validate: func(t *testing.T, cfg *genai.GenerateContentConfig) {
				require.NotNil(t, cfg)
				require.Len(t, cfg.Tools, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := buildConfig(tt.systemText, tt.req, tt.tools, tt.toolConfig)
			if tt.expectNil {
				assert.Nil(t, cfg)
			} else {
				require.NotNil(t, cfg)
				if tt.validate != nil {
					tt.validate(t, cfg)
				}
			}
		})
	}
}

func TestChooseModel(t *testing.T) {
	tests := []struct {
		name         string
		requested    string
		defaultModel string
		expected     string
	}{
		{
			name:         "returns requested model when provided",
			requested:    "gemini-1.5-pro",
			defaultModel: "gemini-2.0-flash",
			expected:     "gemini-1.5-pro",
		},
		{
			name:         "returns default model when requested is empty",
			requested:    "",
			defaultModel: "gemini-2.0-flash",
			expected:     "gemini-2.0-flash",
		},
		{
			name:         "returns fallback when both empty",
			requested:    "",
			defaultModel: "",
			expected:     "gemini-2.0-flash-exp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := chooseModel(tt.requested, tt.defaultModel)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractToolCallDelta(t *testing.T) {
	tests := []struct {
		name     string
		part     *genai.Part
		index    int
		expected *api.ToolCallDelta
	}{
		{
			name: "extracts tool call delta",
			part: &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   "call_123",
					Name: "get_weather",
					Args: map[string]any{"location": "SF"},
				},
			},
			index: 0,
			expected: &api.ToolCallDelta{
				Index:     0,
				ID:        "call_123",
				Name:      "get_weather",
				Arguments: `{"location":"SF"}`,
			},
		},
		{
			name:     "returns nil for nil part",
			part:     nil,
			index:    0,
			expected: nil,
		},
		{
			name:     "returns nil for part without function call",
			part:     &genai.Part{Text: "Hello"},
			index:    0,
			expected: nil,
		},
		{
			name: "generates ID when not provided",
			part: &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   "",
					Name: "get_time",
					Args: map[string]any{},
				},
			},
			index: 1,
			expected: &api.ToolCallDelta{
				Index:     1,
				Name:      "get_time",
				Arguments: `{}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractToolCallDelta(tt.part, tt.index)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Index, result.Index)
				assert.Equal(t, tt.expected.Name, result.Name)
				if tt.part != nil && tt.part.FunctionCall != nil && tt.part.FunctionCall.ID != "" {
					assert.Equal(t, tt.expected.ID, result.ID)
				} else if tt.expected.ID == "" {
					// Generated ID should start with "call_"
					assert.Contains(t, result.ID, "call_")
				}
			}
		})
	}
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func float64Ptr(f float64) *float64 {
	return &f
}
