package google

import (
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"

	"github.com/ajac-zero/latticelm/internal/api"
)

// parseTools converts generic tool definitions from req.Tools (JSON) to Google's []*genai.Tool format.
func parseTools(req *api.ResponseRequest) ([]*genai.Tool, error) {
	if len(req.Tools) == 0 {
		return nil, nil
	}

	// Unmarshal to slice of tool definitions
	var toolDefs []map[string]interface{}
	if err := json.Unmarshal(req.Tools, &toolDefs); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}

	var functionDeclarations []*genai.FunctionDeclaration

	for _, toolDef := range toolDefs {
		// Extract function details
		// Support both flat format (name/description/parameters at top level)
		// and nested format (under "function" key)
		var name, description string
		var parameters interface{}

		if functionData, ok := toolDef["function"].(map[string]interface{}); ok {
			// Nested format: {"type": "function", "function": {...}}
			name, _ = functionData["name"].(string)
			description, _ = functionData["description"].(string)
			parameters = functionData["parameters"]
		} else {
			// Flat format: {"type": "function", "name": "...", ...}
			name, _ = toolDef["name"].(string)
			description, _ = toolDef["description"].(string)
			parameters = toolDef["parameters"]
		}

		if name == "" {
			continue
		}

		// Create function declaration
		funcDecl := &genai.FunctionDeclaration{
			Name:        name,
			Description: description,
		}

		// Google accepts parameters as raw JSON schema
		if parameters != nil {
			funcDecl.ParametersJsonSchema = parameters
		}

		functionDeclarations = append(functionDeclarations, funcDecl)
	}

	// Return single Tool with all function declarations
	if len(functionDeclarations) > 0 {
		return []*genai.Tool{{FunctionDeclarations: functionDeclarations}}, nil
	}

	return nil, nil
}

// parseToolChoice converts req.ToolChoice to Google's ToolConfig with FunctionCallingConfig.
func parseToolChoice(req *api.ResponseRequest) (*genai.ToolConfig, error) {
	if len(req.ToolChoice) == 0 {
		return nil, nil
	}

	var choice interface{}
	if err := json.Unmarshal(req.ToolChoice, &choice); err != nil {
		return nil, fmt.Errorf("unmarshal tool_choice: %w", err)
	}

	config := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{},
	}

	// Handle string values: "auto", "none", "required"/"any"
	if str, ok := choice.(string); ok {
		switch str {
		case "auto":
			config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
		case "none":
			config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeNone
		case "required", "any":
			config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
		default:
			return nil, fmt.Errorf("unknown tool_choice string: %s", str)
		}
		return config, nil
	}

	// Handle object format: {"type": "function", "function": {"name": "..."}}
	if obj, ok := choice.(map[string]interface{}); ok {
		if typeVal, ok := obj["type"].(string); ok && typeVal == "function" {
			config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
			if funcObj, ok := obj["function"].(map[string]interface{}); ok {
				if name, ok := funcObj["name"].(string); ok {
					config.FunctionCallingConfig.AllowedFunctionNames = []string{name}
				}
			}
			return config, nil
		}
	}

	return nil, fmt.Errorf("unsupported tool_choice format")
}

// extractToolCalls extracts tool calls from Google's response format to generic api.ToolCall slice.
func extractToolCalls(resp *genai.GenerateContentResponse) []api.ToolCall {
	var toolCalls []api.ToolCall

	for _, candidate := range resp.Candidates {
		if candidate.Content == nil {
			continue
		}

		for _, part := range candidate.Content.Parts {
			if part == nil || part.FunctionCall == nil {
				continue
			}

			// Extract function call details
			fc := part.FunctionCall

			// Marshal arguments to JSON string
			var argsJSON string
			if fc.Args != nil {
				argsBytes, err := json.Marshal(fc.Args)
				if err == nil {
					argsJSON = string(argsBytes)
				} else {
					// Fallback to empty object
					argsJSON = "{}"
				}
			} else {
				argsJSON = "{}"
			}

			// Generate ID if Google doesn't provide one
			callID := fc.ID
			if callID == "" {
				callID = fmt.Sprintf("call_%s", generateRandomID())
			}

			toolCalls = append(toolCalls, api.ToolCall{
				ID:        callID,
				Name:      fc.Name,
				Arguments: argsJSON,
			})
		}
	}

	return toolCalls
}

// extractToolCallDelta extracts streaming tool call information from response parts.
func extractToolCallDelta(part *genai.Part, index int) *api.ToolCallDelta {
	if part == nil || part.FunctionCall == nil {
		return nil
	}

	fc := part.FunctionCall

	// Marshal arguments to JSON string
	var argsJSON string
	if fc.Args != nil {
		argsBytes, err := json.Marshal(fc.Args)
		if err == nil {
			argsJSON = string(argsBytes)
		} else {
			argsJSON = "{}"
		}
	} else {
		argsJSON = "{}"
	}

	// Generate ID if Google doesn't provide one
	callID := fc.ID
	if callID == "" {
		callID = fmt.Sprintf("call_%s", generateRandomID())
	}

	return &api.ToolCallDelta{
		Index:     index,
		ID:        callID,
		Name:      fc.Name,
		Arguments: argsJSON,
	}
}

// generateRandomID generates a random alphanumeric ID
func generateRandomID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 24
	const fallbackID = "aaaaaaaaaaaaaaaaaaaaaaaa"

	raw := make([]byte, length)
	if _, err := cryptorand.Read(raw); err != nil {
		return fallbackID
	}

	b := make([]byte, length)
	for i, v := range raw {
		b[i] = charset[int(v)%len(charset)]
	}

	return string(b)
}
