package google

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/genai"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/config"
)

const Name = "google"

// Provider implements the Google Generative AI integration.
type Provider struct {
	cfg    config.ProviderConfig
	client *genai.Client
}

// New constructs a Provider using the Google AI API with API key authentication.
func New(cfg config.ProviderConfig) (*Provider, error) {
	var client *genai.Client
	if cfg.APIKey != "" {
		var err error
		client, err = genai.NewClient(context.Background(), &genai.ClientConfig{
			APIKey: cfg.APIKey,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create google client: %w", err)
		}
	}
	return &Provider{
		cfg:    cfg,
		client: client,
	}, nil
}

// NewVertexAI constructs a Provider targeting Vertex AI.
// Vertex AI uses the same genai SDK but with GCP project/location configuration
// and Application Default Credentials (ADC) or service account authentication.
func NewVertexAI(vertexCfg config.VertexAIConfig) (*Provider, error) {
	var client *genai.Client
	if vertexCfg.Project != "" && vertexCfg.Location != "" {
		var err error
		client, err = genai.NewClient(context.Background(), &genai.ClientConfig{
			Project:  vertexCfg.Project,
			Location: vertexCfg.Location,
			Backend:  genai.BackendVertexAI,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create vertex ai client: %w", err)
		}
	}
	return &Provider{
		cfg: config.ProviderConfig{
			// Vertex AI doesn't use API key, but set empty for consistency
			APIKey: "",
		},
		client: client,
	}, nil
}

func (p *Provider) Name() string { return Name }

// Generate routes the request to Gemini and returns a ProviderResult.
func (p *Provider) Generate(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
	if p.client == nil {
		return nil, fmt.Errorf("google client not initialized")
	}

	model := req.Model

	contents, systemText := convertMessages(messages)

	// Parse tools if present
	var tools []*genai.Tool
	if len(req.Tools) > 0 {
		var err error
		tools, err = parseTools(req)
		if err != nil {
			return nil, fmt.Errorf("parse tools: %w", err)
		}
	}

	// Parse tool_choice if present
	var toolConfig *genai.ToolConfig
	if len(req.ToolChoice) > 0 {
		var err error
		toolConfig, err = parseToolChoice(req)
		if err != nil {
			return nil, fmt.Errorf("parse tool_choice: %w", err)
		}
	}

	config := buildConfig(systemText, req, tools, toolConfig)

	resp, err := p.client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("google api error: %w", err)
	}

	var text string
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part != nil {
				text += part.Text
			}
		}
	}

	var toolCalls []api.ToolCall
	if len(resp.Candidates) > 0 {
		toolCalls = extractToolCalls(resp)
	}

	var inputTokens, outputTokens int
	if resp.UsageMetadata != nil {
		inputTokens = int(resp.UsageMetadata.PromptTokenCount)
		outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return &api.ProviderResult{
		ID:        uuid.NewString(),
		Model:     model,
		Text:      text,
		ToolCalls: toolCalls,
		Usage: api.Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  inputTokens + outputTokens,
		},
	}, nil
}

// GenerateStream handles streaming requests to Google.
func (p *Provider) GenerateStream(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
	deltaChan := make(chan *api.ProviderStreamDelta)
	errChan := make(chan error, 1)

	go func() {
		defer close(deltaChan)
		defer close(errChan)

		if p.client == nil {
			errChan <- fmt.Errorf("google client not initialized")
			return
		}

		model := req.Model

		contents, systemText := convertMessages(messages)

		// Parse tools if present
		var tools []*genai.Tool
		if len(req.Tools) > 0 {
			var err error
			tools, err = parseTools(req)
			if err != nil {
				errChan <- fmt.Errorf("parse tools: %w", err)
				return
			}
		}

		// Parse tool_choice if present
		var toolConfig *genai.ToolConfig
		if len(req.ToolChoice) > 0 {
			var err error
			toolConfig, err = parseToolChoice(req)
			if err != nil {
				errChan <- fmt.Errorf("parse tool_choice: %w", err)
				return
			}
		}

		config := buildConfig(systemText, req, tools, toolConfig)

		stream := p.client.Models.GenerateContentStream(ctx, model, contents, config)

		var streamUsage *api.Usage

		for resp, err := range stream {
			if err != nil {
				errChan <- fmt.Errorf("google stream error: %w", err)
				return
			}

			// Track usage from each response (last one has the final totals)
			if resp.UsageMetadata != nil {
				streamUsage = &api.Usage{
					InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
					OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
					TotalTokens:  int(resp.UsageMetadata.TotalTokenCount),
				}
			}

			if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
				for partIndex, part := range resp.Candidates[0].Content.Parts {
					if part != nil {
						// Handle text content
						if part.Text != "" {
							select {
							case deltaChan <- &api.ProviderStreamDelta{Text: part.Text}:
							case <-ctx.Done():
								errChan <- ctx.Err()
								return
							}
						}

						// Handle tool call content
						if part.FunctionCall != nil {
							delta := extractToolCallDelta(part, partIndex)
							if delta != nil {
								select {
								case deltaChan <- &api.ProviderStreamDelta{ToolCallDelta: delta}:
								case <-ctx.Done():
									errChan <- ctx.Err()
									return
								}
							}
						}
					}
				}
			}
		}

		select {
		case deltaChan <- &api.ProviderStreamDelta{Done: true, Usage: streamUsage}:
		case <-ctx.Done():
			errChan <- ctx.Err()
		}
	}()

	return deltaChan, errChan
}

