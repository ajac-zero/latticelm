package openai

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/azure"
	"github.com/openai/openai-go/option"

	"github.com/yourusername/go-llm-gateway/internal/api"
	"github.com/yourusername/go-llm-gateway/internal/config"
)

const Name = "openai"

// Provider implements the OpenAI SDK integration.
// It supports both direct OpenAI API and Azure-hosted endpoints.
type Provider struct {
	cfg    config.ProviderConfig
	client *openai.Client
	azure  bool
}

// New constructs a Provider for the direct OpenAI API.
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

// NewAzure constructs a Provider targeting Azure OpenAI.
// Azure OpenAI uses the OpenAI SDK with the azure subpackage for proper
// endpoint routing, api-version query parameter, and API key header.
func NewAzure(azureCfg config.AzureOpenAIConfig) *Provider {
	var client *openai.Client
	if azureCfg.APIKey != "" && azureCfg.Endpoint != "" {
		apiVersion := azureCfg.APIVersion
		if apiVersion == "" {
			apiVersion = "2024-12-01-preview"
		}
		c := openai.NewClient(
			azure.WithEndpoint(azureCfg.Endpoint, apiVersion),
			azure.WithAPIKey(azureCfg.APIKey),
		)
		client = &c
	}
	return &Provider{
		cfg: config.ProviderConfig{
			APIKey: azureCfg.APIKey,
		},
		client: client,
		azure:  true,
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return Name }

// Generate routes the request to OpenAI and returns a ProviderResult.
func (p *Provider) Generate(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("openai api key missing")
	}
	if p.client == nil {
		return nil, fmt.Errorf("openai client not initialized")
	}

	// Convert messages to OpenAI format
	oaiMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		var content string
		for _, block := range msg.Content {
			if block.Type == "input_text" || block.Type == "output_text" {
				content += block.Text
			}
		}

		switch msg.Role {
		case "user":
			oaiMessages = append(oaiMessages, openai.UserMessage(content))
		case "assistant":
			oaiMessages = append(oaiMessages, openai.AssistantMessage(content))
		case "system":
			oaiMessages = append(oaiMessages, openai.SystemMessage(content))
		case "developer":
			oaiMessages = append(oaiMessages, openai.SystemMessage(content))
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(req.Model),
		Messages: oaiMessages,
	}
	if req.MaxOutputTokens != nil {
		params.MaxTokens = openai.Int(int64(*req.MaxOutputTokens))
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}
	if req.TopP != nil {
		params.TopP = openai.Float(*req.TopP)
	}

	// Call OpenAI API
	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai api error: %w", err)
	}

	var combinedText string
	for _, choice := range resp.Choices {
		combinedText += choice.Message.Content
	}

	return &api.ProviderResult{
		ID:    resp.ID,
		Model: resp.Model,
		Text:  combinedText,
		Usage: api.Usage{
			InputTokens:  int(resp.Usage.PromptTokens),
			OutputTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:  int(resp.Usage.TotalTokens),
		},
	}, nil
}

// GenerateStream handles streaming requests to OpenAI.
func (p *Provider) GenerateStream(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
	deltaChan := make(chan *api.ProviderStreamDelta)
	errChan := make(chan error, 1)

	go func() {
		defer close(deltaChan)
		defer close(errChan)

		if p.cfg.APIKey == "" {
			errChan <- fmt.Errorf("openai api key missing")
			return
		}
		if p.client == nil {
			errChan <- fmt.Errorf("openai client not initialized")
			return
		}

		// Convert messages to OpenAI format
		oaiMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
		for _, msg := range messages {
			var content string
			for _, block := range msg.Content {
				if block.Type == "input_text" || block.Type == "output_text" {
					content += block.Text
				}
			}

			switch msg.Role {
			case "user":
				oaiMessages = append(oaiMessages, openai.UserMessage(content))
			case "assistant":
				oaiMessages = append(oaiMessages, openai.AssistantMessage(content))
			case "system":
				oaiMessages = append(oaiMessages, openai.SystemMessage(content))
			case "developer":
				oaiMessages = append(oaiMessages, openai.SystemMessage(content))
			}
		}

		params := openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(req.Model),
			Messages: oaiMessages,
		}
		if req.MaxOutputTokens != nil {
			params.MaxTokens = openai.Int(int64(*req.MaxOutputTokens))
		}
		if req.Temperature != nil {
			params.Temperature = openai.Float(*req.Temperature)
		}
		if req.TopP != nil {
			params.TopP = openai.Float(*req.TopP)
		}

		// Create streaming request
		stream := p.client.Chat.Completions.NewStreaming(ctx, params)

		// Process stream
		for stream.Next() {
			chunk := stream.Current()

			for _, choice := range chunk.Choices {
				if choice.Delta.Content == "" {
					continue
				}

				select {
				case deltaChan <- &api.ProviderStreamDelta{
					ID:    chunk.ID,
					Model: chunk.Model,
					Text:  choice.Delta.Content,
				}:
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

		// Send final delta
		select {
		case deltaChan <- &api.ProviderStreamDelta{Done: true}:
		case <-ctx.Done():
			errChan <- ctx.Err()
		}
	}()

	return deltaChan, errChan
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
