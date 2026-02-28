package api

import (
	"errors"
	"fmt"
)

// ResponseRequest models the Open Responses create request payload.
type ResponseRequest struct {
	Model           string            `json:"model"`
	Provider        string            `json:"provider,omitempty"`
	MaxOutputTokens int               `json:"max_output_tokens,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Input           []Message         `json:"input"`
	Stream          bool              `json:"stream,omitempty"`
}

// Message captures user, assistant, or system roles.
type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a typed content element (text, data, tool call, etc.).
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Response is a simplified Open Responses response payload.
type Response struct {
	ID       string    `json:"id"`
	Object   string    `json:"object"`
	Created  int64     `json:"created"`
	Model    string    `json:"model"`
	Provider string    `json:"provider"`
	Output   []Message `json:"output"`
	Usage    Usage     `json:"usage"`
}

// Usage captures token accounting.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// StreamChunk represents a single Server-Sent Event in a streaming response.
type StreamChunk struct {
	ID       string         `json:"id,omitempty"`
	Object   string         `json:"object"`
	Created  int64          `json:"created,omitempty"`
	Model    string         `json:"model,omitempty"`
	Provider string         `json:"provider,omitempty"`
	Delta    *StreamDelta   `json:"delta,omitempty"`
	Usage    *Usage         `json:"usage,omitempty"`
	Done     bool           `json:"done,omitempty"`
}

// StreamDelta represents incremental content in a stream chunk.
type StreamDelta struct {
	Role    string         `json:"role,omitempty"`
	Content []ContentBlock `json:"content,omitempty"`
}

// Validate performs basic structural validation.
func (r *ResponseRequest) Validate() error {
	if r == nil {
		return errors.New("request is nil")
	}
	if r.Model == "" {
		return errors.New("model is required")
	}
	if len(r.Input) == 0 {
		return errors.New("input messages are required")
	}
	for i, msg := range r.Input {
		if msg.Role == "" {
			return fmt.Errorf("input[%d] role is required", i)
		}
		if len(msg.Content) == 0 {
			return fmt.Errorf("input[%d] content is required", i)
		}
	}
	return nil
}