// convertMessages splits messages into Gemini contents and system text.
func convertMessages(messages []api.Message) ([]*genai.Content, string) {
	var contents []*genai.Content
	var systemText string

	// Build a map of CallID -> Name from assistant tool calls
	// This allows us to look up function names when processing tool results
	callIDToName := make(map[string]string)
	for _, msg := range messages {
		if msg.Role == "assistant" || msg.Role == "model" {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" && tc.Name != "" {
					callIDToName[tc.ID] = tc.Name
				}
			}
		}
	}

	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "developer" {
			for _, block := range msg.Content {
				if block.Type == "input_text" || block.Type == "output_text" {
					systemText += block.Text
				}
			}
			continue
		}

		if msg.Role == "tool" {
			// Tool results are sent as FunctionResponse in user role message
			var output string
			for _, block := range msg.Content {
				if block.Type == "input_text" || block.Type == "output_text" {
					output += block.Text
				}
			}

			// Parse output as JSON map, or wrap in {"output": "..."} if not JSON
			var responseMap map[string]any
			if err := json.Unmarshal([]byte(output), &responseMap); err != nil {
				// Not JSON, wrap it
				responseMap = map[string]any{"output": output}
			}

			// Get function name from message or look it up from CallID
			name := msg.Name
			if name == "" && msg.CallID != "" {
				name = callIDToName[msg.CallID]
			}

			// Create FunctionResponse part with CallID and Name from message
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:       msg.CallID,
					Name:     name, // Name is required by Google
					Response: responseMap,
				},
			}

			// Add to user role message
			contents = append(contents, &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{part},
			})
			continue
		}

		var parts []*genai.Part
		for _, block := range msg.Content {
			if block.Type == "input_text" || block.Type == "output_text" {
				parts = append(parts, genai.NewPartFromText(block.Text))
			}
		}

		// Add tool calls for assistant messages
		if msg.Role == "assistant" || msg.Role == "model" {
			for _, tc := range msg.ToolCalls {
				// Parse arguments JSON into map
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
					// If unmarshal fails, skip this tool call
					continue
				}

				// Create FunctionCall part
				parts = append(parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   tc.ID,
						Name: tc.Name,
						Args: args,
					},
				})
			}
		}

		role := "user"
		if msg.Role == "assistant" || msg.Role == "model" {
			role = "model"
		}

		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}

	return contents, systemText
}

// buildConfig constructs a GenerateContentConfig from system text and request params.
func buildConfig(systemText string, req *api.ResponseRequest, tools []*genai.Tool, toolConfig *genai.ToolConfig) *genai.GenerateContentConfig {
	var cfg *genai.GenerateContentConfig

	needsCfg := systemText != "" || req.MaxOutputTokens != nil || req.Temperature != nil || req.TopP != nil || tools != nil || toolConfig != nil
	if !needsCfg {
		return nil
	}

	cfg = &genai.GenerateContentConfig{}

	if systemText != "" {
		cfg.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText(systemText)},
		}
	}

	if req.MaxOutputTokens != nil {
		cfg.MaxOutputTokens = clampMaxOutputTokens(*req.MaxOutputTokens)
	}

	if req.Temperature != nil {
		t := float32(*req.Temperature)
		cfg.Temperature = &t
	}

	if req.TopP != nil {
		tp := float32(*req.TopP)
		cfg.TopP = &tp
	}

	if tools != nil {
		cfg.Tools = tools
	}

	if toolConfig != nil {
		cfg.ToolConfig = toolConfig
	}

	return cfg
}

func chooseModel(requested, defaultModel string) string {
	if requested != "" {
		return requested
	}
	if defaultModel != "" {
		return defaultModel
	}
	return "gemini-2.0-flash-exp"
}

const maxInt32Value = int(^uint32(0) >> 1)

func clampMaxOutputTokens(value int) int32 {
	switch {
	case value <= 0:
		return 0
	case value > maxInt32Value:
		return int32(maxInt32Value)
	default:
		return int32(value)
	}
}
