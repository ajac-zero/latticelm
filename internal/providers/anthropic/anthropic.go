package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

	anthropicMsgs, systemBlocks, err := buildAnthropicMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("convert messages: %w", err)
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

	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}

	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}
	if req.TopP != nil {
		params.TopP = anthropic.Float(*req.TopP)
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

	// Call Anthropic API
	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic api error: %w", err)
	}

	// Extract text and tool calls from response
	var text string
	var toolCalls []api.ToolCall
	replayMessage := &api.Message{Role: "assistant"}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textBlock := block.AsText()
			text += textBlock.Text
			replayMessage.Content = append(replayMessage.Content, api.ContentBlock{
				Type: "output_text",
				Text: textBlock.Text,
			})
		case "thinking":
			thinkingBlock := block.AsThinking()
			replayMessage.Content = append(replayMessage.Content, api.ContentBlock{
				Type:      "anthropic_thinking",
				Text:      thinkingBlock.Thinking,
				Signature: thinkingBlock.Signature,
			})
		case "redacted_thinking":
			redacted := block.AsRedactedThinking()
			replayMessage.Content = append(replayMessage.Content, api.ContentBlock{
				Type: "anthropic_redacted_thinking",
				Data: redacted.Data,
			})
		case "tool_use":
			// Extract tool calls
			toolUse := block.AsToolUse()
			argsJSON, _ := json.Marshal(toolUse.Input)
			toolCall := api.ToolCall{
				ID:        toolUse.ID,
				Name:      toolUse.Name,
				Arguments: string(argsJSON),
			}
			toolCalls = append(toolCalls, toolCall)
			replayMessage.ToolCalls = append(replayMessage.ToolCalls, toolCall)
		}
	}
	if len(replayMessage.Content) == 0 && len(replayMessage.ToolCalls) == 0 {
		replayMessage = nil
	}

	return &api.ProviderResult{
		ID:            resp.ID,
		Model:         string(resp.Model),
		Text:          text,
		ToolCalls:     toolCalls,
		ReplayMessage: replayMessage,
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

		anthropicMsgs, systemBlocks, err := buildAnthropicMessages(messages)
		if err != nil {
			errChan <- fmt.Errorf("convert messages: %w", err)
			return
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

		if len(systemBlocks) > 0 {
			params.System = systemBlocks
		}

		if req.Temperature != nil {
			params.Temperature = anthropic.Float(*req.Temperature)
		}
		if req.TopP != nil {
			params.TopP = anthropic.Float(*req.TopP)
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

		// Create stream
		stream := p.client.Messages.NewStreaming(ctx, params)

		// Track content block index and tool call state
		var contentBlockIndex int
		var inputTokens, outputTokens int64
		var responseID string
		var responseModel string
		replayBlocks := make(map[int]*api.ContentBlock)
		streamToolCalls := make(map[int]*api.ToolCall)

		// Process stream
		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "message_start":
				inputTokens = event.Message.Usage.InputTokens
				responseID = event.Message.ID
				responseModel = string(event.Message.Model)

			case "message_delta":
				outputTokens = event.Usage.OutputTokens

			case "content_block_start":
				// New content block (text or tool_use)
				contentBlockIndex = int(event.Index)
				switch event.ContentBlock.Type {
				case "tool_use":
					// Send tool call delta with ID and name
					toolUse := event.ContentBlock.AsToolUse()
					streamToolCalls[contentBlockIndex] = &api.ToolCall{
						ID:   toolUse.ID,
						Name: toolUse.Name,
					}
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
				case "text":
					textBlock := event.ContentBlock.AsText()
					replayBlocks[contentBlockIndex] = &api.ContentBlock{
						Type: "output_text",
						Text: textBlock.Text,
					}
				case "thinking":
					thinkingBlock := event.ContentBlock.AsThinking()
					replayBlocks[contentBlockIndex] = &api.ContentBlock{
						Type:      "anthropic_thinking",
						Text:      thinkingBlock.Thinking,
						Signature: thinkingBlock.Signature,
					}
				case "redacted_thinking":
					redacted := event.ContentBlock.AsRedactedThinking()
					replayBlocks[contentBlockIndex] = &api.ContentBlock{
						Type: "anthropic_redacted_thinking",
						Data: redacted.Data,
					}
				}

			case "content_block_delta":
				if event.Delta.Type == "text_delta" {
					if block := replayBlocks[int(event.Index)]; block != nil {
						block.Text += event.Delta.Text
					}
					// Text streaming
					select {
					case deltaChan <- &api.ProviderStreamDelta{ID: responseID, Model: responseModel, Text: event.Delta.Text}:
					case <-ctx.Done():
						errChan <- ctx.Err()
						return
					}
				} else if event.Delta.Type == "thinking_delta" {
					if block := replayBlocks[int(event.Index)]; block != nil {
						block.Text += event.Delta.Thinking
					}
				} else if event.Delta.Type == "signature_delta" {
					if block := replayBlocks[int(event.Index)]; block != nil {
						block.Signature = event.Delta.Signature
					}
				} else if event.Delta.Type == "input_json_delta" {
					if toolCall := streamToolCalls[int(event.Index)]; toolCall != nil {
						toolCall.Arguments += event.Delta.PartialJSON
					}
					// Tool arguments streaming
					delta := &api.ToolCallDelta{
						Index:     int(event.Index),
						Arguments: event.Delta.PartialJSON,
					}
					select {
					case deltaChan <- &api.ProviderStreamDelta{ID: responseID, Model: responseModel, ToolCallDelta: delta}:
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

		replayMessage := buildStreamReplayMessage(replayBlocks, streamToolCalls)

		// Send final delta with usage
		select {
		case deltaChan <- &api.ProviderStreamDelta{
			ID:            responseID,
			Model:         responseModel,
			Done:          true,
			ReplayMessage: replayMessage,
			Usage: &api.Usage{
				InputTokens:  int(inputTokens),
				OutputTokens: int(outputTokens),
				TotalTokens:  int(inputTokens + outputTokens),
			},
		}:
		case <-ctx.Done():
			errChan <- ctx.Err()
		}
	}()

	return deltaChan, errChan
}

func buildStreamReplayMessage(blocks map[int]*api.ContentBlock, toolCalls map[int]*api.ToolCall) *api.Message {
	replayMessage := &api.Message{Role: "assistant"}

	maxIndex := -1
	for idx := range blocks {
		if idx > maxIndex {
			maxIndex = idx
		}
	}
	for idx := range toolCalls {
		if idx > maxIndex {
			maxIndex = idx
		}
	}

	for i := 0; i <= maxIndex; i++ {
		if block := blocks[i]; block != nil {
			replayMessage.Content = append(replayMessage.Content, *block)
		}
		if toolCall := toolCalls[i]; toolCall != nil {
			replayMessage.ToolCalls = append(replayMessage.ToolCalls, *toolCall)
		}
	}

	if len(replayMessage.Content) == 0 && len(replayMessage.ToolCalls) == 0 {
		return nil
	}
	return replayMessage
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

func buildAnthropicMessages(messages []api.Message) ([]anthropic.MessageParam, []anthropic.TextBlockParam, error) {
	anthropicMsgs := make([]anthropic.MessageParam, 0, len(messages))
	systemBlocks := make([]anthropic.TextBlockParam, 0)

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			blocks, err := buildAnthropicTextBlocks(msg.Content, msg.Role)
			if err != nil {
				return nil, nil, err
			}
			anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(blocks...))
		case "assistant":
			blocks, err := buildAnthropicAssistantBlocks(msg.Content, msg.ToolCalls)
			if err != nil {
				return nil, nil, err
			}
			if len(blocks) > 0 {
				anthropicMsgs = append(anthropicMsgs, anthropic.NewAssistantMessage(blocks...))
			}
		case "tool":
			content, err := buildAnthropicToolResultContent(msg.Content)
			if err != nil {
				return nil, nil, err
			}
			anthropicMsgs = append(anthropicMsgs, anthropic.NewUserMessage(
				anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: msg.CallID,
						Content:   content,
					},
				},
			))
		case "system", "developer":
			blocks, err := buildAnthropicSystemBlocks(msg.Content, msg.Role)
			if err != nil {
				return nil, nil, err
			}
			systemBlocks = append(systemBlocks, blocks...)
		}
	}

	return anthropicMsgs, systemBlocks, nil
}

