package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/anthropics/anthropic-sdk-go"
)

// parseTools converts Open Responses tools to Anthropic format
func parseTools(req *api.ResponseRequest) ([]anthropic.ToolUnionParam, error) {
	if len(req.Tools) == 0 {
		return nil, nil
	}

	var toolDefs []map[string]interface{}
	if err := json.Unmarshal(req.Tools, &toolDefs); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}

	var tools []anthropic.ToolUnionParam
	for _, td := range toolDefs {
		// Extract: name, description, parameters
		// Note: Anthropic uses "input_schema" instead of "parameters"
		name, _ := td["name"].(string)
		desc, _ := td["description"].(string)
		params, _ := td["parameters"].(map[string]interface{})

		inputSchema := anthropic.ToolInputSchemaParam{
			Type:       "object",
			Properties: params["properties"],
		}

		// Add required fields if present
		if required, ok := params["required"].([]interface{}); ok {
			requiredStrs := make([]string, 0, len(required))
			for _, r := range required {
				if str, ok := r.(string); ok {
					requiredStrs = append(requiredStrs, str)
				}
			}
			inputSchema.Required = requiredStrs
		}

		// Create the tool using ToolUnionParamOfTool
		tool := anthropic.ToolUnionParamOfTool(inputSchema, name)

		if desc != "" {
			tool.OfTool.Description = anthropic.String(desc)
		}

		tools = append(tools, tool)
	}

	return tools, nil
}

// parseToolChoice converts Open Responses tool_choice to Anthropic format
func parseToolChoice(req *api.ResponseRequest) (anthropic.ToolChoiceUnionParam, error) {
	var result anthropic.ToolChoiceUnionParam

	if len(req.ToolChoice) == 0 {
		return result, nil
	}

	var choice interface{}
	if err := json.Unmarshal(req.ToolChoice, &choice); err != nil {
		return result, fmt.Errorf("unmarshal tool_choice: %w", err)
	}

	// Handle string values: "auto", "any", "required"
	if str, ok := choice.(string); ok {
		switch str {
		case "auto":
			result.OfAuto = &anthropic.ToolChoiceAutoParam{
				Type: "auto",
			}
		case "any", "required":
			result.OfAny = &anthropic.ToolChoiceAnyParam{
				Type: "any",
			}
		case "none":
			result.OfNone = &anthropic.ToolChoiceNoneParam{
				Type: "none",
			}
		default:
			return result, fmt.Errorf("unknown tool_choice string: %s", str)
		}
		return result, nil
	}

	// Handle specific tool selection: {"type": "tool", "function": {"name": "..."}}
	if obj, ok := choice.(map[string]interface{}); ok {
		// Check for OpenAI format: {"type": "function", "function": {"name": "..."}}
		if funcObj, ok := obj["function"].(map[string]interface{}); ok {
			if name, ok := funcObj["name"].(string); ok {
				result.OfTool = &anthropic.ToolChoiceToolParam{
					Type: "tool",
					Name: name,
				}
				return result, nil
			}
		}

		// Check for direct name field
		if name, ok := obj["name"].(string); ok {
			result.OfTool = &anthropic.ToolChoiceToolParam{
				Type: "tool",
				Name: name,
			}
			return result, nil
		}
	}

	return result, fmt.Errorf("invalid tool_choice format")
}

// extractToolCalls converts Anthropic content blocks to api.ToolCall
func extractToolCalls(content []anthropic.ContentBlockUnion) []api.ToolCall {
	var toolCalls []api.ToolCall

	for _, block := range content {
		// Check if this is a tool_use block
		if block.Type == "tool_use" {
			// Cast to ToolUseBlock to access the fields
			toolUse := block.AsToolUse()

			// Marshal the input to JSON string for Arguments
			argsJSON, _ := json.Marshal(toolUse.Input)

			toolCalls = append(toolCalls, api.ToolCall{
				ID:        toolUse.ID,
				Name:      toolUse.Name,
				Arguments: string(argsJSON),
			})
		}
	}

	return toolCalls
}

