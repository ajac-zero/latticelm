package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/logger"
	"github.com/ajac-zero/latticelm/internal/providers"
)

// persistConversation writes the completed conversation to the store.
// If result produced an assistant message, it is appended to storeMsgs before saving.
// Errors are logged but never returned — storage failures must not fail the response.
func (s *GatewayServer) persistConversation(ctx context.Context, responseID, model, providerName string, storeMsgs []api.Message, result *api.ProviderResult, outputItems []api.OutputItem, origReq *api.ResponseRequest, principal *auth.Principal) {
	assistantMsg := buildAssistantMessage(result)
	var allMsgs []api.Message
	var replayState *api.ReplayState
	if assistantMsg != nil {
		allMsgs = append(storeMsgs, *assistantMsg)
		replayState = buildReplayState(providerName, result, outputItems, len(allMsgs)-1)
	} else {
		allMsgs = storeMsgs
		replayState = buildReplayState(providerName, result, outputItems, -1)
	}
	if _, err := s.convs.Create(ctx, responseID, model, allMsgs, ownerFromPrincipal(principal), origReq, replayState); err != nil {
		s.logger.ErrorContext(ctx, "failed to store conversation",
			logger.LogAttrsWithTrace(ctx,
				slog.String("request_id", logger.FromContext(ctx)),
				slog.String("response_id", responseID),
				slog.String("error", err.Error()),
			)...,
		)
	}
}

func validateToolCallName(req *api.ResponseRequest, name string) *api.ResponseError {
	if req == nil {
		return nil
	}
	toolChoice, err := req.ParseToolChoice()
	if err != nil {
		return &api.ResponseError{Type: "invalid_request", Message: err.Error()}
	}
	if toolChoice.Mode == "none" {
		return &api.ResponseError{Type: "model_error", Message: "model emitted a tool call while tool_choice was none"}
	}
	if name == "" {
		return nil
	}
	declared, err := req.DeclaredToolNames()
	if err != nil {
		return &api.ResponseError{Type: "invalid_request", Message: err.Error()}
	}
	if len(declared) == 0 {
		return &api.ResponseError{Type: "model_error", Message: fmt.Sprintf("model emitted undeclared tool %q", name)}
	}
	if _, ok := declared[name]; !ok {
		return &api.ResponseError{Type: "model_error", Message: fmt.Sprintf("model emitted undeclared tool %q", name)}
	}
	switch toolChoice.Mode {
	case "function":
		if name != toolChoice.RequiredToolName {
			return &api.ResponseError{Type: "model_error", Message: fmt.Sprintf("model emitted tool %q but tool_choice required %q", name, toolChoice.RequiredToolName)}
		}
	case "allowed_tools":
		if _, ok := toolChoice.AllowedTools[name]; !ok {
			return &api.ResponseError{Type: "model_error", Message: fmt.Sprintf("model emitted disallowed tool %q", name)}
		}
	}
	return nil
}

func validateToolCalls(req *api.ResponseRequest, toolCalls []api.ToolCall) *api.ResponseError {
	if req == nil {
		return nil
	}
	for _, toolCall := range toolCalls {
		if respErr := validateToolCallName(req, toolCall.Name); respErr != nil {
			return respErr
		}
	}
	toolChoice, err := req.ParseToolChoice()
	if err != nil {
		return &api.ResponseError{Type: "invalid_request", Message: err.Error()}
	}
	if len(toolCalls) == 0 && (toolChoice.Mode == "required" || toolChoice.Mode == "any" || toolChoice.Mode == "function") {
		msg := "model did not emit a required tool call"
		if toolChoice.Mode == "function" {
			msg = fmt.Sprintf("model did not emit required tool %q", toolChoice.RequiredToolName)
		}
		return &api.ResponseError{Type: "model_error", Message: msg}
	}
	return nil
}

func (s *GatewayServer) resolveProvider(req *api.ResponseRequest) (providers.Provider, error) {
	if req.Provider != "" {
		if provider, ok := s.registry.Get(req.Provider); ok {
			return provider, nil
		}
		return nil, fmt.Errorf("provider %s not configured", req.Provider)
	}
	return s.registry.Default(req.Model)
}

// resolveItemReferences resolves item_reference IDs to their corresponding messages
// from the conversation history. Item references allow clients to refer to specific
// output items from previous responses without resending the full content.
func (s *GatewayServer) resolveItemReferences(refIDs []string, history []api.Message, replay *api.ReplayState, providerName string) []api.Message {
	if replay == nil || len(refIDs) == 0 {
		return nil
	}

	refs := make(map[string]api.ReplayItem, len(replay.Items))
	for _, item := range replay.Items {
		refs[item.ID] = item
	}

	resolved := make([]api.Message, 0, len(refIDs))
	seenIndexes := make(map[int]struct{}, len(refIDs))
	for _, refID := range refIDs {
		item, ok := refs[refID]
		if !ok || item.MessageIndex < 0 || item.MessageIndex >= len(history) {
			continue
		}
		if _, alreadySeen := seenIndexes[item.MessageIndex]; alreadySeen {
			continue
		}
		seenIndexes[item.MessageIndex] = struct{}{}

		if replay.Provider == providerName && item.Message != nil {
			resolved = append(resolved, cloneMessage(*item.Message))
			continue
		}
		resolved = append(resolved, cloneMessage(history[item.MessageIndex]))
	}

	return resolved
}

func cloneMessages(messages []api.Message) []api.Message {
	out := make([]api.Message, len(messages))
	for i, msg := range messages {
		out[i] = cloneMessage(msg)
	}
	return out
}

func cloneMessage(msg api.Message) api.Message {
	return api.Message{
		Role:      msg.Role,
		CallID:    msg.CallID,
		Name:      msg.Name,
		Content:   append([]api.ContentBlock(nil), msg.Content...),
		ToolCalls: cloneToolCalls(msg.ToolCalls),
	}
}

func cloneToolCalls(toolCalls []api.ToolCall) []api.ToolCall {
	return append([]api.ToolCall(nil), toolCalls...)
}

func ownerFromPrincipal(p *auth.Principal) conversation.OwnerInfo {
	if p == nil {
		return conversation.OwnerInfo{}
	}
	return conversation.OwnerInfo{
		OwnerIss: p.Issuer,
		OwnerSub: p.Subject,
		TenantID: p.TenantID,
	}
}

func tenantFromPrincipal(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	return p.TenantID
}

func subFromPrincipal(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	return p.Subject
}

func generateID(prefix string) string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	return prefix + id[:24]
}
