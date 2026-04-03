package api

import (
	"encoding/json"
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

// ParsedToolChoice is the decoded representation of the tool_choice field.
type ParsedToolChoice struct {
	Mode             string
	RequiredToolName string
	AllowedTools     map[string]struct{}
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
	Type             string          `json:"type"`
	Role             string          `json:"role,omitempty"`
	Content          json.RawMessage `json:"content,omitempty"`
	ID               string          `json:"id,omitempty"`
	CallID           string          `json:"call_id,omitempty"`
	Name             string          `json:"name,omitempty"`
	Arguments        string          `json:"arguments,omitempty"`
	Output           any             `json:"output,omitempty"`
	Status           string          `json:"status,omitempty"`
	Summary          json.RawMessage `json:"summary,omitempty"`
	EncryptedContent string          `json:"encrypted_content,omitempty"`
}

// ReasoningSummaryContent represents a reasoning summary text block.
type ReasoningSummaryContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
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
	Param   *string `json:"param,omitempty"`
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
