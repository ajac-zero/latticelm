package google

import (
	"encoding/json"
	"testing"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestParseTools(t *testing.T) {
	tests := []struct {
		name        string
		toolsJSON   string
		expectError bool
		validate    func(t *testing.T, tools []*genai.Tool)
	}{
		{
			name: "flat format tool",
			toolsJSON: `[{
				"type": "function",
				"name": "get_weather",
				"description": "Get the weather for a location",
				"parameters": {
					"type": "object",
					"properties": {
						"location": {"type": "string"}
					},
					"required": ["location"]
				}
			}]`,
			validate: func(t *testing.T, tools []*genai.Tool) {
				require.Len(t, tools, 1, "should have one tool")
				require.Len(t, tools[0].FunctionDeclarations, 1, "should have one function declaration")
				assert.Equal(t, "get_weather", tools[0].FunctionDeclarations[0].Name)
				assert.Equal(t, "Get the weather for a location", tools[0].FunctionDeclarations[0].Description)
			},
		},
		{
			name: "nested format tool",
			toolsJSON: `[{
				"type": "function",
				"function": {
					"name": "get_time",
					"description": "Get current time",
					"parameters": {
						"type": "object",
						"properties": {
							"timezone": {"type": "string"}
						}
					}
				}
			}]`,
			validate: func(t *testing.T, tools []*genai.Tool) {
				require.Len(t, tools, 1, "should have one tool")
				require.Len(t, tools[0].FunctionDeclarations, 1, "should have one function declaration")
				assert.Equal(t, "get_time", tools[0].FunctionDeclarations[0].Name)
				assert.Equal(t, "Get current time", tools[0].FunctionDeclarations[0].Description)
			},
		},
		{
			name: "multiple tools",
			toolsJSON: `[
				{"name": "tool1", "description": "First tool"},
				{"name": "tool2", "description": "Second tool"}
			]`,
			validate: func(t *testing.T, tools []*genai.Tool) {
				require.Len(t, tools, 1, "should consolidate into one tool")
				require.Len(t, tools[0].FunctionDeclarations, 2, "should have two function declarations")
			},
		},
		{
			name: "tool without description",
			toolsJSON: `[{
				"name": "simple_tool",
				"parameters": {"type": "object"}
			}]`,
			validate: func(t *testing.T, tools []*genai.Tool) {
				require.Len(t, tools, 1, "should have one tool")
				assert.Equal(t, "simple_tool", tools[0].FunctionDeclarations[0].Name)
				assert.Empty(t, tools[0].FunctionDeclarations[0].Description)
			},
		},
		{
			name: "tool without parameters",
			toolsJSON: `[{
				"name": "paramless_tool",
				"description": "No params"
			}]`,
			validate: func(t *testing.T, tools []*genai.Tool) {
				require.Len(t, tools, 1, "should have one tool")
				assert.Nil(t, tools[0].FunctionDeclarations[0].ParametersJsonSchema)
			},
		},
		{
			name: "tool without name (should skip)",
			toolsJSON: `[{
				"description": "No name tool",
				"parameters": {"type": "object"}
			}]`,
			validate: func(t *testing.T, tools []*genai.Tool) {
				assert.Nil(t, tools, "should return nil when no valid tools")
			},
		},
		{
			name:        "nil tools",
			toolsJSON:   "",
			expectError: false,
			validate: func(t *testing.T, tools []*genai.Tool) {
				assert.Nil(t, tools, "should return nil for empty tools")
			},
		},
		{
			name:        "invalid JSON",
			toolsJSON:   `{not valid json}`,
			expectError: true,
		},
		{
			name: "empty array",
			toolsJSON: `[]`,
			validate: func(t *testing.T, tools []*genai.Tool) {
				assert.Nil(t, tools, "should return nil for empty array")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req api.ResponseRequest
			if tt.toolsJSON != "" {
				req.Tools = json.RawMessage(tt.toolsJSON)
			}

			tools, err := parseTools(&req)

			if tt.expectError {
				assert.Error(t, err, "expected an error")
				return
			}

			require.NoError(t, err, "unexpected error")
			if tt.validate != nil {
				tt.validate(t, tools)
			}
		})
	}
}

func TestParseToolChoice(t *testing.T) {
	tests := []struct {
		name        string
		choiceJSON  string
		expectError bool
		validate    func(t *testing.T, config *genai.ToolConfig)
	}{
		{
			name:       "auto mode",
			choiceJSON: `"auto"`,
			validate: func(t *testing.T, config *genai.ToolConfig) {
				require.NotNil(t, config, "config should not be nil")
				require.NotNil(t, config.FunctionCallingConfig, "function calling config should be set")
				assert.Equal(t, genai.FunctionCallingConfigModeAuto, config.FunctionCallingConfig.Mode)
			},
		},
		{
			name:       "none mode",
			choiceJSON: `"none"`,
			validate: func(t *testing.T, config *genai.ToolConfig) {
				require.NotNil(t, config, "config should not be nil")
				assert.Equal(t, genai.FunctionCallingConfigModeNone, config.FunctionCallingConfig.Mode)
			},
		},
		{
			name:       "required mode",
			choiceJSON: `"required"`,
			validate: func(t *testing.T, config *genai.ToolConfig) {
				require.NotNil(t, config, "config should not be nil")
				assert.Equal(t, genai.FunctionCallingConfigModeAny, config.FunctionCallingConfig.Mode)
			},
		},
		{
			name:       "any mode",
			choiceJSON: `"any"`,
			validate: func(t *testing.T, config *genai.ToolConfig) {
				require.NotNil(t, config, "config should not be nil")
				assert.Equal(t, genai.FunctionCallingConfigModeAny, config.FunctionCallingConfig.Mode)
			},
		},
		{
			name:       "specific function",
			choiceJSON: `{"type": "function", "function": {"name": "get_weather"}}`,
			validate: func(t *testing.T, config *genai.ToolConfig) {
				require.NotNil(t, config, "config should not be nil")
				assert.Equal(t, genai.FunctionCallingConfigModeAny, config.FunctionCallingConfig.Mode)
				require.Len(t, config.FunctionCallingConfig.AllowedFunctionNames, 1)
				assert.Equal(t, "get_weather", config.FunctionCallingConfig.AllowedFunctionNames[0])
			},
		},
		{
			name:       "nil tool choice",
			choiceJSON: "",
			validate: func(t *testing.T, config *genai.ToolConfig) {
				assert.Nil(t, config, "should return nil for empty choice")
			},
		},
		{
			name:        "unknown string mode",
			choiceJSON:  `"unknown_mode"`,
			expectError: true,
		},
		{
			name:        "invalid JSON",
			choiceJSON:  `{invalid}`,
			expectError: true,
		},
		{
			name:        "unsupported object format",
			choiceJSON:  `{"type": "unsupported"}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req api.ResponseRequest
			if tt.choiceJSON != "" {
				req.ToolChoice = json.RawMessage(tt.choiceJSON)
			}

			config, err := parseToolChoice(&req)

			if tt.expectError {
				assert.Error(t, err, "expected an error")
				return
			}

			require.NoError(t, err, "unexpected error")
			if tt.validate != nil {
				tt.validate(t, config)
			}
		})
	}
}

func TestExtractToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *genai.GenerateContentResponse
		validate func(t *testing.T, toolCalls []api.ToolCall)
	}{
		{
			name: "single tool call",
			setup: func() *genai.GenerateContentResponse {
				args := map[string]interface{}{
					"location": "San Francisco",
				}
				return &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{
						{
							Content: &genai.Content{
								Parts: []*genai.Part{
									{
										FunctionCall: &genai.FunctionCall{
											ID:   "call_123",
											Name: "get_weather",
											Args: args,
										},
									},
								},
							},
						},
					},
				}
			},
			validate: func(t *testing.T, toolCalls []api.ToolCall) {
				require.Len(t, toolCalls, 1)
				assert.Equal(t, "call_123", toolCalls[0].ID)
				assert.Equal(t, "get_weather", toolCalls[0].Name)
				assert.Contains(t, toolCalls[0].Arguments, "location")
			},
		},
		{
			name: "tool call without ID generates one",
			setup: func() *genai.GenerateContentResponse {
				return &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{
						{
							Content: &genai.Content{
								Parts: []*genai.Part{
									{
										FunctionCall: &genai.FunctionCall{
											Name: "get_time",
											Args: map[string]interface{}{},
										},
									},
								},
							},
						},
					},
				}
			},
			validate: func(t *testing.T, toolCalls []api.ToolCall) {
				require.Len(t, toolCalls, 1)
				assert.NotEmpty(t, toolCalls[0].ID, "should generate ID")
				assert.Contains(t, toolCalls[0].ID, "call_")
			},
		},
		{
			name: "response with nil candidates",
			setup: func() *genai.GenerateContentResponse {
				return &genai.GenerateContentResponse{
					Candidates: nil,
				}
			},
			validate: func(t *testing.T, toolCalls []api.ToolCall) {
				assert.Nil(t, toolCalls)
			},
		},
		{
			name: "empty candidates",
			setup: func() *genai.GenerateContentResponse {
				return &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{},
				}
			},
			validate: func(t *testing.T, toolCalls []api.ToolCall) {
				assert.Nil(t, toolCalls)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := tt.setup()
			toolCalls := extractToolCalls(resp)
			tt.validate(t, toolCalls)
		})
	}
}

func TestGenerateRandomID(t *testing.T) {
	t.Run("generates non-empty ID", func(t *testing.T) {
		id := generateRandomID()
		assert.NotEmpty(t, id)
		assert.Equal(t, 24, len(id), "ID should be 24 characters")
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		id1 := generateRandomID()
		id2 := generateRandomID()
		assert.NotEqual(t, id1, id2, "IDs should be unique")
	})

	t.Run("only contains valid characters", func(t *testing.T) {
		id := generateRandomID()
		for _, c := range id {
			assert.True(t, (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'),
				"ID should only contain lowercase letters and numbers")
		}
	})
}
