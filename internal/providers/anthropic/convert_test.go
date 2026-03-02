package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/ajac-zero/latticelm/internal/api"
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
		name        string
		choiceJSON  string
		expectAuto  bool
		expectAny   bool
		expectTool  bool
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
