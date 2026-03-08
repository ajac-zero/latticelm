package openai

import (
	"encoding/json"
	"fmt"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

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
