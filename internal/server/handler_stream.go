package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/logger"
	"github.com/ajac-zero/latticelm/internal/providers"
	"github.com/ajac-zero/latticelm/internal/ratelimit"
	"github.com/ajac-zero/latticelm/internal/usage"
)

type streamMessageBuilder struct {
	itemID       string
	outputIndex  int
	contentIndex int
	text         string
}

type streamToolCallBuilder struct {
	streamIndex int
	outputIndex int
	itemID      string
	id          string
	name        string
	arguments   string
}

func (s *GatewayServer) handleStreamingResponse(w http.ResponseWriter, r *http.Request, provider providers.Provider, providerMsgs []api.Message, resolvedReq *api.ResponseRequest, origReq *api.ResponseRequest, storeMsgs []api.Message) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteOpenResponsesError(w, s.logger, "streaming not supported", "server_error", http.StatusInternalServerError, nil, nil)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	responseID := generateID("resp_")
	seq := 0
	nextOutputIdx := 0

	initialResp := s.buildResponse(origReq, &api.ProviderResult{
		Model: origReq.Model,
	}, provider.Name(), responseID)
	initialResp.Status = "in_progress"
	initialResp.CompletedAt = nil
	initialResp.Output = []api.OutputItem{}
	initialResp.Usage = nil

	s.sendSSE(w, flusher, &seq, "response.in_progress", &api.StreamEvent{
		Type:     "response.in_progress",
		Response: initialResp,
	})

	var assistantMsg *streamMessageBuilder
	toolCallsInProgress := make(map[int]*streamToolCallBuilder)
	var streamErr error
	var streamRespErr *api.ResponseError
	var providerModel string
	var providerResultID string
	var streamUsage *api.Usage
	var replayMessage *api.Message

	ensureMessageStarted := func() {
		if assistantMsg != nil {
			return
		}
		assistantMsg = &streamMessageBuilder{
			itemID:       generateID("msg_"),
			outputIndex:  nextOutputIdx,
			contentIndex: 0,
		}
		nextOutputIdx++
		s.sendSSE(w, flusher, &seq, "response.output_item.added", &api.StreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: &assistantMsg.outputIndex,
			Item: &api.OutputItem{
				ID:      assistantMsg.itemID,
				Type:    "message",
				Status:  "in_progress",
				Role:    "assistant",
				Content: []api.ContentPart{},
			},
		})
		s.sendSSE(w, flusher, &seq, "response.content_part.added", &api.StreamEvent{
			Type:         "response.content_part.added",
			ItemID:       assistantMsg.itemID,
			OutputIndex:  &assistantMsg.outputIndex,
			ContentIndex: &assistantMsg.contentIndex,
			Part: &api.ContentPart{
				Type:        "output_text",
				Text:        "",
				Annotations: []api.Annotation{},
			},
		})
	}

	deltaChan, errChan := provider.GenerateStream(r.Context(), providerMsgs, resolvedReq)

