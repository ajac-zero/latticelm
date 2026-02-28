package openai

import (
	"context"
	"fmt"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/yourusername/go-llm-gateway/internal/api"
	"github.com/yourusername/go-llm-gateway/internal/config"
)

const Name = "openai"

// Provider implements the OpenAI SDK integration.
type Provider struct {
	cfg    config.ProviderConfig
	client *openai.Client
}

// New constructs a Provider from configuration.
func New(cfg config.ProviderConfig) *Provider {
	var client *openai.Client
	if cfg.APIKey != "" {
		c := openai.NewClient(option.WithAPIKey(cfg.APIKey))
		client = &c
	}
	return &Provider{
		cfg:    cfg,
		client: client,
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return Name }

// Generate routes the Open Responses request to OpenAI.
func (p *Provider) Generate(ctx context.Context, req *api.ResponseRequest) (*api.Response, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("openai api key missing")
	}
	if p.client == nil {
		return nil, fmt.Errorf("openai client not initialized")
	}

	model := chooseModel(req.Model, p.cfg.Model)

	// Convert Open Responses messages to OpenAI format
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Input))
	for _, msg := range req.Input {
		var content string
		for _, block := range msg.Content {
			if block.Type == "input_text" || block.Type == "output_text" {
				content += block.Text
			}
		}
		
		switch msg.Role {
		case "user":
			messages = append(messages, openai.UserMessage(content))
		case "assistant":
			messages = append(messages, openai.AssistantMessage(content))
		case "system":
			messages = append(messages, openai.SystemMessage(content))
		}
	}

	// Call OpenAI API
	resp, err := p.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(model),
		Messages: messages,
	})
	if err != nil {
		return nil, fmt.Errorf("openai api error: %w", err)
	}

	// Convert OpenAI response to Open Responses format
	output := make([]api.Message, 0, len(resp.Choices))
	for _, choice := range resp.Choices {
		output = append(output, api.Message{
			Role: "assistant",
			Content: []api.ContentBlock{
				{Type: "output_text", Text: choice.Message.Content},
			},
		})
	}

	return &api.Response{
		ID:       resp.ID,
		Object:   "response",
		Created:  time.Now().Unix(),
		Model:    resp.Model,
		Provider: Name,
		Output:   output,
		Usage: api.Usage{
			InputTokens:  int(resp.Usage.PromptTokens),
			OutputTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:  int(resp.Usage.TotalTokens),
		},
	}, nil
}

// GenerateStream handles streaming requests to OpenAI.
func (p *Provider) GenerateStream(ctx context.Context, req *api.ResponseRequest) (<-chan *api.StreamChunk, <-chan error) {
	chunkChan := make(chan *api.StreamChunk)
	errChan := make(chan error, 1)

	go func() {
		defer close(chunkChan)
		defer close(errChan)

		if p.cfg.APIKey == "" {
			errChan <- fmt.Errorf("openai api key missing")
			return
		}
		if p.client == nil {
			errChan <- fmt.Errorf("openai client not initialized")
			return
		}

		model := chooseModel(req.Model, p.cfg.Model)

		// Convert messages
		messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Input))
		for _, msg := range req.Input {
			var content string
			for _, block := range msg.Content {
				if block.Type == "input_text" || block.Type == "output_text" {
					content += block.Text
				}
				}
				
				switch msg.Role {
				case "user":
				messages = append(messages, openai.UserMessage(content))
				case "assistant":
				messages = append(messages, openai.AssistantMessage(content))
				case "system":
				messages = append(messages, openai.SystemMessage(content))
				}
				}

				// Create streaming request
		stream := p.client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(model),
			Messages: messages,
		})

		// Process stream
		for stream.Next() {
			chunk := stream.Current()
			
			for _, choice := range chunk.Choices {
				delta := &api.StreamDelta{}
				
				if choice.Delta.Role != "" {
					delta.Role = string(choice.Delta.Role)
				}
				
				if choice.Delta.Content != "" {
					delta.Content = []api.ContentBlock{
						{Type: "output_text", Text: choice.Delta.Content},
					}
				}

				streamChunk := &api.StreamChunk{
					ID:       chunk.ID,
					Object:   "response.chunk",
					Created:  time.Now().Unix(),
					Model:    chunk.Model,
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
		}

		if err := stream.Err(); err != nil {
			errChan <- fmt.Errorf("openai stream error: %w", err)
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
	return "gpt-4o-mini"
}
