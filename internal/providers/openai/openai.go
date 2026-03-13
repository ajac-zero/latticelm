package openai

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/azure"
	"github.com/openai/openai-go/v3/option"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/config"
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
			// If assistant message has tool calls, include them
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					toolCalls[i] = openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: tc.Arguments,
							},
						},
					}
				}
				msgParam := openai.ChatCompletionAssistantMessageParam{
					ToolCalls: toolCalls,
				}
				if content != "" {
					msgParam.Content.OfString = openai.String(content)
				}
				oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &msgParam,
				})
			} else {
				oaiMessages = append(oaiMessages, openai.AssistantMessage(content))
			}
		case "system":
			oaiMessages = append(oaiMessages, openai.SystemMessage(content))
		case "developer":
			oaiMessages = append(oaiMessages, openai.SystemMessage(content))
		case "tool":
			oaiMessages = append(oaiMessages, openai.ToolMessage(content, msg.CallID))
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

	// Add tools if present
	if len(req.Tools) > 0 {
		tools, err := parseTools(req)
		if err != nil {
			return nil, fmt.Errorf("parse tools: %w", err)
		}
		params.Tools = tools
	}

	// Add tool_choice if present
	if len(req.ToolChoice) > 0 {
		toolChoice, err := parseToolChoice(req)
		if err != nil {
			return nil, fmt.Errorf("parse tool_choice: %w", err)
		}
		params.ToolChoice = toolChoice
	}

	// Add parallel_tool_calls if specified
	if req.ParallelToolCalls != nil {
		params.ParallelToolCalls = openai.Bool(*req.ParallelToolCalls)
	}

	// Call OpenAI API
	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai api error: %w", err)
	}

	var combinedText string
	var toolCalls []api.ToolCall

	for _, choice := range resp.Choices {
		combinedText += choice.Message.Content
		if len(choice.Message.ToolCalls) > 0 {
			toolCalls = append(toolCalls, extractToolCalls(choice.Message)...)
		}
	}

	return &api.ProviderResult{
		ID:        resp.ID,
		Model:     resp.Model,
		Text:      combinedText,
		ToolCalls: toolCalls,
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
				// If assistant message has tool calls, include them
				if len(msg.ToolCalls) > 0 {
					toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, len(msg.ToolCalls))
					for i, tc := range msg.ToolCalls {
						toolCalls[i] = openai.ChatCompletionMessageToolCallUnionParam{
							OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
								ID: tc.ID,
								Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
									Name:      tc.Name,
									Arguments: tc.Arguments,
								},
							},
						}
					}
					msgParam := openai.ChatCompletionAssistantMessageParam{
						ToolCalls: toolCalls,
					}
					if content != "" {
						msgParam.Content.OfString = openai.String(content)
					}
					oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
						OfAssistant: &msgParam,
					})
				} else {
					oaiMessages = append(oaiMessages, openai.AssistantMessage(content))
				}
			case "system":
				oaiMessages = append(oaiMessages, openai.SystemMessage(content))
			case "developer":
				oaiMessages = append(oaiMessages, openai.SystemMessage(content))
			case "tool":
				oaiMessages = append(oaiMessages, openai.ToolMessage(content, msg.CallID))
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

		// Add tools if present
		if len(req.Tools) > 0 {
			tools, err := parseTools(req)
			if err != nil {
				errChan <- fmt.Errorf("parse tools: %w", err)
				return
			}
			params.Tools = tools
		}

		// Add tool_choice if present
		if len(req.ToolChoice) > 0 {
			toolChoice, err := parseToolChoice(req)
			if err != nil {
				errChan <- fmt.Errorf("parse tool_choice: %w", err)
				return
			}
			params.ToolChoice = toolChoice
		}

		// Add parallel_tool_calls if specified
		if req.ParallelToolCalls != nil {
			params.ParallelToolCalls = openai.Bool(*req.ParallelToolCalls)
		}

		// Request usage in the final stream chunk
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		}

		// Create streaming request
		stream := p.client.Chat.Completions.NewStreaming(ctx, params)

		var streamUsage *api.Usage

		// Process stream
		for stream.Next() {
			chunk := stream.Current()

			// Capture usage from the final chunk
			if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
				streamUsage = &api.Usage{
					InputTokens:  int(chunk.Usage.PromptTokens),
					OutputTokens: int(chunk.Usage.CompletionTokens),
					TotalTokens:  int(chunk.Usage.TotalTokens),
				}
			}

			for _, choice := range chunk.Choices {
				// Handle text content
				if choice.Delta.Content != "" {
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

				// Handle tool call deltas
				if len(choice.Delta.ToolCalls) > 0 {
					delta := extractToolCallDelta(choice)
					if delta != nil {
						select {
						case deltaChan <- &api.ProviderStreamDelta{
							ID:            chunk.ID,
							Model:         chunk.Model,
							ToolCallDelta: delta,
						}:
						case <-ctx.Done():
							errChan <- ctx.Err()
							return
						}
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			errChan <- fmt.Errorf("openai stream error: %w", err)
			return
		}

		// Send final delta with usage
		select {
		case deltaChan <- &api.ProviderStreamDelta{Done: true, Usage: streamUsage}:
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
