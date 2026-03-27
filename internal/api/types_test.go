package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputUnion_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		validate    func(t *testing.T, u InputUnion)
	}{
		{
			name:  "string input",
			input: `"hello world"`,
			validate: func(t *testing.T, u InputUnion) {
				require.NotNil(t, u.String)
				assert.Equal(t, "hello world", *u.String)
				assert.Nil(t, u.Items)
			},
		},
		{
			name:  "empty string input",
			input: `""`,
			validate: func(t *testing.T, u InputUnion) {
				require.NotNil(t, u.String)
				assert.Equal(t, "", *u.String)
				assert.Nil(t, u.Items)
			},
		},
		{
			name:  "null input",
			input: `null`,
			validate: func(t *testing.T, u InputUnion) {
				assert.Nil(t, u.String)
				assert.Nil(t, u.Items)
			},
		},
		{
			name: "array input with single message",
			input: `[{
				"type": "message",
				"role": "user",
				"content": "hello"
			}]`,
			validate: func(t *testing.T, u InputUnion) {
				assert.Nil(t, u.String)
				require.Len(t, u.Items, 1)
				assert.Equal(t, "message", u.Items[0].Type)
				assert.Equal(t, "user", u.Items[0].Role)
			},
		},
		{
			name: "array input with multiple messages",
			input: `[{
				"type": "message",
				"role": "user",
				"content": "hello"
			}, {
				"type": "message",
				"role": "assistant",
				"content": "hi there"
			}]`,
			validate: func(t *testing.T, u InputUnion) {
				assert.Nil(t, u.String)
				require.Len(t, u.Items, 2)
				assert.Equal(t, "user", u.Items[0].Role)
				assert.Equal(t, "assistant", u.Items[1].Role)
			},
		},
		{
			name:  "empty array",
			input: `[]`,
			validate: func(t *testing.T, u InputUnion) {
				assert.Nil(t, u.String)
				require.NotNil(t, u.Items)
				assert.Len(t, u.Items, 0)
			},
		},
		{
			name: "array with function_call_output",
			input: `[{
				"type": "function_call_output",
				"call_id": "call_123",
				"name": "get_weather",
				"output": "{\"temperature\": 72}"
			}]`,
			validate: func(t *testing.T, u InputUnion) {
				assert.Nil(t, u.String)
				require.Len(t, u.Items, 1)
				assert.Equal(t, "function_call_output", u.Items[0].Type)
				assert.Equal(t, "call_123", u.Items[0].CallID)
				assert.Equal(t, "get_weather", u.Items[0].Name)
				assert.Equal(t, `{"temperature": 72}`, u.Items[0].Output)
			},
		},
		{
			name:        "invalid JSON",
			input:       `{invalid json}`,
			expectError: true,
		},
		{
			name:        "invalid type - number",
			input:       `123`,
			expectError: true,
		},
		{
			name:        "invalid type - object",
			input:       `{"key": "value"}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u InputUnion
			err := json.Unmarshal([]byte(tt.input), &u)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, u)
			}
		})
	}
}

func TestInputUnion_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    InputUnion
		expected string
	}{
		{
			name: "string value",
			input: InputUnion{
				String: stringPtr("hello world"),
			},
			expected: `"hello world"`,
		},
		{
			name: "empty string",
			input: InputUnion{
				String: stringPtr(""),
			},
			expected: `""`,
		},
		{
			name: "array value",
			input: InputUnion{
				Items: []InputItem{
					{Type: "message", Role: "user"},
				},
			},
			expected: `[{"type":"message","role":"user"}]`,
		},
		{
			name: "empty array",
			input: InputUnion{
				Items: []InputItem{},
			},
			expected: `[]`,
		},
		{
			name:     "nil values",
			input:    InputUnion{},
			expected: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestInputUnion_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input InputUnion
	}{
		{
			name: "string",
			input: InputUnion{
				String: stringPtr("test message"),
			},
		},
		{
			name: "array with messages",
			input: InputUnion{
				Items: []InputItem{
					{Type: "message", Role: "user", Content: json.RawMessage(`"hello"`)},
					{Type: "message", Role: "assistant", Content: json.RawMessage(`"hi"`)},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.input)
			require.NoError(t, err)

			// Unmarshal
			var result InputUnion
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			// Verify equivalence
			if tt.input.String != nil {
				require.NotNil(t, result.String)
				assert.Equal(t, *tt.input.String, *result.String)
			}
			if tt.input.Items != nil {
				require.NotNil(t, result.Items)
				assert.Len(t, result.Items, len(tt.input.Items))
			}
		})
	}
}

func TestResponseRequest_NormalizeInput(t *testing.T) {
	tests := []struct {
		name     string
		request  ResponseRequest
		validate func(t *testing.T, msgs []Message)
	}{
		{
			name: "string input creates user message",
			request: ResponseRequest{
				Input: InputUnion{
					String: stringPtr("hello world"),
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				assert.Equal(t, "user", msgs[0].Role)
				require.Len(t, msgs[0].Content, 1)
				assert.Equal(t, "input_text", msgs[0].Content[0].Type)
				assert.Equal(t, "hello world", msgs[0].Content[0].Text)
			},
		},
		{
			name: "message with string content",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type:    "message",
							Role:    "user",
							Content: json.RawMessage(`"what is the weather?"`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				assert.Equal(t, "user", msgs[0].Role)
				require.Len(t, msgs[0].Content, 1)
				assert.Equal(t, "input_text", msgs[0].Content[0].Type)
				assert.Equal(t, "what is the weather?", msgs[0].Content[0].Text)
			},
		},
		{
			name: "assistant message with string content uses output_text",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type:    "message",
							Role:    "assistant",
							Content: json.RawMessage(`"The weather is sunny"`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				assert.Equal(t, "assistant", msgs[0].Role)
				require.Len(t, msgs[0].Content, 1)
				assert.Equal(t, "output_text", msgs[0].Content[0].Type)
				assert.Equal(t, "The weather is sunny", msgs[0].Content[0].Text)
			},
		},
		{
			name: "message with content blocks array",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type: "message",
							Role: "user",
							Content: json.RawMessage(`[
								{"type": "input_text", "text": "hello"},
								{"type": "input_text", "text": "world"}
							]`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				assert.Equal(t, "user", msgs[0].Role)
				require.Len(t, msgs[0].Content, 2)
				assert.Equal(t, "input_text", msgs[0].Content[0].Type)
				assert.Equal(t, "hello", msgs[0].Content[0].Text)
				assert.Equal(t, "input_text", msgs[0].Content[1].Type)
				assert.Equal(t, "world", msgs[0].Content[1].Text)
			},
		},
		{
			name: "message preserves structured multimodal content blocks",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type: "message",
							Role: "user",
							Content: json.RawMessage(`[
								{"type": "input_text", "text": "hello"},
								{"type": "input_image", "image_url": "https://example.com/image.png", "detail": "high"},
								{"type": "input_file", "filename": "notes.txt", "file_data": "Zm9vYmFy"}
							]`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				require.Len(t, msgs[0].Content, 3)
				assert.Equal(t, "input_text", msgs[0].Content[0].Type)
				assert.Equal(t, "hello", msgs[0].Content[0].Text)
				assert.Equal(t, "input_image", msgs[0].Content[1].Type)
				assert.Equal(t, "https://example.com/image.png", msgs[0].Content[1].ImageURL)
				assert.Equal(t, "high", msgs[0].Content[1].Detail)
				assert.Equal(t, "input_file", msgs[0].Content[2].Type)
				assert.Equal(t, "notes.txt", msgs[0].Content[2].Filename)
				assert.Equal(t, "Zm9vYmFy", msgs[0].Content[2].FileData)
			},
		},
		{
			name: "message with tool_use blocks",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type: "message",
							Role: "assistant",
							Content: json.RawMessage(`[
								{
									"type": "tool_use",
									"id": "call_123",
									"name": "get_weather",
									"input": {"location": "San Francisco"}
								}
							]`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				assert.Equal(t, "assistant", msgs[0].Role)
				assert.Len(t, msgs[0].Content, 0)
				require.Len(t, msgs[0].ToolCalls, 1)
				assert.Equal(t, "call_123", msgs[0].ToolCalls[0].ID)
				assert.Equal(t, "get_weather", msgs[0].ToolCalls[0].Name)
				assert.JSONEq(t, `{"location":"San Francisco"}`, msgs[0].ToolCalls[0].Arguments)
			},
		},
		{
			name: "message with mixed text and tool_use",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type: "message",
							Role: "assistant",
							Content: json.RawMessage(`[
								{
									"type": "output_text",
									"text": "Let me check the weather"
								},
								{
									"type": "tool_use",
									"id": "call_456",
									"name": "get_weather",
									"input": {"location": "Boston"}
								}
							]`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				assert.Equal(t, "assistant", msgs[0].Role)
				require.Len(t, msgs[0].Content, 1)
				assert.Equal(t, "output_text", msgs[0].Content[0].Type)
				assert.Equal(t, "Let me check the weather", msgs[0].Content[0].Text)
				require.Len(t, msgs[0].ToolCalls, 1)
				assert.Equal(t, "call_456", msgs[0].ToolCalls[0].ID)
			},
		},
		{
			name: "assistant message preserves refusal content blocks",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type: "message",
							Role: "assistant",
							Content: json.RawMessage(`[
								{"type": "output_text", "text": "I can't comply."},
								{"type": "refusal", "refusal": "This request is disallowed."}
							]`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				require.Len(t, msgs[0].Content, 2)
				assert.Equal(t, "output_text", msgs[0].Content[0].Type)
				assert.Equal(t, "I can't comply.", msgs[0].Content[0].Text)
				assert.Equal(t, "refusal", msgs[0].Content[1].Type)
				assert.Equal(t, "This request is disallowed.", msgs[0].Content[1].Refusal)
			},
		},
		{
			name: "multiple tool_use blocks",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type: "message",
							Role: "assistant",
							Content: json.RawMessage(`[
								{
									"type": "tool_use",
									"id": "call_1",
									"name": "get_weather",
									"input": {"location": "NYC"}
								},
								{
									"type": "tool_use",
									"id": "call_2",
									"name": "get_time",
									"input": {"timezone": "EST"}
								}
							]`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				require.Len(t, msgs[0].ToolCalls, 2)
				assert.Equal(t, "call_1", msgs[0].ToolCalls[0].ID)
				assert.Equal(t, "get_weather", msgs[0].ToolCalls[0].Name)
				assert.Equal(t, "call_2", msgs[0].ToolCalls[1].ID)
				assert.Equal(t, "get_time", msgs[0].ToolCalls[1].Name)
			},
		},
		{
			name: "function_call_output item",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type:   "function_call_output",
							CallID: "call_123",
							Name:   "get_weather",
							Output: `{"temperature": 72, "condition": "sunny"}`,
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				assert.Equal(t, "tool", msgs[0].Role)
				assert.Equal(t, "call_123", msgs[0].CallID)
				assert.Equal(t, "get_weather", msgs[0].Name)
				require.Len(t, msgs[0].Content, 1)
				assert.Equal(t, "input_text", msgs[0].Content[0].Type)
				assert.Equal(t, `{"temperature": 72, "condition": "sunny"}`, msgs[0].Content[0].Text)
			},
		},
		{
			name: "function_call_output item preserves array content",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type:   "function_call_output",
							CallID: "call_456",
							Name:   "render_report",
							Output: []map[string]any{
								{"type": "input_text", "text": "See attached report."},
								{"type": "input_file", "filename": "report.txt", "file_data": "cmVwb3J0"},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				assert.Equal(t, "tool", msgs[0].Role)
				require.Len(t, msgs[0].Content, 2)
				assert.Equal(t, "input_text", msgs[0].Content[0].Type)
				assert.Equal(t, "See attached report.", msgs[0].Content[0].Text)
				assert.Equal(t, "input_file", msgs[0].Content[1].Type)
				assert.Equal(t, "report.txt", msgs[0].Content[1].Filename)
			},
		},
		{
			name: "multiple messages in conversation",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type:    "message",
							Role:    "user",
							Content: json.RawMessage(`"what is 2+2?"`),
						},
						{
							Type:    "message",
							Role:    "assistant",
							Content: json.RawMessage(`"The answer is 4"`),
						},
						{
							Type:    "message",
							Role:    "user",
							Content: json.RawMessage(`"thanks!"`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 3)
				assert.Equal(t, "user", msgs[0].Role)
				assert.Equal(t, "assistant", msgs[1].Role)
				assert.Equal(t, "user", msgs[2].Role)
			},
		},
		{
			name: "complete tool calling flow",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type:    "message",
							Role:    "user",
							Content: json.RawMessage(`"what is the weather?"`),
						},
						{
							Type: "message",
							Role: "assistant",
							Content: json.RawMessage(`[
								{
									"type": "tool_use",
									"id": "call_abc",
									"name": "get_weather",
									"input": {"location": "Seattle"}
								}
							]`),
						},
						{
							Type:   "function_call_output",
							CallID: "call_abc",
							Name:   "get_weather",
							Output: `{"temp": 55, "condition": "rainy"}`,
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 3)
				assert.Equal(t, "user", msgs[0].Role)
				assert.Equal(t, "assistant", msgs[1].Role)
				require.Len(t, msgs[1].ToolCalls, 1)
				assert.Equal(t, "tool", msgs[2].Role)
				assert.Equal(t, "call_abc", msgs[2].CallID)
			},
		},
		{
			name: "message without type defaults to message",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Role:    "user",
							Content: json.RawMessage(`"hello"`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				assert.Equal(t, "user", msgs[0].Role)
			},
		},
		{
			name: "message with nil content",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type:    "message",
							Role:    "user",
							Content: nil,
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				assert.Equal(t, "user", msgs[0].Role)
				assert.Len(t, msgs[0].Content, 0)
			},
		},
		{
			name: "tool_use with empty input",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type: "message",
							Role: "assistant",
							Content: json.RawMessage(`[
								{
									"type": "tool_use",
									"id": "call_xyz",
									"name": "no_args_function",
									"input": {}
								}
							]`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				require.Len(t, msgs[0].ToolCalls, 1)
				assert.Equal(t, "call_xyz", msgs[0].ToolCalls[0].ID)
				assert.JSONEq(t, `{}`, msgs[0].ToolCalls[0].Arguments)
			},
		},
		{
			name: "function_call item converts to assistant message with tool calls",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type:      "function_call",
							CallID:    "call_123",
							Name:      "get_weather",
							Arguments: `{"location":"Seattle"}`,
						},
						{
							Type:   "function_call_output",
							CallID: "call_123",
							Name:   "get_weather",
							Output: `{"temp": 55}`,
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 2)
				assert.Equal(t, "assistant", msgs[0].Role)
				require.Len(t, msgs[0].ToolCalls, 1)
				assert.Equal(t, "call_123", msgs[0].ToolCalls[0].ID)
				assert.Equal(t, "get_weather", msgs[0].ToolCalls[0].Name)
				assert.Equal(t, `{"location":"Seattle"}`, msgs[0].ToolCalls[0].Arguments)
				assert.Equal(t, "tool", msgs[1].Role)
				assert.Equal(t, "call_123", msgs[1].CallID)
			},
		},
		{
			name: "sequential function_call items produce separate assistant messages",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type:      "function_call",
							CallID:    "call_1",
							Name:      "tool_a",
							Arguments: `{"x":1}`,
						},
						{
							Type:   "function_call_output",
							CallID: "call_1",
							Output: `"result_a"`,
						},
						{
							Type:      "function_call",
							CallID:    "call_2",
							Name:      "tool_b",
							Arguments: `{"y":2}`,
						},
						{
							Type:   "function_call_output",
							CallID: "call_2",
							Output: `"result_b"`,
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 4)
				assert.Equal(t, "assistant", msgs[0].Role)
				assert.Equal(t, "call_1", msgs[0].ToolCalls[0].ID)
				assert.Equal(t, "tool", msgs[1].Role)
				assert.Equal(t, "call_1", msgs[1].CallID)
				assert.Equal(t, "assistant", msgs[2].Role)
				assert.Equal(t, "call_2", msgs[2].ToolCalls[0].ID)
				assert.Equal(t, "tool", msgs[3].Role)
				assert.Equal(t, "call_2", msgs[3].CallID)
			},
		},
		{
			name: "parallel function_call items are merged into one assistant message",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type:      "function_call",
							CallID:    "call_1",
							Name:      "tool_a",
							Arguments: `{"x":1}`,
						},
						{
							Type:      "function_call",
							CallID:    "call_2",
							Name:      "tool_b",
							Arguments: `{"y":2}`,
						},
						{
							Type:   "function_call_output",
							CallID: "call_1",
							Output: `"result_a"`,
						},
						{
							Type:   "function_call_output",
							CallID: "call_2",
							Output: `"result_b"`,
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 3)
				assert.Equal(t, "assistant", msgs[0].Role)
				require.Len(t, msgs[0].ToolCalls, 2)
				assert.Equal(t, "call_1", msgs[0].ToolCalls[0].ID)
				assert.Equal(t, "call_2", msgs[0].ToolCalls[1].ID)
				assert.Equal(t, "tool", msgs[1].Role)
				assert.Equal(t, "call_1", msgs[1].CallID)
				assert.Equal(t, "tool", msgs[2].Role)
				assert.Equal(t, "call_2", msgs[2].CallID)
			},
		},
		{
			name: "content blocks with unknown types ignored",
			request: ResponseRequest{
				Input: InputUnion{
					Items: []InputItem{
						{
							Type: "message",
							Role: "user",
							Content: json.RawMessage(`[
								{"type": "input_text", "text": "visible"},
								{"type": "unknown_type", "data": "ignored"},
								{"type": "input_text", "text": "also visible"}
							]`),
						},
					},
				},
			},
			validate: func(t *testing.T, msgs []Message) {
				require.Len(t, msgs, 1)
				require.Len(t, msgs[0].Content, 2)
				assert.Equal(t, "visible", msgs[0].Content[0].Text)
				assert.Equal(t, "also visible", msgs[0].Content[1].Text)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := tt.request.NormalizeInput()
			if tt.validate != nil {
				tt.validate(t, msgs)
			}
		})
	}
}

func TestResponseRequest_Validate(t *testing.T) {
	tests := []struct {
		name        string
		request     *ResponseRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request with string input",
			request: &ResponseRequest{
				Model: "gpt-4",
				Input: InputUnion{
					String: stringPtr("hello"),
				},
			},
			expectError: false,
		},
		{
			name: "valid request with array input",
			request: &ResponseRequest{
				Model: "gpt-4",
				Input: InputUnion{
					Items: []InputItem{
						{Type: "message", Role: "user", Content: json.RawMessage(`"hello"`)},
					},
				},
			},
			expectError: false,
		},
		{
			name:        "nil request",
			request:     nil,
			expectError: true,
			errorMsg:    "request is nil",
		},
		{
			name: "missing model",
			request: &ResponseRequest{
				Model: "",
				Input: InputUnion{
					String: stringPtr("hello"),
				},
			},
			expectError: true,
			errorMsg:    "model is required",
		},
		{
			name: "missing input",
			request: &ResponseRequest{
				Model: "gpt-4",
				Input: InputUnion{},
			},
			expectError: true,
			errorMsg:    "input is required",
		},
		{
			name: "empty string input is invalid",
			request: &ResponseRequest{
				Model: "gpt-4",
				Input: InputUnion{
					String: stringPtr(""),
				},
			},
			expectError: false, // Empty string is technically valid
		},
		{
			name: "empty array input is invalid",
			request: &ResponseRequest{
				Model: "gpt-4",
				Input: InputUnion{
					Items: []InputItem{},
				},
			},
			expectError: true,
			errorMsg:    "input is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestGetStringField(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		key      string
		expected string
	}{
		{
			name: "existing string field",
			input: map[string]interface{}{
				"name": "value",
			},
			key:      "name",
			expected: "value",
		},
		{
			name: "missing field",
			input: map[string]interface{}{
				"other": "value",
			},
			key:      "name",
			expected: "",
		},
		{
			name: "wrong type - int",
			input: map[string]interface{}{
				"name": 123,
			},
			key:      "name",
			expected: "",
		},
		{
			name: "wrong type - bool",
			input: map[string]interface{}{
				"name": true,
			},
			key:      "name",
			expected: "",
		},
		{
			name: "wrong type - object",
			input: map[string]interface{}{
				"name": map[string]string{"nested": "value"},
			},
			key:      "name",
			expected: "",
		},
		{
			name: "empty string value",
			input: map[string]interface{}{
				"name": "",
			},
			key:      "name",
			expected: "",
		},
		{
			name:     "nil map",
			input:    nil,
			key:      "name",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStringField(tt.input, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInputItem_ComplexContent(t *testing.T) {
	tests := []struct {
		name     string
		itemJSON string
		validate func(t *testing.T, item InputItem)
	}{
		{
			name: "content with nested objects",
			itemJSON: `{
				"type": "message",
				"role": "assistant",
				"content": [{
					"type": "tool_use",
					"id": "call_complex",
					"name": "search",
					"input": {
						"query": "test",
						"filters": {
							"category": "docs",
							"date": "2024-01-01"
						},
						"limit": 10
					}
				}]
			}`,
			validate: func(t *testing.T, item InputItem) {
				assert.Equal(t, "message", item.Type)
				assert.Equal(t, "assistant", item.Role)
				assert.NotNil(t, item.Content)
			},
		},
		{
			name: "content with array in input",
			itemJSON: `{
				"type": "message",
				"role": "assistant",
				"content": [{
					"type": "tool_use",
					"id": "call_arr",
					"name": "batch_process",
					"input": {
						"items": ["a", "b", "c"]
					}
				}]
			}`,
			validate: func(t *testing.T, item InputItem) {
				assert.Equal(t, "message", item.Type)
				assert.NotNil(t, item.Content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var item InputItem
			err := json.Unmarshal([]byte(tt.itemJSON), &item)
			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, item)
			}
		})
	}
}

func TestResponseRequest_CompleteWorkflow(t *testing.T) {
	requestJSON := `{
		"model": "gpt-4",
		"input": [{
			"type": "message",
			"role": "user",
			"content": "What's the weather in NYC and LA?"
		}, {
			"type": "message",
			"role": "assistant",
			"content": [{
				"type": "output_text",
				"text": "Let me check both locations for you."
			}, {
				"type": "tool_use",
				"id": "call_1",
				"name": "get_weather",
				"input": {"location": "New York City"}
			}, {
				"type": "tool_use",
				"id": "call_2",
				"name": "get_weather",
				"input": {"location": "Los Angeles"}
			}]
		}, {
			"type": "function_call_output",
			"call_id": "call_1",
			"name": "get_weather",
			"output": "{\"temp\": 45, \"condition\": \"cloudy\"}"
		}, {
			"type": "function_call_output",
			"call_id": "call_2",
			"name": "get_weather",
			"output": "{\"temp\": 72, \"condition\": \"sunny\"}"
		}],
		"stream": true,
		"temperature": 0.7
	}`

	var req ResponseRequest
	err := json.Unmarshal([]byte(requestJSON), &req)
	require.NoError(t, err)

	// Validate
	err = req.Validate()
	require.NoError(t, err)

	// Normalize
	msgs := req.NormalizeInput()
	require.Len(t, msgs, 4)

	// Check user message
	assert.Equal(t, "user", msgs[0].Role)
	assert.Len(t, msgs[0].Content, 1)

	// Check assistant message with tool calls
	assert.Equal(t, "assistant", msgs[1].Role)
	assert.Len(t, msgs[1].Content, 1)
	assert.Len(t, msgs[1].ToolCalls, 2)
	assert.Equal(t, "call_1", msgs[1].ToolCalls[0].ID)
	assert.Equal(t, "call_2", msgs[1].ToolCalls[1].ID)

	// Check tool responses
	assert.Equal(t, "tool", msgs[2].Role)
	assert.Equal(t, "call_1", msgs[2].CallID)
	assert.Equal(t, "tool", msgs[3].Role)
	assert.Equal(t, "call_2", msgs[3].CallID)
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}
