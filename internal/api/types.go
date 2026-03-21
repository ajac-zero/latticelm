package api

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ============================================================
// Request Types (CreateResponseBody)
// ============================================================

// ResponseRequest models the OpenResponses CreateResponseBody.
type ResponseRequest struct {
	Model              string            `json:"model"`
	Input              InputUnion        `json:"input"`
	Instructions       *string           `json:"instructions,omitempty"`
	MaxOutputTokens    *int              `json:"max_output_tokens,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Stream             bool              `json:"stream,omitempty"`
	PreviousResponseID *string           `json:"previous_response_id,omitempty"`
	Temperature        *float64          `json:"temperature,omitempty"`
	TopP               *float64          `json:"top_p,omitempty"`
	FrequencyPenalty   *float64          `json:"frequency_penalty,omitempty"`
	PresencePenalty    *float64          `json:"presence_penalty,omitempty"`
	TopLogprobs        *int              `json:"top_logprobs,omitempty"`
	Truncation         *string           `json:"truncation,omitempty"`
	ToolChoice         json.RawMessage   `json:"tool_choice,omitempty"`
	Tools              json.RawMessage   `json:"tools,omitempty"`
	ParallelToolCalls  *bool             `json:"parallel_tool_calls,omitempty"`
	Store              *bool             `json:"store,omitempty"`
	Text               json.RawMessage   `json:"text,omitempty"`
	Reasoning          json.RawMessage   `json:"reasoning,omitempty"`
	Include            []string          `json:"include,omitempty"`
	ServiceTier        *string           `json:"service_tier,omitempty"`
	Background         *bool             `json:"background,omitempty"`
	StreamOptions      json.RawMessage   `json:"stream_options,omitempty"`
	MaxToolCalls       *int              `json:"max_tool_calls,omitempty"`

	// Non-spec extension: allows client to select a specific provider.
	Provider string `json:"provider,omitempty"`
}

// InputUnion handles the polymorphic "input" field: string or []InputItem.
type InputUnion struct {
	String *string
	Items  []InputItem
}

func (u *InputUnion) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		u.String = &s
		return nil
	}
	var items []InputItem
	if err := json.Unmarshal(data, &items); err == nil {
		u.Items = items
		return nil
	}
	return fmt.Errorf("input must be a string or array of items")
}

func (u InputUnion) MarshalJSON() ([]byte, error) {
	if u.String != nil {
		return json.Marshal(*u.String)
	}
	if u.Items != nil {
		return json.Marshal(u.Items)
	}
	return []byte("null"), nil
}

// InputItem is a discriminated union on "type".
// Valid types: message, item_reference, function_call, function_call_output, reasoning.
type InputItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	ID        string          `json:"id,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Output    string          `json:"output,omitempty"`
	Status    string          `json:"status,omitempty"`
}

// ============================================================
// Internal Types (providers + conversation store)
// ============================================================

// Message is the normalized internal message representation.
type Message struct {
	Role      string         `json:"role"`
	Content   []ContentBlock `json:"content"`
	CallID    string         `json:"call_id,omitempty"`    // for tool messages
	Name      string         `json:"name,omitempty"`       // for tool messages
	ToolCalls []ToolCall     `json:"tool_calls,omitempty"` // for assistant messages
}

