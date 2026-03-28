package google

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

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
	parsed, err := req.ParseToolChoice()
	if err != nil {
		return nil, err
	}
	if len(req.ToolChoice) == 0 {
		return nil, nil
	}

	config := &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{},
	}

	switch parsed.Mode {
	case "auto", "allowed_tools":
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAuto
		return config, nil
	case "none":
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeNone
		return config, nil
	case "required", "any":
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
		return config, nil
	case "function":
		config.FunctionCallingConfig.Mode = genai.FunctionCallingConfigModeAny
		config.FunctionCallingConfig.AllowedFunctionNames = []string{parsed.RequiredToolName}
		return config, nil
	default:
		return nil, fmt.Errorf("unsupported tool_choice format")
	}
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
	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		return "", "", fmt.Errorf("invalid base64 data: %w", err)
	}
	if defaultMediaType == "" {
		defaultMediaType = "application/octet-stream"
	}
	return defaultMediaType, value, nil
}