func buildAnthropicSystemBlocks(blocks []api.ContentBlock, role string) ([]anthropic.TextBlockParam, error) {
	result := make([]anthropic.TextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "encrypted_reasoning" {
			continue
		}
		text, ok := block.TextValue()
		if !ok {
			return nil, fmt.Errorf("%s messages only support text content in the Anthropic provider; found %q", role, block.Type)
		}
		result = append(result, anthropic.TextBlockParam{Text: text, Type: "text"})
	}
	return result, nil
}

func buildAnthropicTextBlocks(blocks []api.ContentBlock, role string) ([]anthropic.ContentBlockParamUnion, error) {
	result := make([]anthropic.ContentBlockParamUnion, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "text", "input_text", "output_text":
			result = append(result, anthropic.NewTextBlock(block.Text))
		case "input_image":
			imgBlock, err := buildAnthropicImageBlock(block)
			if err != nil {
				return nil, fmt.Errorf("build image block: %w", err)
			}
			result = append(result, imgBlock)
		case "input_file":
			docBlock, err := buildAnthropicDocumentBlock(block)
			if err != nil {
				return nil, fmt.Errorf("build document block: %w", err)
			}
			result = append(result, docBlock)
		case "encrypted_reasoning":
			continue
		default:
			return nil, fmt.Errorf("%s messages do not support %q content in the Anthropic provider", role, block.Type)
		}
	}
	return result, nil
}

