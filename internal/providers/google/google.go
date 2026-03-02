package google

import (
	"context"
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

// New constructs a Provider using the provided configuration.
func New(cfg config.ProviderConfig) *Provider {
	var client *genai.Client
	if cfg.APIKey != "" {
		var err error
		client, err = genai.NewClient(context.Background(), &genai.ClientConfig{
			APIKey: cfg.APIKey,
		})
		if err != nil {
			// Log error but don't fail construction - will fail on Generate
			fmt.Printf("warning: failed to create google client: %v\n", err)
		}
	}
	return &Provider{
		cfg:    cfg,
		client: client,
	}
}

func (p *Provider) Name() string { return Name }

// Generate routes the request to Gemini and returns a ProviderResult.
func (p *Provider) Generate(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("google api key missing")
	}
	if p.client == nil {
		return nil, fmt.Errorf("google client not initialized")
	}

	model := req.Model

	contents, systemText := convertMessages(messages)

	config := buildConfig(systemText, req)

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

	var inputTokens, outputTokens int
	if resp.UsageMetadata != nil {
		inputTokens = int(resp.UsageMetadata.PromptTokenCount)
		outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return &api.ProviderResult{
		ID:    uuid.NewString(),
		Model: model,
		Text:  text,
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

		if p.cfg.APIKey == "" {
			errChan <- fmt.Errorf("google api key missing")
			return
		}
		if p.client == nil {
			errChan <- fmt.Errorf("google client not initialized")
			return
		}

		model := req.Model

		contents, systemText := convertMessages(messages)

		config := buildConfig(systemText, req)

		stream := p.client.Models.GenerateContentStream(ctx, model, contents, config)

		for resp, err := range stream {
			if err != nil {
				errChan <- fmt.Errorf("google stream error: %w", err)
				return
			}

			var text string
			if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
				for _, part := range resp.Candidates[0].Content.Parts {
					if part != nil {
						text += part.Text
					}
				}
			}

			if text != "" {
				select {
				case deltaChan <- &api.ProviderStreamDelta{Text: text}:
				case <-ctx.Done():
					errChan <- ctx.Err()
					return
				}
			}
		}

		select {
		case deltaChan <- &api.ProviderStreamDelta{Done: true}:
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

	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "developer" {
			for _, block := range msg.Content {
				if block.Type == "input_text" || block.Type == "output_text" {
					systemText += block.Text
				}
			}
			continue
		}

		var parts []*genai.Part
		for _, block := range msg.Content {
			if block.Type == "input_text" || block.Type == "output_text" {
				parts = append(parts, genai.NewPartFromText(block.Text))
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
func buildConfig(systemText string, req *api.ResponseRequest) *genai.GenerateContentConfig {
	var cfg *genai.GenerateContentConfig

	needsCfg := systemText != "" || req.MaxOutputTokens != nil || req.Temperature != nil || req.TopP != nil
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
		cfg.MaxOutputTokens = int32(*req.MaxOutputTokens)
	}

	if req.Temperature != nil {
		t := float32(*req.Temperature)
		cfg.Temperature = &t
	}

	if req.TopP != nil {
		tp := float32(*req.TopP)
		cfg.TopP = &tp
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
