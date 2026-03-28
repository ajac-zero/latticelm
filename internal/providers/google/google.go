package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

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

	contents, systemInstruction, err := convertMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("convert messages: %w", err)
	}

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

	config := buildConfig(systemInstruction, req, tools, toolConfig)

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

		contents, systemInstruction, err := convertMessages(messages)
		if err != nil {
			errChan <- fmt.Errorf("convert messages: %w", err)
			return
		}

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

		config := buildConfig(systemInstruction, req, tools, toolConfig)

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

// convertMessages splits messages into Gemini contents and system instructions.
func convertMessages(messages []api.Message) ([]*genai.Content, *genai.Content, error) {
	var contents []*genai.Content
	var systemParts []*genai.Part

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
			parts, err := buildGeminiTextParts(msg.Content, msg.Role)
			if err != nil {
				return nil, nil, err
			}
			systemParts = append(systemParts, parts...)
			continue
		}

		if msg.Role == "tool" {
			responseMap, err := buildGeminiToolResponse(msg.Content)
			if err != nil {
				return nil, nil, err
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

		parts, err := buildGeminiTextParts(msg.Content, msg.Role)
		if err != nil {
			return nil, nil, err
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

	var systemInstruction *genai.Content
	if len(systemParts) > 0 {
		systemInstruction = &genai.Content{Parts: systemParts}
	}

	return contents, systemInstruction, nil
}

// buildConfig constructs a GenerateContentConfig from system instructions and request params.
func buildConfig(systemInstruction *genai.Content, req *api.ResponseRequest, tools []*genai.Tool, toolConfig *genai.ToolConfig) *genai.GenerateContentConfig {
	var cfg *genai.GenerateContentConfig

	needsCfg := systemInstruction != nil || req.MaxOutputTokens != nil || req.Temperature != nil || req.TopP != nil || tools != nil || toolConfig != nil
	if !needsCfg {
		return nil
	}

	cfg = &genai.GenerateContentConfig{}

	if systemInstruction != nil {
		cfg.SystemInstruction = systemInstruction
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

func buildGeminiTextParts(blocks []api.ContentBlock, role string) ([]*genai.Part, error) {
	parts := make([]*genai.Part, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "text", "input_text", "output_text":
			parts = append(parts, genai.NewPartFromText(block.Text))
		case "input_image":
			imgPart, err := buildGeminiImagePart(block)
			if err != nil {
				return nil, fmt.Errorf("build image part: %w", err)
			}
			parts = append(parts, imgPart)
		case "input_file":
			filePart, err := buildGeminiFilePart(block)
			if err != nil {
				return nil, fmt.Errorf("build file part: %w", err)
			}
			parts = append(parts, filePart)
		case "input_video":
			videoPart, err := buildGeminiVideoPart(block)
			if err != nil {
				return nil, fmt.Errorf("build video part: %w", err)
			}
			parts = append(parts, videoPart)
		case "encrypted_reasoning":
			continue
		default:
			return nil, fmt.Errorf("%s messages do not support %q content in the Google provider", role, block.Type)
		}
	}
	return parts, nil
}

func buildGeminiImagePart(block api.ContentBlock) (*genai.Part, error) {
	if block.ImageURL != "" {
		// Handle data URL or regular URL
		if strings.HasPrefix(block.ImageURL, "data:") {
			mediaType, data, err := parseDataURL(block.ImageURL)
			if err != nil {
				return nil, fmt.Errorf("parse image data URL: %w", err)
			}
			decodedData, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return nil, fmt.Errorf("decode base64 image: %w", err)
			}
			return genai.NewPartFromBytes(decodedData, mediaType), nil
		}
		// Regular URL - use URI-based part
		mimeType := guessImageMimeType(block.ImageURL)
		return genai.NewPartFromURI(block.ImageURL, mimeType), nil
	}
	return nil, fmt.Errorf("image input requires image_url field")
}

func buildGeminiFilePart(block api.ContentBlock) (*genai.Part, error) {
	if block.FileURL != "" {
		// URL-based file
		mimeType := guessFileMimeType(block.FileURL)
		return genai.NewPartFromURI(block.FileURL, mimeType), nil
	}
	if block.FileData != "" {
		// Base64-encoded file data
		mediaType, data, err := parseBase64Payload(block.FileData, guessFileMimeType(block.Filename))
		if err != nil {
			return nil, fmt.Errorf("parse file data: %w", err)
		}
		decodedData, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("decode base64 file: %w", err)
		}
		return genai.NewPartFromBytes(decodedData, mediaType), nil
	}
	return nil, fmt.Errorf("file input requires file_url or file_data field")
}

func buildGeminiVideoPart(block api.ContentBlock) (*genai.Part, error) {
	if block.VideoURL != "" {
		if strings.HasPrefix(block.VideoURL, "data:") {
			mediaType, data, err := parseDataURL(block.VideoURL)
			if err != nil {
				return nil, fmt.Errorf("parse video data URL: %w", err)
			}
			decodedData, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return nil, fmt.Errorf("decode base64 video: %w", err)
			}
			return genai.NewPartFromBytes(decodedData, mediaType), nil
		}
		// URL-based video
		mimeType := guessVideoMimeType(block.VideoURL)
		return genai.NewPartFromURI(block.VideoURL, mimeType), nil
	}
	return nil, fmt.Errorf("video input requires video_url field")
}

