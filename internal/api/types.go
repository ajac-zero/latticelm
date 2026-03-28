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
	Type             string `json:"type"`
	Text             string `json:"text,omitempty"`
	Refusal          string `json:"refusal,omitempty"`
	ImageURL         string `json:"image_url,omitempty"`
	Detail           string `json:"detail,omitempty"`
	FileData         string `json:"file_data,omitempty"`
	FileURL          string `json:"file_url,omitempty"`
	Filename         string `json:"filename,omitempty"`
	VideoURL         string `json:"video_url,omitempty"`
	EncryptedContent string `json:"encrypted_content,omitempty"`
	Signature        string `json:"signature,omitempty"`
	Data             string `json:"data,omitempty"`
}

func (b ContentBlock) TextValue() (string, bool) {
	switch b.Type {
	case "text", "input_text", "output_text":
		return b.Text, true
	case "refusal":
		return b.Refusal, true
	default:
		return "", false
	}
}

// NormalizeInput converts the request Input into messages for providers.
// Does NOT include instructions (the server prepends those separately).
// Returns the normalized messages and any item_reference IDs that need resolution.
func (r *ResponseRequest) NormalizeInput() ([]Message, []string) {
	if r.Input.String != nil {
		return []Message{{
			Role:    "user",
			Content: []ContentBlock{textContentBlock("user", *r.Input.String)},
		}}, nil
	}

	var msgs []Message
	var itemRefs []string
	for _, item := range r.Input.Items {
		switch item.Type {
		case "message", "":
			msg := Message{Role: item.Role}
			if item.Content != nil {
				msg.Content, msg.ToolCalls = normalizeMessageContent(item.Role, item.Content)
			}
			msgs = append(msgs, msg)
		case "item_reference":
			// item_reference items are IDs referencing previous output items.
			// These need to be resolved by the server using previous_response_id or stored context.
			if item.ID != "" {
				itemRefs = append(itemRefs, item.ID)
			}
		case "reasoning":
			// reasoning items represent prior model reasoning content.
			msg := Message{Role: "assistant"}
			var content []ContentBlock
			if item.Content != nil {
				content, _ = normalizeMessageContent("assistant", item.Content)
			}
			// Reasoning summaries are normalized as assistant text so providers
			// that only support text assistant content can still accept them.
			if item.Summary != nil {
				var summaries []ReasoningSummaryContent
				if err := json.Unmarshal(item.Summary, &summaries); err == nil {
					for _, s := range summaries {
						if s.Text != "" {
							content = append(content, ContentBlock{
								Type: "output_text",
								Text: s.Text,
							})
						}
					}
				}
			}
			// Preserve encrypted reasoning for storage/retrieval, but providers
			// should ignore it because this gateway does not yet have a portable
			// way to forward opaque reasoning blobs downstream.
			if item.EncryptedContent != "" {
				content = append(content, ContentBlock{
					Type:             "encrypted_reasoning",
					EncryptedContent: item.EncryptedContent,
				})
			}
			if len(content) > 0 {
				msg.Content = content
				msgs = append(msgs, msg)
			}
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
				Content: normalizeToolOutputContent(item.Output),
				CallID:  item.CallID,
				Name:    item.Name,
			})
		}
	}
	return msgs, itemRefs
}

// ReplayState stores provider-native artifacts that can be rehydrated on
// same-provider follow-up requests.
type ReplayState struct {
	Provider           string       `json:"provider"`
	ProviderResponseID string       `json:"provider_response_id,omitempty"`
	Items              []ReplayItem `json:"items,omitempty"`
}

