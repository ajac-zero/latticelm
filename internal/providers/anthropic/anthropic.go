package anthropic

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/yourusername/go-llm-gateway/internal/api"
	"github.com/yourusername/go-llm-gateway/internal/config"
)

const Name = "anthropic"

// Provider implements the Anthropic SDK integration.
// It supports both direct Anthropic API and Azure-hosted (Microsoft Foundry) endpoints.
type Provider struct {
	cfg    config.ProviderConfig
	client *anthropic.Client
	azure  bool
}

// New constructs a Provider for the direct Anthropic API.
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

// NewAzure constructs a Provider targeting Azure-hosted Anthropic (Microsoft Foundry).
// The Azure endpoint uses api-key header auth and a base URL like
// https://<resource>.services.ai.azure.com/anthropic.
func NewAzure(azureCfg config.AzureAnthropicConfig) *Provider {
	var client *anthropic.Client
	if azureCfg.APIKey != "" && azureCfg.Endpoint != "" {
		c := anthropic.NewClient(
			option.WithBaseURL(azureCfg.Endpoint),
			option.WithAPIKey("unused"),
			option.WithAuthToken(azureCfg.APIKey),
		)
		client = &c
	}
	return &Provider{
		cfg: config.ProviderConfig{
			APIKey: azureCfg.APIKey,
			Model:  azureCfg.Model,
		},
		client: client,
		azure:  true,
	}
}

func (p *Provider) Name() string { return Name }

// Generate routes the request to Anthropic's API.
func (p *Provider) Generate(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic api key missing")
	}
	if p.client == nil {
		return nil, fmt.Errorf("anthropic client not initialized")
	}

	// Convert messages to Anthropic format
	anthropicMsgs := make([]anthropic.MessageParam, 0, len(messages))
	var system string

	for _, msg := range messages {
		var content string
		for _, block := range msg.Content {
			if block.Type == "input_text" || block.Type == "output_text" {
				content += block.Text
			}
		}

		switch msg.Role {
		case "user":
			anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
		case "assistant":
			anthropicMsgs = append(anthropicMsgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(content)))
		case "system", "developer":
			system = content
		}
	}

	// Build request params
	maxTokens := int64(4096)
	if req.MaxOutputTokens != nil {
		maxTokens = int64(*req.MaxOutputTokens)
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		Messages:  anthropicMsgs,
		MaxTokens: maxTokens,
	}

	if system != "" {
		systemBlocks := []anthropic.TextBlockParam{
			{Text: system, Type: "text"},
		}
		params.System = systemBlocks
	}

	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}
	if req.TopP != nil {
		params.TopP = anthropic.Float(*req.TopP)
	}

	// Call Anthropic API
	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic api error: %w", err)
	}

	// Extract text from response
	var text string
	for _, block := range resp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return &api.ProviderResult{
		ID:    resp.ID,
		Model: string(resp.Model),
		Text:  text,
		Usage: api.Usage{
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
			TotalTokens:  int(resp.Usage.InputTokens + resp.Usage.OutputTokens),
		},
	}, nil
}

// GenerateStream handles streaming requests to Anthropic.
func (p *Provider) GenerateStream(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
	deltaChan := make(chan *api.ProviderStreamDelta)
	errChan := make(chan error, 1)

	go func() {
		defer close(deltaChan)
		defer close(errChan)

		if p.cfg.APIKey == "" {
			errChan <- fmt.Errorf("anthropic api key missing")
			return
		}
		if p.client == nil {
			errChan <- fmt.Errorf("anthropic client not initialized")
			return
		}

		// Convert messages to Anthropic format
		anthropicMsgs := make([]anthropic.MessageParam, 0, len(messages))
		var system string

		for _, msg := range messages {
			var content string
			for _, block := range msg.Content {
				if block.Type == "input_text" || block.Type == "output_text" {
					content += block.Text
				}
			}

			switch msg.Role {
			case "user":
				anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
			case "assistant":
				anthropicMsgs = append(anthropicMsgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(content)))
			case "system", "developer":
				system = content
			}
		}

		// Build params
		maxTokens := int64(4096)
		if req.MaxOutputTokens != nil {
			maxTokens = int64(*req.MaxOutputTokens)
		}

		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(req.Model),
			Messages:  anthropicMsgs,
			MaxTokens: maxTokens,
		}

		if system != "" {
			systemBlocks := []anthropic.TextBlockParam{
				{Text: system, Type: "text"},
			}
			params.System = systemBlocks
		}

		if req.Temperature != nil {
			params.Temperature = anthropic.Float(*req.Temperature)
		}
		if req.TopP != nil {
			params.TopP = anthropic.Float(*req.TopP)
		}

		// Create stream
		stream := p.client.Messages.NewStreaming(ctx, params)

		// Process stream
		for stream.Next() {
			event := stream.Current()

			if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
				select {
				case deltaChan <- &api.ProviderStreamDelta{Text: event.Delta.Text}:
				case <-ctx.Done():
					errChan <- ctx.Err()
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			errChan <- fmt.Errorf("anthropic stream error: %w", err)
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
	return "claude-3-5-sonnet"
}
