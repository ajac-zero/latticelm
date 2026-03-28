package anthropic

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

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

	parsed, err := req.ParseToolChoice()
	if err != nil {
		return result, err
	}
	if len(req.ToolChoice) == 0 {
		return result, nil
	}

	switch parsed.Mode {
	case "auto", "allowed_tools":
		result.OfAuto = &anthropic.ToolChoiceAutoParam{Type: "auto"}
		return result, nil
	case "any", "required":
		result.OfAny = &anthropic.ToolChoiceAnyParam{Type: "any"}
		return result, nil
	case "none":
		result.OfNone = &anthropic.ToolChoiceNoneParam{Type: "none"}
		return result, nil
	case "function":
		result.OfTool = &anthropic.ToolChoiceToolParam{
			Type: "tool",
			Name: parsed.RequiredToolName,
		}
		return result, nil
	default:
		return result, fmt.Errorf("invalid tool_choice format")
	}
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

// parseDataURL parses a data URL into its media type and base64 data components.
// Supports format: data:[<mediatype>];base64,<data>
func parseDataURL(dataURL string) (mediaType string, data string, err error) {
	if !strings.HasPrefix(dataURL, "data:") {
		return "", "", fmt.Errorf("invalid data URL: must start with 'data:'")
	}

	// Parse the data URL
	remaining := strings.TrimPrefix(dataURL, "data:")

	// Find the comma separating metadata from data
	commaIdx := strings.Index(remaining, ",")
	if commaIdx == -1 {
		return "", "", fmt.Errorf("invalid data URL: missing comma before data")
	}

	metadata := remaining[:commaIdx]
	encodedData := remaining[commaIdx+1:]

	// Parse metadata: [<mediatype>];base64
	parts := strings.Split(metadata, ";")
	if len(parts) < 2 || parts[len(parts)-1] != "base64" {
		return "", "", fmt.Errorf("invalid data URL: expected base64 encoding")
	}

	mediaType = parts[0]
	if mediaType == "" {
		return "", "", fmt.Errorf("invalid data URL: missing media type")
	}

	// URL-decode the data portion if needed
	decodedData, err := url.QueryUnescape(encodedData)
	if err != nil {
		decodedData = encodedData
	}

	// Validate it's valid base64
	if _, err := base64.StdEncoding.DecodeString(decodedData); err != nil {
		return "", "", fmt.Errorf("invalid base64 data: %w", err)
	}

	return mediaType, decodedData, nil
}

func parseBase64Payload(value string, defaultMediaType string) (mediaType string, data string, err error) {
	if strings.HasPrefix(value, "data:") {
		return parseDataURL(value)
	}
	if defaultMediaType == "" {
		return "", "", fmt.Errorf("raw base64 content requires a known media type")
	}
	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		return "", "", fmt.Errorf("invalid base64 data: %w", err)
	}
	return defaultMediaType, value, nil
}

func decodeBase64Text(data string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("decode base64 text: %w", err)
	}
	return string(decoded), nil
}
