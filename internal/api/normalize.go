package api

import "encoding/json"

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

// InheritMissingContext copies fields from prev that are not set on r.
// Used when continuing a conversation via previous_response_id.
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

// getStringField safely extracts a string field from a map.
func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
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