loop:
	for {
		select {
		case delta, ok := <-deltaChan:
			if !ok {
				if err, ok := <-errChan; ok && err != nil {
					streamErr = err
				}
				break loop
			}
			if delta.Model != "" && providerModel == "" {
				providerModel = delta.Model
			}
			if delta.ID != "" && providerResultID == "" {
				providerResultID = delta.ID
			}

			// Handle text content
			if delta.Text != "" {
				ensureMessageStarted()
				assistantMsg.text += delta.Text
				s.sendSSE(w, flusher, &seq, "response.output_text.delta", &api.StreamEvent{
					Type:         "response.output_text.delta",
					ItemID:       assistantMsg.itemID,
					OutputIndex:  &assistantMsg.outputIndex,
					ContentIndex: &assistantMsg.contentIndex,
					Delta:        delta.Text,
				})
			}
			if delta.ToolCallDelta != nil {
				tc := delta.ToolCallDelta
				builder, exists := toolCallsInProgress[tc.Index]
				if !exists {
					builder = &streamToolCallBuilder{
						streamIndex: tc.Index,
						outputIndex: nextOutputIdx,
						itemID:      generateID("item_"),
						id:          tc.ID,
						name:        tc.Name,
					}
					nextOutputIdx++
					toolCallsInProgress[tc.Index] = builder
					if respErr := validateToolCallName(origReq, builder.name); respErr != nil {
						streamRespErr = respErr
						break loop
					}
					s.sendSSE(w, flusher, &seq, "response.output_item.added", &api.StreamEvent{
						Type:        "response.output_item.added",
						OutputIndex: &builder.outputIndex,
						Item: &api.OutputItem{
							ID:        builder.itemID,
							Type:      "function_call",
							Status:    "in_progress",
							CallID:    builder.id,
							Name:      builder.name,
							Arguments: "",
						},
					})
				}
				if builder.id == "" && tc.ID != "" {
					builder.id = tc.ID
				}
				if tc.Name != "" && builder.name == "" {
					builder.name = tc.Name
					if respErr := validateToolCallName(origReq, builder.name); respErr != nil {
						streamRespErr = respErr
						break loop
					}
				}
				if tc.Arguments != "" {
					builder.arguments += tc.Arguments
					s.sendSSE(w, flusher, &seq, "response.function_call_arguments.delta", &api.StreamEvent{
						Type:        "response.function_call_arguments.delta",
						ItemID:      builder.itemID,
						OutputIndex: &builder.outputIndex,
						Delta:       tc.Arguments,
					})
				}
			}
			if delta.Usage != nil {
				streamUsage = delta.Usage
			}
			if delta.ReplayMessage != nil {
				replayMessage = delta.ReplayMessage
			}
			if delta.Done {
				break loop
			}
		case err := <-errChan:
			if err != nil {
				streamErr = err
			}
			break loop
		case <-r.Context().Done():
			s.logger.InfoContext(r.Context(), "client disconnected",
				slog.String("request_id", logger.FromContext(r.Context())),
			)
			return
		}
	}

	if streamErr != nil && streamRespErr == nil {
		s.logger.ErrorContext(r.Context(), "stream error",
			logger.LogAttrsWithTrace(r.Context(),
				slog.String("request_id", logger.FromContext(r.Context())),
				slog.String("provider", provider.Name()),
				slog.String("model", origReq.Model),
				slog.String("error", streamErr.Error()),
			)...,
		)
		streamRespErr = &api.ResponseError{
			Type:    "model_error",
			Message: streamErr.Error(),
		}
		if isCircuitBreakerError(streamErr) {
			streamRespErr.Type = "server_error"
			streamRespErr.Message = "service temporarily unavailable - circuit breaker open"
		}
	}

	toolCalls := collectToolCalls(toolCallsInProgress)
	if streamRespErr == nil {
		streamRespErr = validateToolCalls(origReq, toolCalls)
	}

	if streamRespErr != nil {
		failedResp := s.buildResponse(origReq, &api.ProviderResult{
			Model: origReq.Model,
		}, provider.Name(), responseID)
		failedResp.Status = "failed"
		failedResp.CompletedAt = nil
		failedResp.Output = []api.OutputItem{}
		failedResp.Error = streamRespErr
		s.sendSSE(w, flusher, &seq, "response.failed", &api.StreamEvent{
			Type:     "response.failed",
			Response: failedResp,
		})
		s.sendSSEDone(w, flusher)
		return
	}

	if assistantMsg != nil {
		s.sendSSE(w, flusher, &seq, "response.output_text.done", &api.StreamEvent{
			Type:         "response.output_text.done",
			ItemID:       assistantMsg.itemID,
			OutputIndex:  &assistantMsg.outputIndex,
			ContentIndex: &assistantMsg.contentIndex,
			Text:         assistantMsg.text,
		})
		completedPart := &api.ContentPart{
			Type:        "output_text",
			Text:        assistantMsg.text,
			Annotations: []api.Annotation{},
		}
		s.sendSSE(w, flusher, &seq, "response.content_part.done", &api.StreamEvent{
			Type:         "response.content_part.done",
			ItemID:       assistantMsg.itemID,
			OutputIndex:  &assistantMsg.outputIndex,
			ContentIndex: &assistantMsg.contentIndex,
			Part:         completedPart,
		})
		s.sendSSE(w, flusher, &seq, "response.output_item.done", &api.StreamEvent{
			Type:        "response.output_item.done",
			OutputIndex: &assistantMsg.outputIndex,
			Item: &api.OutputItem{
				ID:      assistantMsg.itemID,
				Type:    "message",
				Status:  "completed",
				Role:    "assistant",
				Content: []api.ContentPart{*completedPart},
			},
		})
	}

	for _, builder := range sortedToolCallBuilders(toolCallsInProgress) {
		s.sendSSE(w, flusher, &seq, "response.function_call_arguments.done", &api.StreamEvent{
			Type:        "response.function_call_arguments.done",
			ItemID:      builder.itemID,
			OutputIndex: &builder.outputIndex,
			Arguments:   builder.arguments,
		})
		s.sendSSE(w, flusher, &seq, "response.output_item.done", &api.StreamEvent{
			Type:        "response.output_item.done",
			OutputIndex: &builder.outputIndex,
			Item: &api.OutputItem{
				ID:        builder.itemID,
				Type:      "function_call",
				Status:    "completed",
				CallID:    builder.id,
				Name:      builder.name,
				Arguments: builder.arguments,
			},
		})
	}

	model := origReq.Model
	if providerModel != "" {
		model = providerModel
	}
	finalResult := &api.ProviderResult{
		ID:            providerResultID,
		Model:         model,
		ToolCalls:     toolCalls,
		ReplayMessage: replayMessage,
	}
	if assistantMsg != nil {
		finalResult.Text = assistantMsg.text
	}
	if streamUsage != nil {
		finalResult.Usage = *streamUsage
	}
	outputItems := buildOrderedStreamOutput(assistantMsg, toolCallsInProgress)
	completedResp := s.buildResponseWithOutput(origReq, finalResult, provider.Name(), responseID, outputItems)
	s.sendSSE(w, flusher, &seq, "response.completed", &api.StreamEvent{
		Type:     "response.completed",
		Response: completedResp,
	})
	s.sendSSEDone(w, flusher)

	if s.shouldStore(origReq) && (finalResult.Text != "" || len(toolCalls) > 0) {
		s.persistConversation(r.Context(), responseID, model, provider.Name(), storeMsgs, finalResult, completedResp.Output, origReq, auth.PrincipalFromContext(r.Context()))
	}

	if completedResp.Usage != nil {
		ratelimit.RecordUsageFromContext(r.Context(), completedResp.Usage.InputTokens, completedResp.Usage.OutputTokens)
		principal := auth.PrincipalFromContext(r.Context())
		usage.RecordFromContext(r.Context(), usage.UsageEvent{
			TenantID:     tenantFromPrincipal(principal),
			UserSub:      subFromPrincipal(principal),
			Provider:     provider.Name(),
			Model:        model,
			InputTokens:  completedResp.Usage.InputTokens,
			OutputTokens: completedResp.Usage.OutputTokens,
			ResponseID:   responseID,
			Stream:       true,
		})
	}

	if finalResult.Text != "" || len(toolCalls) > 0 {
		s.logger.InfoContext(r.Context(), "streaming response completed",
			slog.String("request_id", logger.FromContext(r.Context())),
			slog.String("provider", provider.Name()),
			slog.String("model", model),
			slog.String("response_id", responseID),
			slog.Bool("has_tool_calls", len(toolCalls) > 0),
			slog.Bool("stored", s.shouldStore(origReq)),
		)
	}
}

