package anthropic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/config"
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
			// Build content blocks including text and tool calls
			var contentBlocks []anthropic.ContentBlockParamUnion
			if content != "" {
				contentBlocks = append(contentBlocks, anthropic.NewTextBlock(content))
			}
			// Add tool use blocks
			for _, tc := range msg.ToolCalls {
				var input map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
					// If unmarshal fails, skip this tool call
					continue
				}
				contentBlocks = append(contentBlocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
			}
			if len(contentBlocks) > 0 {
				anthropicMsgs = append(anthropicMsgs, anthropic.NewAssistantMessage(contentBlocks...))
			}
		case "tool":
			// Tool results must be in user message with tool_result blocks
			anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(msg.CallID, content, false),
			))
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

	// Add tools if present
	if req.Tools != nil && len(req.Tools) > 0 {
		tools, err := parseTools(req)
		if err != nil {
			return nil, fmt.Errorf("parse tools: %w", err)
		}
		params.Tools = tools
	}

	// Add tool_choice if present
	if req.ToolChoice != nil && len(req.ToolChoice) > 0 {
		toolChoice, err := parseToolChoice(req)
		if err != nil {
			return nil, fmt.Errorf("parse tool_choice: %w", err)
		}
		params.ToolChoice = toolChoice
	}

	// Call Anthropic API
	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic api error: %w", err)
	}

	// Extract text and tool calls from response
	var text string
	var toolCalls []api.ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			text += block.AsText().Text
		case "tool_use":
			// Extract tool calls
			toolUse := block.AsToolUse()
			argsJSON, _ := json.Marshal(toolUse.Input)
			toolCalls = append(toolCalls, api.ToolCall{
				ID:        toolUse.ID,
				Name:      toolUse.Name,
				Arguments: string(argsJSON),
			})
		}
	}

	return &api.ProviderResult{
		ID:        resp.ID,
		Model:     string(resp.Model),
		Text:      text,
		ToolCalls: toolCalls,
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
				// Build content blocks including text and tool calls
				var contentBlocks []anthropic.ContentBlockParamUnion
				if content != "" {
					contentBlocks = append(contentBlocks, anthropic.NewTextBlock(content))
				}
				// Add tool use blocks
				for _, tc := range msg.ToolCalls {
					var input map[string]interface{}
					if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
						// If unmarshal fails, skip this tool call
						continue
					}
					contentBlocks = append(contentBlocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
				}
				if len(contentBlocks) > 0 {
					anthropicMsgs = append(anthropicMsgs, anthropic.NewAssistantMessage(contentBlocks...))
				}
			case "tool":
				// Tool results must be in user message with tool_result blocks
				anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(
					anthropic.NewToolResultBlock(msg.CallID, content, false),
				))
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

		// Add tools if present
		if req.Tools != nil && len(req.Tools) > 0 {
			tools, err := parseTools(req)
			if err != nil {
				errChan <- fmt.Errorf("parse tools: %w", err)
				return
			}
			params.Tools = tools
		}

		// Add tool_choice if present
		if req.ToolChoice != nil && len(req.ToolChoice) > 0 {
			toolChoice, err := parseToolChoice(req)
			if err != nil {
				errChan <- fmt.Errorf("parse tool_choice: %w", err)
				return
			}
			params.ToolChoice = toolChoice
		}

		// Create stream
		stream := p.client.Messages.NewStreaming(ctx, params)

		// Track content block index and tool call state
		var contentBlockIndex int

		// Process stream
		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "content_block_start":
				// New content block (text or tool_use)
				contentBlockIndex = int(event.Index)
				if event.ContentBlock.Type == "tool_use" {
					// Send tool call delta with ID and name
					toolUse := event.ContentBlock.AsToolUse()
					delta := &api.ToolCallDelta{
						Index: contentBlockIndex,
						ID:    toolUse.ID,
						Name:  toolUse.Name,
					}
					select {
					case deltaChan <- &api.ProviderStreamDelta{ToolCallDelta: delta}:
					case <-ctx.Done():
						errChan <- ctx.Err()
						return
					}
				}

			case "content_block_delta":
				if event.Delta.Type == "text_delta" {
					// Text streaming
					select {
					case deltaChan <- &api.ProviderStreamDelta{Text: event.Delta.Text}:
					case <-ctx.Done():
						errChan <- ctx.Err()
						return
					}
				} else if event.Delta.Type == "input_json_delta" {
					// Tool arguments streaming
					delta := &api.ToolCallDelta{
						Index:     int(event.Index),
						Arguments: event.Delta.PartialJSON,
					}
					select {
					case deltaChan <- &api.ProviderStreamDelta{ToolCallDelta: delta}:
					case <-ctx.Done():
						errChan <- ctx.Err()
						return
					}
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