// ContentBlock is a typed content element.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// NormalizeInput converts the request Input into messages for providers.
// Does NOT include instructions (the server prepends those separately).
func (r *ResponseRequest) NormalizeInput() []Message {
	if r.Input.String != nil {
		return []Message{{
			Role:    "user",
			Content: []ContentBlock{{Type: "input_text", Text: *r.Input.String}},
		}}
	}

	var msgs []Message
	for _, item := range r.Input.Items {
		switch item.Type {
		case "message", "":
			msg := Message{Role: item.Role}
			if item.Content != nil {
				var s string
				if err := json.Unmarshal(item.Content, &s); err == nil {
					contentType := "input_text"
					if item.Role == "assistant" {
						contentType = "output_text"
					}
					msg.Content = []ContentBlock{{Type: contentType, Text: s}}
				} else {
					// Content is an array of blocks - parse them
					var rawBlocks []map[string]interface{}
					if err := json.Unmarshal(item.Content, &rawBlocks); err == nil {
						// Extract content blocks and tool calls
						for _, block := range rawBlocks {
							blockType, _ := block["type"].(string)

							if blockType == "tool_use" {
								// Extract tool call information
								toolCall := ToolCall{
									ID:   getStringField(block, "id"),
									Name: getStringField(block, "name"),
								}
								// input field contains the arguments as a map
								if input, ok := block["input"].(map[string]interface{}); ok {
									if inputJSON, err := json.Marshal(input); err == nil {
										toolCall.Arguments = string(inputJSON)
									}
								}
								msg.ToolCalls = append(msg.ToolCalls, toolCall)
							} else if blockType == "output_text" || blockType == "input_text" {
								// Regular text content block
								msg.Content = append(msg.Content, ContentBlock{
									Type: blockType,
									Text: getStringField(block, "text"),
								})
							}
						}
					}
				}
			}
			msgs = append(msgs, msg)
		case "function_call":
			// function_call items represent the assistant's tool invocation.
			// Consecutive function_call items (parallel tool calls) must be merged
			// into a single assistant message, since OpenAI requires all tool_call_ids
			// in one assistant turn to each have a corresponding tool response.
			tc := ToolCall{
				ID:        item.CallID,
				Name:      item.Name,
				Arguments: item.Arguments,
			}
			if n := len(msgs); n > 0 && msgs[n-1].Role == "assistant" && len(msgs[n-1].ToolCalls) > 0 && len(msgs[n-1].Content) == 0 {
				msgs[n-1].ToolCalls = append(msgs[n-1].ToolCalls, tc)
			} else {
				msgs = append(msgs, Message{
					Role:      "assistant",
					ToolCalls: []ToolCall{tc},
				})
			}
		case "function_call_output":
			msgs = append(msgs, Message{
				Role:    "tool",
				Content: []ContentBlock{{Type: "input_text", Text: item.Output}},
				CallID:  item.CallID,
				Name:    item.Name,
			})
		}
	}
	return msgs
}

// ============================================================
// Response Types (ResponseResource)
// ============================================================

// Response is the spec-compliant ResponseResource.
type Response struct {
	ID                 string             `json:"id"`
	Object             string             `json:"object"`
	CreatedAt          int64              `json:"created_at"`
	CompletedAt        *int64             `json:"completed_at"`
	Status             string             `json:"status"`
	IncompleteDetails  *IncompleteDetails `json:"incomplete_details"`
	Model              string             `json:"model"`
	PreviousResponseID *string            `json:"previous_response_id"`
	Instructions       *string            `json:"instructions"`
	Output             []OutputItem       `json:"output"`
	Error              *ResponseError     `json:"error"`
	Tools              json.RawMessage    `json:"tools"`
	ToolChoice         json.RawMessage    `json:"tool_choice"`
	Truncation         string             `json:"truncation"`
	ParallelToolCalls  bool               `json:"parallel_tool_calls"`
	Text               json.RawMessage    `json:"text"`
	TopP               float64            `json:"top_p"`
	PresencePenalty    float64            `json:"presence_penalty"`
	FrequencyPenalty   float64            `json:"frequency_penalty"`
	TopLogprobs        int                `json:"top_logprobs"`
	Temperature        float64            `json:"temperature"`
	Reasoning          json.RawMessage    `json:"reasoning"`
	Usage              *Usage             `json:"usage"`
	MaxOutputTokens    *int               `json:"max_output_tokens"`
	MaxToolCalls       *int               `json:"max_tool_calls"`
	Store              bool               `json:"store"`
	Background         bool               `json:"background"`
	ServiceTier        string             `json:"service_tier"`
	Metadata           map[string]string  `json:"metadata"`
	SafetyIdentifier   *string            `json:"safety_identifier"`
	PromptCacheKey     *string            `json:"prompt_cache_key"`

	// Non-spec extension
	Provider string `json:"provider,omitempty"`
}

