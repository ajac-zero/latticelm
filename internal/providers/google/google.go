package google

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/genai"

	"github.com/yourusername/go-llm-gateway/internal/api"
	"github.com/yourusername/go-llm-gateway/internal/config"
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

// Generate routes the Open Responses request to Gemini.
func (p *Provider) Generate(ctx context.Context, req *api.ResponseRequest) (*api.Response, error) {
	if p.cfg.APIKey == "" {
		return nil, fmt.Errorf("google api key missing")
	}
	if p.client == nil {
		return nil, fmt.Errorf("google client not initialized")
	}

	model := chooseModel(req.Model, p.cfg.Model)

	// Convert Open Responses messages to Gemini format
	var contents []*genai.Content
	var systemText string
	
	for _, msg := range req.Input {
		if msg.Role == "system" {
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

	// Build config with system instruction if present
	var config *genai.GenerateContentConfig
	if systemText != "" {
		config = &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromText(systemText)},
			},
		}
	}

	// Generate content
	resp, err := p.client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("google api error: %w", err)
	}

	// Convert Gemini response to Open Responses format
	output := make([]api.Message, 0, 1)
	var text string
	
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part != nil {
				text += part.Text
			}
		}
	}
	
	output = append(output, api.Message{
		Role: "assistant",
		Content: []api.ContentBlock{
			{Type: "output_text", Text: text},
		},
	})

	// Extract usage info if available
	var inputTokens, outputTokens int
	if resp.UsageMetadata != nil {
		inputTokens = int(resp.UsageMetadata.PromptTokenCount)
		outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return &api.Response{
		ID:       uuid.NewString(),
		Object:   "response",
		Created:  time.Now().Unix(),
		Model:    model,
		Provider: Name,
		Output:   output,
		Usage: api.Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  inputTokens + outputTokens,
		},
	}, nil
}

// GenerateStream handles streaming requests to Google.
func (p *Provider) GenerateStream(ctx context.Context, req *api.ResponseRequest) (<-chan *api.StreamChunk, <-chan error) {
	chunkChan := make(chan *api.StreamChunk)
	errChan := make(chan error, 1)

	go func() {
		defer close(chunkChan)
		defer close(errChan)

		if p.cfg.APIKey == "" {
			errChan <- fmt.Errorf("google api key missing")
			return
		}
		if p.client == nil {
			errChan <- fmt.Errorf("google client not initialized")
			return
		}

		model := chooseModel(req.Model, p.cfg.Model)

		// Convert messages
		var contents []*genai.Content
		var systemText string
		
		for _, msg := range req.Input {
			if msg.Role == "system" {
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

		// Build config with system instruction if present
		var config *genai.GenerateContentConfig
		if systemText != "" {
			config = &genai.GenerateContentConfig{
				SystemInstruction: &genai.Content{
					Parts: []*genai.Part{genai.NewPartFromText(systemText)},
				},
			}
		}

		// Create stream
		stream := p.client.Models.GenerateContentStream(ctx, model, contents, config)

		// Process stream
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

			delta := &api.StreamDelta{}
			if text != "" {
				delta.Content = []api.ContentBlock{
					{Type: "output_text", Text: text},
				}
			}

			streamChunk := &api.StreamChunk{
				Object:   "response.chunk",
				Created:  time.Now().Unix(),
				Model:    model,
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
	return "gemini-2.0-flash-exp"
}