// ReplayItem maps a public output item ID back to the stored message and any
// provider-native assistant message that should replace it during replay.
type ReplayItem struct {
	ID             string   `json:"id"`
	OutputItemType string   `json:"output_item_type"`
	MessageIndex   int      `json:"message_index"`
	Message        *Message `json:"message,omitempty"`
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
// Provider Result Types (internal, not exposed via HTTP)
// ============================================================

// ProviderResult is returned by Provider.Generate.
type ProviderResult struct {
	ID            string
	Model         string
	Text          string
	Usage         Usage
	ToolCalls     []ToolCall
	ReplayMessage *Message
}

// ProviderStreamDelta is sent through the stream channel.
type ProviderStreamDelta struct {
	ID            string
	Model         string
	Text          string
	Done          bool
	Usage         *Usage
	ToolCallDelta *ToolCallDelta
	ReplayMessage *Message
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
	if r.Input.String == nil && len(r.Input.Items) == 0 && (r.PreviousResponseID == nil || *r.PreviousResponseID == "") {
		return errors.New("input is required")
	}
	if r.Truncation != nil && *r.Truncation != "auto" && *r.Truncation != "disabled" {
		return errors.New(`truncation must be "auto" or "disabled"`)
	}
	toolChoice, err := r.ParseToolChoice()
	if err != nil {
		return err
	}
	if toolChoice.Mode != "" && toolChoice.Mode != "auto" && toolChoice.Mode != "none" {
		toolNames, err := r.DeclaredToolNames()
		if err != nil {
			return err
		}
		if len(toolNames) == 0 {
			return errors.New("tool_choice requires tools to be declared")
		}
		switch toolChoice.Mode {
		case "required", "any":
		case "function":
			if _, ok := toolNames[toolChoice.RequiredToolName]; !ok {
				return fmt.Errorf("tool_choice references unknown tool %q", toolChoice.RequiredToolName)
			}
		case "allowed_tools":
			for name := range toolChoice.AllowedTools {
				if _, ok := toolNames[name]; !ok {
					return fmt.Errorf("allowed_tools references unknown tool %q", name)
				}
			}
		default:
			return fmt.Errorf("unsupported tool_choice mode %q", toolChoice.Mode)
		}
	}
	hasItemReference := false
	for _, item := range r.Input.Items {
		switch item.Type {
		case "", "message", "function_call", "function_call_output":
		case "item_reference":
			hasItemReference = true
			if item.ID == "" {
				return errors.New("item_reference id is required")
			}
		case "reasoning":
			if item.Content == nil && item.Summary == nil && item.EncryptedContent == "" {
				return errors.New("reasoning item must include content, summary, or encrypted_content")
			}
		default:
			return fmt.Errorf("unsupported input item type %q", item.Type)
		}
	}
	if hasItemReference && (r.PreviousResponseID == nil || *r.PreviousResponseID == "") {
		return errors.New("previous_response_id is required when using item_reference")
	}
	return nil
}

func (r *ResponseRequest) ParseToolChoice() (ParsedToolChoice, error) {
	parsed := ParsedToolChoice{Mode: "auto"}
	if r == nil || len(r.ToolChoice) == 0 {
		return parsed, nil
	}

	var choice interface{}
	if err := json.Unmarshal(r.ToolChoice, &choice); err != nil {
		return parsed, fmt.Errorf("invalid tool_choice: %w", err)
	}

	if str, ok := choice.(string); ok {
		switch str {
		case "auto", "none", "required", "any":
			parsed.Mode = str
			return parsed, nil
		default:
			return parsed, fmt.Errorf("invalid tool_choice value %q", str)
		}
	}

	obj, ok := choice.(map[string]interface{})
	if !ok {
		return parsed, errors.New("invalid tool_choice format")
	}

	switch objType, _ := obj["type"].(string); objType {
	case "function", "tool":
		name := parseToolChoiceName(obj)
		if name == "" {
			return parsed, errors.New("tool_choice function name is required")
		}
		parsed.Mode = "function"
		parsed.RequiredToolName = name
		return parsed, nil
	case "allowed_tools":
		tools, _ := obj["tools"].([]interface{})
		if len(tools) == 0 {
			return parsed, errors.New("allowed_tools requires at least one tool")
		}
		parsed.Mode = "allowed_tools"
		parsed.AllowedTools = make(map[string]struct{}, len(tools))
		for _, rawTool := range tools {
			tool, ok := rawTool.(map[string]interface{})
			if !ok {
				return parsed, errors.New("allowed_tools entries must be objects")
			}
			name := parseToolChoiceName(tool)
			if name == "" {
				return parsed, errors.New("allowed_tools entries require a tool name")
			}
			parsed.AllowedTools[name] = struct{}{}
		}
		return parsed, nil
	default:
		return parsed, errors.New("invalid tool_choice format")
	}
}

func (r *ResponseRequest) DeclaredToolNames() (map[string]struct{}, error) {
	names := map[string]struct{}{}
	if r == nil || len(r.Tools) == 0 {
		return names, nil
	}

	var toolDefs []map[string]interface{}
	if err := json.Unmarshal(r.Tools, &toolDefs); err != nil {
		return nil, fmt.Errorf("invalid tools: %w", err)
	}

	for _, toolDef := range toolDefs {
		name := parseToolChoiceName(toolDef)
		if name != "" {
			names[name] = struct{}{}
		}
	}

	return names, nil
}

func (r *ResponseRequest) InheritMissingContext(prev *ResponseRequest) {
	if r == nil || prev == nil {
		return
	}

	if r.Instructions == nil && prev.Instructions != nil {
		v := *prev.Instructions
		r.Instructions = &v
	}
	if len(r.Tools) == 0 && len(prev.Tools) > 0 {
		r.Tools = cloneRawMessage(prev.Tools)
	}
	if len(r.ToolChoice) == 0 && len(prev.ToolChoice) > 0 {
		r.ToolChoice = cloneRawMessage(prev.ToolChoice)
	}
	if r.MaxOutputTokens == nil && prev.MaxOutputTokens != nil {
		v := *prev.MaxOutputTokens
		r.MaxOutputTokens = &v
	}
	if r.Temperature == nil && prev.Temperature != nil {
		v := *prev.Temperature
		r.Temperature = &v
	}
	if r.TopP == nil && prev.TopP != nil {
		v := *prev.TopP
		r.TopP = &v
	}
	if r.FrequencyPenalty == nil && prev.FrequencyPenalty != nil {
		v := *prev.FrequencyPenalty
		r.FrequencyPenalty = &v
	}
	if r.PresencePenalty == nil && prev.PresencePenalty != nil {
		v := *prev.PresencePenalty
		r.PresencePenalty = &v
	}
	if r.TopLogprobs == nil && prev.TopLogprobs != nil {
		v := *prev.TopLogprobs
		r.TopLogprobs = &v
	}
	if r.Truncation == nil && prev.Truncation != nil {
		v := *prev.Truncation
		r.Truncation = &v
	}
	if r.ParallelToolCalls == nil && prev.ParallelToolCalls != nil {
		v := *prev.ParallelToolCalls
		r.ParallelToolCalls = &v
	}
	if r.Text == nil && prev.Text != nil {
		r.Text = cloneRawMessage(prev.Text)
	}
	if r.Reasoning == nil && prev.Reasoning != nil {
		r.Reasoning = cloneRawMessage(prev.Reasoning)
	}
	if len(r.Include) == 0 && len(prev.Include) > 0 {
		r.Include = append([]string(nil), prev.Include...)
	}
	if r.ServiceTier == nil && prev.ServiceTier != nil {
		v := *prev.ServiceTier
		r.ServiceTier = &v
	}
	if r.MaxToolCalls == nil && prev.MaxToolCalls != nil {
		v := *prev.MaxToolCalls
		r.MaxToolCalls = &v
	}
	if len(r.StreamOptions) == 0 && len(prev.StreamOptions) > 0 {
		r.StreamOptions = cloneRawMessage(prev.StreamOptions)
	}
	if r.Provider == "" {
		r.Provider = prev.Provider
	}
	if len(r.Metadata) == 0 && len(prev.Metadata) > 0 {
		r.Metadata = make(map[string]string, len(prev.Metadata))
		for k, v := range prev.Metadata {
			r.Metadata[k] = v
		}
	}
}

// getStringField is a helper to safely extract string fields from a map
func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func textContentBlock(role, text string) ContentBlock {
	contentType := "input_text"
	if role == "assistant" {
		contentType = "output_text"
	}
	return ContentBlock{
		Type: contentType,
		Text: text,
	}
}

func normalizeMessageContent(role string, raw json.RawMessage) ([]ContentBlock, []ToolCall) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []ContentBlock{textContentBlock(role, s)}, nil
	}

	var rawBlocks []map[string]interface{}
	if err := json.Unmarshal(raw, &rawBlocks); err != nil {
		return nil, nil
	}

	content := make([]ContentBlock, 0, len(rawBlocks))
	toolCalls := make([]ToolCall, 0)
	for _, block := range rawBlocks {
		blockType, _ := block["type"].(string)

		if blockType == "tool_use" {
			toolCall := ToolCall{
				ID:   getStringField(block, "id"),
				Name: getStringField(block, "name"),
			}
			if input, ok := block["input"].(map[string]interface{}); ok {
				if inputJSON, err := json.Marshal(input); err == nil {
					toolCall.Arguments = string(inputJSON)
				}
			}
			toolCalls = append(toolCalls, toolCall)
			continue
		}

		if normalized, ok := normalizeContentBlockMap(block); ok {
			content = append(content, normalized)
		}
	}

	return content, toolCalls
}