func buildAnthropicImageBlock(block api.ContentBlock) (anthropic.ContentBlockParamUnion, error) {
	if block.ImageURL != "" {
		if strings.HasPrefix(block.ImageURL, "data:") {
			mediaType, data, err := parseDataURL(block.ImageURL)
			if err != nil {
				return anthropic.ContentBlockParamUnion{}, fmt.Errorf("parse image data: %w", err)
			}
			return anthropic.NewImageBlockBase64(mediaType, data), nil
		}
		// URL-based image
		return anthropic.NewImageBlock(anthropic.URLImageSourceParam{
			URL: block.ImageURL,
		}), nil
	}
	// Note: Base64 images would be passed via FileData field, but that's not currently
	// populated in the ContentBlock. When that's added, we can support base64 images here.
	return anthropic.ContentBlockParamUnion{}, fmt.Errorf("image input requires image_url field")
}

func buildAnthropicDocumentBlock(block api.ContentBlock) (anthropic.ContentBlockParamUnion, error) {
	if block.FileURL != "" {
		if inferAnthropicDocumentMediaType(block.FileURL) != "application/pdf" {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("anthropic only supports PDF document URLs")
		}
		// URL-based document (PDF)
		return anthropic.NewDocumentBlock(anthropic.URLPDFSourceParam{
			URL: block.FileURL,
		}), nil
	}
	if block.FileData != "" {
		mediaType, data, err := parseBase64Payload(block.FileData, inferAnthropicDocumentMediaType(block.Filename))
		if err != nil {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("parse file data: %w", err)
		}
		switch mediaType {
		case "application/pdf":
			return anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{
				Data: data,
			}), nil
		case "text/plain":
			text, err := decodeBase64Text(data)
			if err != nil {
				return anthropic.ContentBlockParamUnion{}, err
			}
			return anthropic.NewDocumentBlock(anthropic.PlainTextSourceParam{
				Data: text,
			}), nil
		default:
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("anthropic only supports PDF or text documents, got %q", mediaType)
		}
	}
	return anthropic.ContentBlockParamUnion{}, fmt.Errorf("document input requires file_url or file_data field")
}

func inferAnthropicDocumentMediaType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".txt"),
		strings.HasSuffix(lower, ".md"),
		strings.HasSuffix(lower, ".csv"),
		strings.HasSuffix(lower, ".json"),
		strings.HasSuffix(lower, ".xml"),
		strings.HasSuffix(lower, ".yaml"),
		strings.HasSuffix(lower, ".yml"):
		return "text/plain"
	default:
		return ""
	}
}

func buildAnthropicAssistantBlocks(blocks []api.ContentBlock, toolCalls []api.ToolCall) ([]anthropic.ContentBlockParamUnion, error) {
	contentBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(blocks)+len(toolCalls))
	for _, block := range blocks {
		switch block.Type {
		case "encrypted_reasoning":
			continue
		case "anthropic_thinking":
			contentBlocks = append(contentBlocks, anthropic.NewThinkingBlock(block.Signature, block.Text))
		case "anthropic_redacted_thinking":
			contentBlocks = append(contentBlocks, anthropic.NewRedactedThinkingBlock(block.Data))
		default:
			text, ok := block.TextValue()
			if !ok {
				return nil, fmt.Errorf("assistant messages only support text or Anthropic thinking content; found %q", block.Type)
			}
			contentBlocks = append(contentBlocks, anthropic.NewTextBlock(text))
		}
	}
	for _, tc := range toolCalls {
		var input map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
			continue
		}
		contentBlocks = append(contentBlocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
	}
	return contentBlocks, nil
}

func buildAnthropicToolResultContent(blocks []api.ContentBlock) ([]anthropic.ToolResultBlockParamContentUnion, error) {
	content := make([]anthropic.ToolResultBlockParamContentUnion, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "encrypted_reasoning" {
			continue
		}
		text, ok := block.TextValue()
		if !ok {
			return nil, fmt.Errorf("tool results only support text content in the Anthropic provider; found %q", block.Type)
		}
		content = append(content, anthropic.ToolResultBlockParamContentUnion{
			OfText: &anthropic.TextBlockParam{Text: text},
		})
	}
	return content, nil
}
