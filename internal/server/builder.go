package server

import (
	"encoding/json"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/conversation"
)

// responseDefaults holds request parameters resolved to their spec defaults.
type responseDefaults struct {
	tools             json.RawMessage
	toolChoice        json.RawMessage
	text              json.RawMessage
	truncation        string
	temperature       float64
	topP              float64
	presencePenalty   float64
	frequencyPenalty  float64
	topLogprobs       int
	parallelToolCalls bool
	serviceTier       string
	reasoning         json.RawMessage
	background        bool
	metadata          map[string]string
}

// resolveDefaults applies spec defaults and overlays any non-nil values from req.
// Safe to call with a nil req.
func resolveDefaults(req *api.ResponseRequest) responseDefaults {
	d := responseDefaults{
		tools:             json.RawMessage(`[]`),
		toolChoice:        json.RawMessage(`"auto"`),
		text:              json.RawMessage(`{"format":{"type":"text"}}`),
		truncation:        "disabled",
		temperature:       1.0,
		topP:              1.0,
		parallelToolCalls: true,
		serviceTier:       "default",
		metadata:          map[string]string{},
	}
	if req == nil {
		return d
	}
	if req.Tools != nil {
		d.tools = req.Tools
	}
	if req.ToolChoice != nil {
		d.toolChoice = req.ToolChoice
	}
	if req.Text != nil {
		d.text = req.Text
	}
	if req.Truncation != nil {
		d.truncation = *req.Truncation
	}
	if req.Temperature != nil {
		d.temperature = *req.Temperature
	}
	if req.TopP != nil {
		d.topP = *req.TopP
	}
	if req.PresencePenalty != nil {
		d.presencePenalty = *req.PresencePenalty
	}
	if req.FrequencyPenalty != nil {
		d.frequencyPenalty = *req.FrequencyPenalty
	}
	if req.TopLogprobs != nil {
		d.topLogprobs = *req.TopLogprobs
	}
	if req.ParallelToolCalls != nil {
		d.parallelToolCalls = *req.ParallelToolCalls
	}
	if req.ServiceTier != nil {
		d.serviceTier = *req.ServiceTier
	}
	if req.Reasoning != nil {
		d.reasoning = req.Reasoning
	}
	if req.Background != nil {
		d.background = *req.Background
	}
	if req.Metadata != nil {
		d.metadata = req.Metadata
	}
	return d
}

func (s *GatewayServer) buildResponseFromConversation(conv *conversation.Conversation) *api.Response {
	// Build output items from the last assistant message
	var outputItems []api.OutputItem
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		msg := conv.Messages[i]
		if msg.Role == "assistant" {
			outputItems = buildOutputItemsFromMessage(msg)
			break
		}
	}

	d := resolveDefaults(conv.Request)

	return &api.Response{
		ID:                 conv.ID,
		Object:             "response",
		CreatedAt:          conv.CreatedAt.Unix(),
		CompletedAt:        ptrInt64(conv.UpdatedAt.Unix()),
		Status:             "completed",
		Model:              conv.Model,
		PreviousResponseID: previousResponseIDFromRequest(conv.Request),
		Instructions:       instructionsFromRequest(conv.Request),
		Output:             outputItems,
		Tools:              d.tools,
		ToolChoice:         d.toolChoice,
		Truncation:         d.truncation,
		ParallelToolCalls:  d.parallelToolCalls,
		Text:               d.text,
		TopP:               d.topP,
		PresencePenalty:    d.presencePenalty,
		FrequencyPenalty:   d.frequencyPenalty,
		TopLogprobs:        d.topLogprobs,
		Temperature:        d.temperature,
		Reasoning:          d.reasoning,
		Usage:              nil, // Usage not stored in conversation
		MaxOutputTokens:    maxOutputTokensFromRequest(conv.Request),
		MaxToolCalls:       maxToolCallsFromRequest(conv.Request),
		Store:              true, // If we retrieved it, it was stored
		Background:         false,
		ServiceTier:        d.serviceTier,
		Metadata:           d.metadata,
	}
}

func buildOutputItemsFromMessage(msg api.Message) []api.OutputItem {
	var items []api.OutputItem

	// Add message item with content
	if len(msg.Content) > 0 {
		contentParts := make([]api.ContentPart, len(msg.Content))
		for i, block := range msg.Content {
			contentParts[i] = api.ContentPart{
				Type:        block.Type,
				Text:        block.Text,
				Annotations: []api.Annotation{},
			}
		}
		items = append(items, api.OutputItem{
			ID:      generateID("msg_"),
			Type:    "message",
			Status:  "completed",
			Role:    "assistant",
			Content: contentParts,
		})
	}

	// Add function_call items
	for _, tc := range msg.ToolCalls {
		items = append(items, api.OutputItem{
			ID:        generateID("item_"),
			Type:      "function_call",
			Status:    "completed",
			CallID:    tc.ID,
			Name:      tc.Name,
			Arguments: tc.Arguments,
		})
	}

	return items
}

func ptrInt64(v int64) *int64 {
	return &v
}

