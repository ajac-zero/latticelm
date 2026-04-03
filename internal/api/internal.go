package api

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