// guessImageMimeType attempts to infer MIME type from URL extension
func guessImageMimeType(location string) string {
	location = normalizeMediaLocation(location)
	switch {
	case strings.HasSuffix(strings.ToLower(location), ".png"):
		return "image/png"
	case strings.HasSuffix(strings.ToLower(location), ".gif"):
		return "image/gif"
	case strings.HasSuffix(strings.ToLower(location), ".webp"):
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// guessFileMimeType attempts to infer MIME type for files
func guessFileMimeType(location string) string {
	location = normalizeMediaLocation(location)
	switch {
	case strings.HasSuffix(strings.ToLower(location), ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(strings.ToLower(location), ".txt"),
		strings.HasSuffix(strings.ToLower(location), ".md"),
		strings.HasSuffix(strings.ToLower(location), ".csv"),
		strings.HasSuffix(strings.ToLower(location), ".xml"),
		strings.HasSuffix(strings.ToLower(location), ".yaml"),
		strings.HasSuffix(strings.ToLower(location), ".yml"):
		return "text/plain"
	case strings.HasSuffix(strings.ToLower(location), ".json"):
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

// guessVideoMimeType attempts to infer MIME type for videos
func guessVideoMimeType(location string) string {
	location = normalizeMediaLocation(location)
	switch {
	case strings.HasSuffix(strings.ToLower(location), ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(strings.ToLower(location), ".webm"):
		return "video/webm"
	case strings.HasSuffix(strings.ToLower(location), ".mov"):
		return "video/quicktime"
	default:
		return "video/mp4"
	}
}

func normalizeMediaLocation(location string) string {
	if location == "" {
		return ""
	}
	if parsed, err := url.Parse(location); err == nil && parsed.Path != "" {
		return parsed.Path
	}
	return location
}

func buildGeminiToolResponse(blocks []api.ContentBlock) (map[string]any, error) {
	if len(blocks) == 1 {
		if text, ok := blocks[0].TextValue(); ok && blocks[0].Type != "refusal" {
			var responseMap map[string]any
			if err := json.Unmarshal([]byte(text), &responseMap); err == nil {
				return responseMap, nil
			}
			return map[string]any{"output": text}, nil
		}
	}

	content := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "encrypted_reasoning" {
			continue
		}
		text, ok := block.TextValue()
		if !ok {
			return nil, fmt.Errorf("tool results only support text content in the Google provider; found %q", block.Type)
		}
		entry := map[string]any{"type": block.Type}
		if block.Type == "refusal" {
			entry["refusal"] = text
		} else {
			entry["text"] = text
		}
		content = append(content, entry)
	}
	return map[string]any{"content": content}, nil
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
