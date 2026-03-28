package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTools(t *testing.T) {
	// Create a sample tool definition
	toolsJSON := `[{
		"type": "function",
		"name": "get_weather",
		"description": "Get the weather for a location",
		"parameters": {
			"type": "object",
			"properties": {
				"location": {
					"type": "string",
					"description": "The city and state"
				}
			},
			"required": ["location"]
		}
	}]`

	req := &api.ResponseRequest{
		Tools: json.RawMessage(toolsJSON),
	}

	tools, err := parseTools(req)
	if err != nil {
		t.Fatalf("parseTools failed: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	tool := tools[0]
	if tool.OfTool == nil {
		t.Fatal("expected OfTool to be set")
	}

	if tool.OfTool.Name != "get_weather" {
		t.Errorf("expected name 'get_weather', got '%s'", tool.OfTool.Name)
	}

	desc := tool.GetDescription()
	if desc == nil || *desc != "Get the weather for a location" {
		t.Errorf("expected description 'Get the weather for a location', got '%v'", desc)
	}

	if len(tool.OfTool.InputSchema.Required) != 1 || tool.OfTool.InputSchema.Required[0] != "location" {
		t.Errorf("expected required=['location'], got %v", tool.OfTool.InputSchema.Required)
	}
}

func TestParseToolChoice(t *testing.T) {
	tests := []struct {
		name         string
		choiceJSON   string
		expectAuto   bool
		expectAny    bool
		expectTool   bool
		expectedName string
	}{
		{
			name:       "auto",
			choiceJSON: `"auto"`,
			expectAuto: true,
		},
		{
			name:       "any",
			choiceJSON: `"any"`,
			expectAny:  true,
		},
		{
			name:       "required",
			choiceJSON: `"required"`,
			expectAny:  true,
		},
		{
			name:         "specific tool",
			choiceJSON:   `{"type": "function", "function": {"name": "get_weather"}}`,
			expectTool:   true,
			expectedName: "get_weather",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &api.ResponseRequest{
				ToolChoice: json.RawMessage(tt.choiceJSON),
			}

			choice, err := parseToolChoice(req)
			if err != nil {
				t.Fatalf("parseToolChoice failed: %v", err)
			}

			if tt.expectAuto && choice.OfAuto == nil {
				t.Error("expected OfAuto to be set")
			}
			if tt.expectAny && choice.OfAny == nil {
				t.Error("expected OfAny to be set")
			}
			if tt.expectTool {
				if choice.OfTool == nil {
					t.Fatal("expected OfTool to be set")
				}
				if choice.OfTool.Name != tt.expectedName {
					t.Errorf("expected name '%s', got '%s'", tt.expectedName, choice.OfTool.Name)
				}
			}
		})
	}
}

func TestBuildAnthropicTextBlocks(t *testing.T) {
	tests := []struct {
		name        string
		blocks      []api.ContentBlock
		role        string
		expectError bool
		validate    func(t *testing.T, blocks []interface{})
	}{
		{
			name: "text only",
			blocks: []api.ContentBlock{
				{Type: "input_text", Text: "Hello"},
			},
			role: "user",
			validate: func(t *testing.T, blocks []interface{}) {
				assert.Len(t, blocks, 1)
			},
		},
		{
			name: "text and image URL",
			blocks: []api.ContentBlock{
				{Type: "input_text", Text: "Describe this:"},
				{Type: "input_image", ImageURL: "https://example.com/image.png"},
			},
			role: "user",
			validate: func(t *testing.T, blocks []interface{}) {
				assert.Len(t, blocks, 2)
			},
		},
		{
			name: "image with base64 data URL",
			blocks: []api.ContentBlock{
				{Type: "input_image", ImageURL: "data:image/png;base64,iVBORw0KGgo="},
			},
			role: "user",
			validate: func(t *testing.T, blocks []interface{}) {
				assert.Len(t, blocks, 1)
			},
		},
		{
			name: "document with file URL",
			blocks: []api.ContentBlock{
				{Type: "input_file", FileURL: "https://example.com/doc.pdf"},
			},
			role: "user",
			validate: func(t *testing.T, blocks []interface{}) {
				assert.Len(t, blocks, 1)
			},
		},
		{
			name: "document with base64 data",
			blocks: []api.ContentBlock{
				{Type: "input_file", FileData: "data:application/pdf;base64,SGVsbG8gV29ybGQ="},
			},
			role: "user",
			validate: func(t *testing.T, blocks []interface{}) {
				assert.Len(t, blocks, 1)
			},
		},
		{
			name: "text document with raw base64 data",
			blocks: []api.ContentBlock{
				{Type: "input_file", Filename: "notes.txt", FileData: "Zm9vYmFy"},
			},
			role: "user",
			validate: func(t *testing.T, blocks []interface{}) {
				assert.Len(t, blocks, 1)
			},
		},
		{
			name: "unsupported content type",
			blocks: []api.ContentBlock{
				{Type: "input_video", VideoURL: "https://example.com/video.mp4"},
			},
			role:        "user",
			expectError: true,
		},
		{
			name: "non-pdf document URL fails",
			blocks: []api.ContentBlock{
				{Type: "input_file", FileURL: "https://example.com/doc.txt"},
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
			blocks, err := buildAnthropicTextBlocks(tt.blocks, tt.role)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.validate != nil {
				var blocksInterface []interface{}
				for _, b := range blocks {
					blocksInterface = append(blocksInterface, b)
				}
				tt.validate(t, blocksInterface)
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
