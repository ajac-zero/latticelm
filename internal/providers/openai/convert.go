package openai

import (
	"encoding/json"
	"fmt"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

// maxToolCallIDLen is the maximum tool call ID length accepted by OpenAI / Azure OpenAI.
const maxToolCallIDLen = 40

// sanitizeToolCallID truncates a tool call ID to maxToolCallIDLen.
// This is a local concern of the OpenAI wire format; other providers are unaffected.
func sanitizeToolCallID(id string) string {
	if len(id) > maxToolCallIDLen {
		return id[:maxToolCallIDLen]
	}
	return id
}

// buildOAIMessages converts internal messages to OpenAI chat completion format.
// Any tool call IDs longer than maxToolCallIDLen are truncated consistently across
// assistant tool_calls and the corresponding tool result messages so that the
// conversation remains coherent when sent to OpenAI / Azure OpenAI.
func buildOAIMessages(messages []api.Message) []openai.ChatCompletionMessageParamUnion {
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
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, len(msg.ToolCalls))
				for i, tc := range msg.ToolCalls {
					toolCalls[i] = openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: sanitizeToolCallID(tc.ID),
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
			oaiMessages = append(oaiMessages, openai.ToolMessage(content, sanitizeToolCallID(msg.CallID)))
		}
	}
	return oaiMessages
}

// parseTools converts Open Responses tools to OpenAI format
func parseTools(req *api.ResponseRequest) ([]openai.ChatCompletionToolUnionParam, error) {
	if len(req.Tools) == 0 {
		return nil, nil
	}

	var toolDefs []map[string]interface{}
	if err := json.Unmarshal(req.Tools, &toolDefs); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}

	var tools []openai.ChatCompletionToolUnionParam
	for _, td := range toolDefs {
		// Convert Open Responses tool to OpenAI ChatCompletionFunctionToolParam
		// Extract: name, description, parameters
		name, _ := td["name"].(string)
		desc, _ := td["description"].(string)
		params, _ := td["parameters"].(map[string]interface{})

		funcDef := shared.FunctionDefinitionParam{
			Name: name,
		}

		if desc != "" {
			funcDef.Description = openai.String(desc)
		}

		if params != nil {
			funcDef.Parameters = shared.FunctionParameters(params)
		}

		tools = append(tools, openai.ChatCompletionFunctionTool(funcDef))
	}

	return tools, nil
}

// parseToolChoice converts Open Responses tool_choice to OpenAI format
func parseToolChoice(req *api.ResponseRequest) (openai.ChatCompletionToolChoiceOptionUnionParam, error) {
	var result openai.ChatCompletionToolChoiceOptionUnionParam

	if len(req.ToolChoice) == 0 {
		return result, nil
	}

	var choice interface{}
	if err := json.Unmarshal(req.ToolChoice, &choice); err != nil {
		return result, fmt.Errorf("unmarshal tool_choice: %w", err)
	}

	// Handle string values: "auto", "none", "required"
	if str, ok := choice.(string); ok {
		result.OfAuto = openai.String(str)
		return result, nil
	}

	// Handle specific function selection: {"type": "function", "function": {"name": "..."}}
	if obj, ok := choice.(map[string]interface{}); ok {
		funcObj, _ := obj["function"].(map[string]interface{})
		name, _ := funcObj["name"].(string)

		return openai.ToolChoiceOptionFunctionToolChoice(
			openai.ChatCompletionNamedToolChoiceFunctionParam{
				Name: name,
			},
		), nil
	}

	return result, fmt.Errorf("invalid tool_choice format")
}

// extractToolCalls converts OpenAI tool calls to api.ToolCall
func extractToolCalls(message openai.ChatCompletionMessage) []api.ToolCall {
	if len(message.ToolCalls) == 0 {
		return nil
	}

	var toolCalls []api.ToolCall
	for _, tc := range message.ToolCalls {
		toolCalls = append(toolCalls, api.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return toolCalls
}

// extractToolCallDelta extracts tool call delta from streaming chunk choice
func extractToolCallDelta(choice openai.ChatCompletionChunkChoice) *api.ToolCallDelta {
	if len(choice.Delta.ToolCalls) == 0 {
		return nil
	}

	// OpenAI sends tool calls with index in the delta
	for _, tc := range choice.Delta.ToolCalls {
		return &api.ToolCallDelta{
			Index:     int(tc.Index),
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		}
	}

	return nil
}