func previousResponseIDFromRequest(req *api.ResponseRequest) *string {
	if req == nil || req.PreviousResponseID == nil || *req.PreviousResponseID == "" {
		return nil
	}
	return req.PreviousResponseID
}

func instructionsFromRequest(req *api.ResponseRequest) *string {
	if req == nil || req.Instructions == nil || *req.Instructions == "" {
		return nil
	}
	return req.Instructions
}

func maxOutputTokensFromRequest(req *api.ResponseRequest) *int {
	if req == nil {
		return nil
	}
	return req.MaxOutputTokens
}

func maxToolCallsFromRequest(req *api.ResponseRequest) *int {
	if req == nil {
		return nil
	}
	return req.MaxToolCalls
}

func (s *GatewayServer) buildResponse(req *api.ResponseRequest, result *api.ProviderResult, providerName string, responseID string) *api.Response {
	return s.buildResponseWithOutput(req, result, providerName, responseID, buildOutputItems(result))
}

func (s *GatewayServer) buildResponseWithOutput(req *api.ResponseRequest, result *api.ProviderResult, providerName string, responseID string, outputItems []api.OutputItem) *api.Response {
	now := time.Now().Unix()

	model := result.Model
	if model == "" {
		model = req.Model
	}

	d := resolveDefaults(req)

	return &api.Response{
		ID:                 responseID,
		Object:             "response",
		CreatedAt:          now,
		CompletedAt:        &now,
		Status:             "completed",
		IncompleteDetails:  nil,
		Model:              model,
		PreviousResponseID: req.PreviousResponseID,
		Instructions:       req.Instructions,
		Output:             outputItems,
		Error:              nil,
		Tools:              d.tools,
		ToolChoice:         d.toolChoice,
		Truncation:         d.truncation,
		ParallelToolCalls:  d.parallelToolCalls,
		Text:               d.text,
		TopP:               d.topP,
		PresencePenalty:    d.presencePenalty,
		FrequencyPenalty:   d.frequencyPenalty,
		TopLogprobs:        d.topLogprobs,
		Temperature:        d.temperature,
		Reasoning:          d.reasoning,
		Usage:              &result.Usage,
		MaxOutputTokens:    req.MaxOutputTokens,
		MaxToolCalls:       req.MaxToolCalls,
		Store:              s.shouldStore(req),
		Background:         d.background,
		ServiceTier:        d.serviceTier,
		Metadata:           d.metadata,
		SafetyIdentifier:   nil,
		PromptCacheKey:     nil,
		Provider:           providerName,
	}
}

func buildOutputItems(result *api.ProviderResult) []api.OutputItem {
	outputItems := []api.OutputItem{}
	if result == nil {
		return outputItems
	}

	if result.Text != "" {
		outputItems = append(outputItems, api.OutputItem{
			ID:     generateID("msg_"),
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []api.ContentPart{{
				Type:        "output_text",
				Text:        result.Text,
				Annotations: []api.Annotation{},
			}},
		})
	}

	for _, tc := range result.ToolCalls {
		outputItems = append(outputItems, api.OutputItem{
			ID:        generateID("item_"),
			Type:      "function_call",
			Status:    "completed",
			CallID:    tc.ID,
			Name:      tc.Name,
			Arguments: tc.Arguments,
		})
	}

	return outputItems
}

func buildAssistantMessage(result *api.ProviderResult) *api.Message {
	if result == nil || (result.Text == "" && len(result.ToolCalls) == 0) {
		return nil
	}

	msg := &api.Message{
		Role:      "assistant",
		ToolCalls: cloneToolCalls(result.ToolCalls),
	}
	if result.Text != "" {
		msg.Content = []api.ContentBlock{{Type: "output_text", Text: result.Text}}
	}
	return msg
}

func buildReplayState(providerName string, result *api.ProviderResult, outputItems []api.OutputItem, messageIndex int) *api.ReplayState {
	if result == nil {
		return nil
	}

	state := &api.ReplayState{
		Provider:           providerName,
		ProviderResponseID: result.ID,
	}
	for _, item := range outputItems {
		if item.Type != "message" && item.Type != "function_call" {
			continue
		}
		replayItem := api.ReplayItem{
			ID:             item.ID,
			OutputItemType: item.Type,
			MessageIndex:   messageIndex,
		}
		if item.Type == "message" && result.ReplayMessage != nil {
			replayMsg := cloneMessage(*result.ReplayMessage)
			replayItem.Message = &replayMsg
		}
		state.Items = append(state.Items, replayItem)
	}

	if state.ProviderResponseID == "" && len(state.Items) == 0 {
		return nil
	}
	return state
}

func applyReplayState(messages []api.Message, replay *api.ReplayState, providerName string) []api.Message {
	out := cloneMessages(messages)
	if replay == nil || replay.Provider != providerName {
		return out
	}

	for _, item := range replay.Items {
		if item.Message == nil || item.MessageIndex < 0 || item.MessageIndex >= len(out) {
			continue
		}
		out[item.MessageIndex] = cloneMessage(*item.Message)
	}

	return out
}
