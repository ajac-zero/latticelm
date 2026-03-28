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
			name:      "empty array",
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

func TestBuildGeminiTextParts(t *testing.T) {
	tests := []struct {
		name        string
		blocks      []api.ContentBlock
		role        string
		expectError bool
		validate    func(t *testing.T, parts []*genai.Part)
	}{
		{
			name: "text only",
			blocks: []api.ContentBlock{
				{Type: "input_text", Text: "Hello"},
			},
			role: "user",
			validate: func(t *testing.T, parts []*genai.Part) {
				require.Len(t, parts, 1)
				assert.Equal(t, "Hello", parts[0].Text)
			},
		},
		{
			name: "text and image URL",
			blocks: []api.ContentBlock{
				{Type: "input_text", Text: "Describe this:"},
				{Type: "input_image", ImageURL: "https://example.com/image.png"},
			},
			role: "user",
			validate: func(t *testing.T, parts []*genai.Part) {
				require.Len(t, parts, 2)
				assert.Equal(t, "Describe this:", parts[0].Text)
				assert.NotNil(t, parts[1].FileData)
			},
		},
		{
			name: "image with base64 data URL",
			blocks: []api.ContentBlock{
				{Type: "input_image", ImageURL: "data:image/png;base64,iVBORw0KGgo="},
			},
			role: "user",
			validate: func(t *testing.T, parts []*genai.Part) {
				require.Len(t, parts, 1)
				assert.NotNil(t, parts[0].InlineData)
			},
		},
		{
			name: "video URL",
			blocks: []api.ContentBlock{
				{Type: "input_video", VideoURL: "https://example.com/video.mp4"},
			},
			role: "user",
			validate: func(t *testing.T, parts []*genai.Part) {
				require.Len(t, parts, 1)
				assert.NotNil(t, parts[0].FileData)
			},
		},
		{
			name: "file with URL",
			blocks: []api.ContentBlock{
				{Type: "input_file", FileURL: "https://example.com/doc.pdf"},
			},
			role: "user",
			validate: func(t *testing.T, parts []*genai.Part) {
				require.Len(t, parts, 1)
				assert.NotNil(t, parts[0].FileData)
			},
		},
		{
			name: "file with base64 data",
			blocks: []api.ContentBlock{
				{Type: "input_file", FileData: "data:application/pdf;base64,SGVsbG8gV29ybGQ="},
			},
			role: "user",
			validate: func(t *testing.T, parts []*genai.Part) {
				require.Len(t, parts, 1)
				assert.NotNil(t, parts[0].InlineData)
			},
		},
		{
			name: "file with raw base64 data",
			blocks: []api.ContentBlock{
				{Type: "input_file", Filename: "report.txt", FileData: "cmVwb3J0"},
			},
			role: "user",
			validate: func(t *testing.T, parts []*genai.Part) {
				require.Len(t, parts, 1)
				assert.NotNil(t, parts[0].InlineData)
				assert.Equal(t, "text/plain", parts[0].InlineData.MIMEType)
			},
		},
		{
			name: "video with base64 data URL",
			blocks: []api.ContentBlock{
				{Type: "input_video", VideoURL: "data:video/mp4;base64,SGVsbG8="},
			},
			role: "user",
			validate: func(t *testing.T, parts []*genai.Part) {
				require.Len(t, parts, 1)
				assert.NotNil(t, parts[0].InlineData)
				assert.Equal(t, "video/mp4", parts[0].InlineData.MIMEType)
			},
		},
		{
			name: "unsupported content type",
			blocks: []api.ContentBlock{
				{Type: "unknown_type"},
			},
			role:        "user",
			expectError: true,
		},
		{
			name: "image without URL fails",
			blocks: []api.ContentBlock{
				{Type: "input_image"},
			},
			role:        "user",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, err := buildGeminiTextParts(tt.blocks, tt.role)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, parts)
			}
		})
	}
}

func TestParseDataURL(t *testing.T) {
	tests := []struct {
		name        string
		dataURL     string
		expectMedia string
		expectData  string
		expectError bool
	}{
		{
			name:        "valid PDF data URL",
			dataURL:     "data:application/pdf;base64,SGVsbG8gV29ybGQ=",
			expectMedia: "application/pdf",
			expectData:  "SGVsbG8gV29ybGQ=",
		},
		{
			name:        "valid image data URL",
			dataURL:     "data:image/png;base64,iVBORw0KGgo=",
			expectMedia: "image/png",
			expectData:  "iVBORw0KGgo=",
		},
		{
			name:        "missing data prefix",
			dataURL:     "image/png;base64,SGVsbG8=",
			expectError: true,
		},
		{
			name:        "missing comma",
			dataURL:     "data:image/png;base64",
			expectError: true,
		},
		{
			name:        "missing base64 marker",
			dataURL:     "data:image/png,SGVsbG8=",
			expectError: true,
		},
		{
			name:        "invalid base64",
			dataURL:     "data:image/png;base64,not-valid-base64!!!",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mediaType, data, err := parseDataURL(tt.dataURL)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectMedia, mediaType)
			assert.Equal(t, tt.expectData, data)
		})
	}
}

func TestGuessMimeTypes(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		guessFunc    func(string) string
		expectedMime string
	}{
		{"png image", "https://example.com/image.png", guessImageMimeType, "image/png"},
		{"jpeg image", "https://example.com/image.jpg", guessImageMimeType, "image/jpeg"},
		{"jpeg image uppercase", "https://example.com/image.JPG", guessImageMimeType, "image/jpeg"},
		{"gif image", "https://example.com/image.gif", guessImageMimeType, "image/gif"},
		{"webp image", "https://example.com/image.webp", guessImageMimeType, "image/webp"},
		{"unknown image", "https://example.com/image.unknown", guessImageMimeType, "image/jpeg"},
		{"png image with query", "https://example.com/image.png?sig=1", guessImageMimeType, "image/png"},
		{"pdf file", "https://example.com/doc.pdf", guessFileMimeType, "application/pdf"},
		{"text file", "https://example.com/doc.txt", guessFileMimeType, "text/plain"},
		{"json file", "https://example.com/doc.json", guessFileMimeType, "application/json"},
		{"json file with query", "https://example.com/doc.json?download=1", guessFileMimeType, "application/json"},
		{"unknown file", "https://example.com/doc.unknown", guessFileMimeType, "application/octet-stream"},
		{"mp4 video", "https://example.com/video.mp4", guessVideoMimeType, "video/mp4"},
		{"webm video", "https://example.com/video.webm", guessVideoMimeType, "video/webm"},
		{"mov video", "https://example.com/video.mov", guessVideoMimeType, "video/quicktime"},
		{"unknown video", "https://example.com/video.unknown", guessVideoMimeType, "video/mp4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.guessFunc(tt.url)
			assert.Equal(t, tt.expectedMime, result)
		})
	}
}