func (s *GatewayServer) sendSSE(w http.ResponseWriter, flusher http.Flusher, seq *int, eventType string, event *api.StreamEvent) {
	event.SequenceNumber = *seq
	*seq++
	data, err := json.Marshal(event)
	if err != nil {
		s.logger.Error("failed to marshal SSE event",
			slog.String("event_type", eventType),
			slog.String("error", err.Error()),
		)
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
	flusher.Flush()
}

func (s *GatewayServer) sendSSEDone(w http.ResponseWriter, flusher http.Flusher) {
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func collectToolCalls(builders map[int]*streamToolCallBuilder) []api.ToolCall {
	sorted := sortedToolCallBuilders(builders)
	toolCalls := make([]api.ToolCall, 0, len(sorted))
	for _, builder := range sorted {
		toolCalls = append(toolCalls, api.ToolCall{
			ID:        builder.id,
			Name:      builder.name,
			Arguments: builder.arguments,
		})
	}
	return toolCalls
}

func sortedToolCallBuilders(builders map[int]*streamToolCallBuilder) []*streamToolCallBuilder {
	sorted := make([]*streamToolCallBuilder, 0, len(builders))
	for _, builder := range builders {
		sorted = append(sorted, builder)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].outputIndex < sorted[j].outputIndex
	})
	return sorted
}

func buildOrderedStreamOutput(message *streamMessageBuilder, toolCalls map[int]*streamToolCallBuilder) []api.OutputItem {
	type orderedItem struct {
		index int
		item  api.OutputItem
	}
	items := make([]orderedItem, 0, len(toolCalls)+1)
	if message != nil {
		items = append(items, orderedItem{
			index: message.outputIndex,
			item: api.OutputItem{
				ID:     message.itemID,
				Type:   "message",
				Status: "completed",
				Role:   "assistant",
				Content: []api.ContentPart{{
					Type:        "output_text",
					Text:        message.text,
					Annotations: []api.Annotation{},
				}},
			},
		})
	}
	for _, builder := range toolCalls {
		items = append(items, orderedItem{
			index: builder.outputIndex,
			item: api.OutputItem{
				ID:        builder.itemID,
				Type:      "function_call",
				Status:    "completed",
				CallID:    builder.id,
				Name:      builder.name,
				Arguments: builder.arguments,
			},
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].index < items[j].index
	})
	output := make([]api.OutputItem, 0, len(items))
	for _, item := range items {
		output = append(output, item.item)
	}
	return output
}