func normalizeToolOutputContent(raw any) []ContentBlock {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		return []ContentBlock{{Type: "input_text", Text: v}}
	case json.RawMessage:
		return normalizeToolOutputRaw(v)
	default:
		rawJSON, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		return normalizeToolOutputRaw(rawJSON)
	}
}

func normalizeToolOutputRaw(raw json.RawMessage) []ContentBlock {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []ContentBlock{{Type: "input_text", Text: s}}
	}

	var rawBlocks []map[string]interface{}
	if err := json.Unmarshal(raw, &rawBlocks); err != nil {
		return nil
	}

	content := make([]ContentBlock, 0, len(rawBlocks))
	for _, block := range rawBlocks {
		if normalized, ok := normalizeContentBlockMap(block); ok {
			content = append(content, normalized)
		}
	}
	return content
}

func normalizeContentBlockMap(block map[string]interface{}) (ContentBlock, bool) {
	blockType, _ := block["type"].(string)
	switch blockType {
	case "text", "input_text", "output_text":
		return ContentBlock{
			Type: blockType,
			Text: getStringField(block, "text"),
		}, true
	case "refusal":
		return ContentBlock{
			Type:    blockType,
			Refusal: getStringField(block, "refusal"),
		}, true
	case "input_image":
		return ContentBlock{
			Type:     blockType,
			ImageURL: getStringField(block, "image_url"),
			Detail:   getStringField(block, "detail"),
		}, true
	case "input_file":
		return ContentBlock{
			Type:     blockType,
			FileData: getStringField(block, "file_data"),
			FileURL:  getStringField(block, "file_url"),
			Filename: getStringField(block, "filename"),
		}, true
	case "input_video":
		return ContentBlock{
			Type:     blockType,
			VideoURL: getStringField(block, "video_url"),
		}, true
	default:
		return ContentBlock{}, false
	}
}

func parseToolChoiceName(obj map[string]interface{}) string {
	if name, ok := obj["name"].(string); ok {
		return name
	}
	if function, ok := obj["function"].(map[string]interface{}); ok {
		if name, ok := function["name"].(string); ok {
			return name
		}
	}
	return ""
}

func cloneRawMessage(v json.RawMessage) json.RawMessage {
	if len(v) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(v))
	copy(out, v)
	return out
}