// OutputItem represents a typed item in the response output.
type OutputItem struct {
	ID        string        `json:"id"`
	Type      string        `json:"type"`
	Status    string        `json:"status"`
	Role      string        `json:"role,omitempty"`
	Content   []ContentPart `json:"content,omitempty"`
	CallID    string        `json:"call_id,omitempty"`   // for function_call
	Name      string        `json:"name,omitempty"`      // for function_call
	Arguments string        `json:"arguments,omitempty"` // for function_call
}

// ContentPart is a content block within an output item.
type ContentPart struct {
	Type        string       `json:"type"`
	Text        string       `json:"text"`
	Annotations []Annotation `json:"annotations"`
}

// Annotation on output text content.
type Annotation struct {
	Type string `json:"type"`
}

// IncompleteDetails explains why a response is incomplete.
type IncompleteDetails struct {
	Reason string `json:"reason"`
}

// ResponseError describes an error in the response.
type ResponseError struct {
	Type    string  `json:"type"`
	Message string  `json:"message"`
	Code    *string `json:"code"`
}

// ============================================================
// Usage Types
// ============================================================

// Usage captures token accounting with sub-details.
type Usage struct {
	InputTokens         int                 `json:"input_tokens"`
	OutputTokens        int                 `json:"output_tokens"`
	TotalTokens         int                 `json:"total_tokens"`
	InputTokensDetails  InputTokensDetails  `json:"input_tokens_details"`
	OutputTokensDetails OutputTokensDetails `json:"output_tokens_details"`
}

// InputTokensDetails breaks down input token usage.
type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// OutputTokensDetails breaks down output token usage.
type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// ============================================================
// Streaming Types
// ============================================================

// StreamEvent represents a single SSE event in the streaming response.
// Fields are selectively populated based on the event Type.
type StreamEvent struct {
	Type           string       `json:"type"`
	SequenceNumber int          `json:"sequence_number"`
	Response       *Response    `json:"response,omitempty"`
	OutputIndex    *int         `json:"output_index,omitempty"`
	Item           *OutputItem  `json:"item,omitempty"`
	ItemID         string       `json:"item_id,omitempty"`
	ContentIndex   *int         `json:"content_index,omitempty"`
	Part           *ContentPart `json:"part,omitempty"`
	Delta          string       `json:"delta,omitempty"`
	Text           string       `json:"text,omitempty"`
	Arguments      string       `json:"arguments,omitempty"` // for function_call_arguments.done
}

// ============================================================
// Provider Result Types (internal, not exposed via HTTP)
// ============================================================

// ProviderResult is returned by Provider.Generate.
type ProviderResult struct {
	ID        string
	Model     string
	Text      string
	Usage     Usage
	ToolCalls []ToolCall
}

// ProviderStreamDelta is sent through the stream channel.
type ProviderStreamDelta struct {
	ID            string
	Model         string
	Text          string
	Done          bool
	Usage         *Usage
	ToolCallDelta *ToolCallDelta
}

// ToolCall represents a function call from the model.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON string
}

// ToolCallDelta represents a streaming chunk of a tool call.
type ToolCallDelta struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

// ============================================================
// Models Endpoint Types
// ============================================================

// ModelInfo describes a single model available through the gateway.
type ModelInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

// ModelsResponse is returned by GET /v1/models.
type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

// ============================================================
// Validation
// ============================================================

// Validate performs basic structural validation.
func (r *ResponseRequest) Validate() error {
	if r == nil {
		return errors.New("request is nil")
	}
	if r.Model == "" {
		return errors.New("model is required")
	}
	if r.Input.String == nil && len(r.Input.Items) == 0 {
		return errors.New("input is required")
	}
	return nil
}

// getStringField is a helper to safely extract string fields from a map
func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}
