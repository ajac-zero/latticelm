package anthropic

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/yourusername/go-llm-gateway/internal/api"
	"github.com/yourusername/go-llm-gateway/internal/config"
)

const Name = "anthropic"

// Provider implements the Anthropic SDK integration.
type Provider struct {
	cfg    config.ProviderConfig
	client *anthropic.Client
}

// New constructs a Provider from configuration.
func New(cfg config.ProviderConfig) *Provider {
	var client *anthropic.Client
	if cfg.APIKey != "" {
		c := anthropic.NewClient(option.WithAPIKey(cfg.APIKey))
		client = &c
	}
	return &Provider{
		cfg:    cfg,
		client: client,
	}
}

func (p *Provider) Name() string { return Name }

// Generate routes the Open Responses request to Anthropic's API.
func (p *Provider) Generate(ctx context.Context, req *api.ResponseRequest) (*api.Response, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic api key missing")
	}
	if p.client == nil {
		return nil, fmt.Errorf("anthropic client not initialized")
	}

	model := chooseModel(req.Model, p.cfg.Model)

	// Convert Open Responses messages to Anthropic format
	messages := make([]anthropic.MessageParam, 0, len(req.Input))
	var system string
	
	for _, msg := range req.Input {
		var content string
		for _, block := range msg.Content {
			if block.Type == "input_text" {
				content += block.Text
			}
		}
		
		switch msg.Role {
		case "user":
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
		case "assistant":
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(content)))
		case "system":
			system = content
		}
	}

	// Build request params
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		Messages:  messages,
		MaxTokens: int64(4096),
	}
	
	if system != "" {
		systemBlocks := []anthropic.TextBlockParam{
			{Text: system, Type: "text"},
		}
		params.System = systemBlocks
	}

	// Call Anthropic API
	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic api error: %w", err)
	}

	// Convert Anthropic response to Open Responses format
	output := make([]api.Message, 0, 1)
	var text string
	
	for _, block := range resp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	
	output = append(output, api.Message{
		Role: "assistant",
		Content: []api.ContentBlock{
			{Type: "output_text", Text: text},
		},
	})

	return &api.Response{
		ID:       resp.ID,
		Object:   "response",
		Created:  time.Now().Unix(),
		Model:    string(resp.Model),
		Provider: Name,
		Output:   output,
		Usage: api.Usage{
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
			TotalTokens:  int(resp.Usage.InputTokens + resp.Usage.OutputTokens),
		},
	}, nil
}

// GenerateStream handles streaming requests to Anthropic.
func (p *Provider) GenerateStream(ctx context.Context, req *api.ResponseRequest) (<-chan *api.StreamChunk, <-chan error) {
	chunkChan := make(chan *api.StreamChunk)
	errChan := make(chan error, 1)

	go func() {
		defer close(chunkChan)
		defer close(errChan)

		if p.cfg.APIKey == "" {
			errChan <- fmt.Errorf("anthropic api key missing")
			return
		}
		if p.client == nil {
			errChan <- fmt.Errorf("anthropic client not initialized")
			return
		}

		model := chooseModel(req.Model, p.cfg.Model)

		// Convert messages
		messages := make([]anthropic.MessageParam, 0, len(req.Input))
		var system string
		
		for _, msg := range req.Input {
			var content string
			for _, block := range msg.Content {
				if block.Type == "input_text" {
					content += block.Text
				}
			}
			
			switch msg.Role {
			case "user":
				messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
			case "assistant":
				messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(content)))
			case "system":
				system = content
			}
		}

		// Build params
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(model),
			Messages:  messages,
			MaxTokens: int64(4096),
		}
		
		if system != "" {
			systemBlocks := []anthropic.TextBlockParam{
				{Text: system, Type: "text"},
			}
			params.System = systemBlocks
		}

		// Create stream
		stream := p.client.Messages.NewStreaming(ctx, params)

		// Process stream
		for stream.Next() {
			event := stream.Current()
			
			delta := &api.StreamDelta{}
			var text string
			
			// Handle different event types
			if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
				text = event.Delta.Text
				delta.Content = []api.ContentBlock{
					{Type: "output_text", Text: text},
				}
			}
			
			if event.Type == "message_start" {
				delta.Role = "assistant"
			}

			streamChunk := &api.StreamChunk{
				Object:   "response.chunk",
				Created:  time.Now().Unix(),
				Model:    string(model),
				Provider: Name,
				Delta:    delta,
			}

			select {
			case chunkChan <- streamChunk:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}

		if err := stream.Err(); err != nil {
			errChan <- fmt.Errorf("anthropic stream error: %w", err)
			return
		}

		// Send final chunk
		select {
		case chunkChan <- &api.StreamChunk{Object: "response.chunk", Done: true}:
		case <-ctx.Done():
			errChan <- ctx.Err()
		}
	}()

	return chunkChan, errChan
}

func chooseModel(requested, defaultModel string) string {
	if requested != "" {
		return requested
	}
	if defaultModel != "" {
		return defaultModel
	}
	return "claude-3-5-sonnet"
}
