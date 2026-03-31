package api

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Validate performs basic structural validation.
func (r *ResponseRequest) Validate() error {
	if r == nil {
		return errors.New("request is nil")
	}
	if r.Model == "" {
		return errors.New("model is required")
	}
	if r.Input.String == nil && len(r.Input.Items) == 0 && (r.PreviousResponseID == nil || *r.PreviousResponseID == "") {
		return errors.New("input is required")
	}
	if r.Truncation != nil && *r.Truncation != "auto" && *r.Truncation != "disabled" {
		return errors.New(`truncation must be "auto" or "disabled"`)
	}
	toolChoice, err := r.ParseToolChoice()
	if err != nil {
		return err
	}
	if toolChoice.Mode != "" && toolChoice.Mode != "auto" && toolChoice.Mode != "none" {
		toolNames, err := r.DeclaredToolNames()
		if err != nil {
			return err
		}
		if len(toolNames) == 0 {
			return errors.New("tool_choice requires tools to be declared")
		}
		switch toolChoice.Mode {
		case "required", "any":
		case "function":
			if _, ok := toolNames[toolChoice.RequiredToolName]; !ok {
				return fmt.Errorf("tool_choice references unknown tool %q", toolChoice.RequiredToolName)
			}
		case "allowed_tools":
			for name := range toolChoice.AllowedTools {
				if _, ok := toolNames[name]; !ok {
					return fmt.Errorf("allowed_tools references unknown tool %q", name)
				}
			}
		default:
			return fmt.Errorf("unsupported tool_choice mode %q", toolChoice.Mode)
		}
	}
	hasItemReference := false
	for _, item := range r.Input.Items {
		switch item.Type {
		case "", "message", "function_call", "function_call_output":
		case "item_reference":
			hasItemReference = true
			if item.ID == "" {
				return errors.New("item_reference id is required")
			}
		case "reasoning":
			if item.Content == nil && item.Summary == nil && item.EncryptedContent == "" {
				return errors.New("reasoning item must include content, summary, or encrypted_content")
			}
		default:
			return fmt.Errorf("unsupported input item type %q", item.Type)
		}
	}
	if hasItemReference && (r.PreviousResponseID == nil || *r.PreviousResponseID == "") {
		return errors.New("previous_response_id is required when using item_reference")
	}
	return nil
}

// ParseToolChoice decodes the tool_choice field into a ParsedToolChoice.
func (r *ResponseRequest) ParseToolChoice() (ParsedToolChoice, error) {
	parsed := ParsedToolChoice{Mode: "auto"}
	if r == nil || len(r.ToolChoice) == 0 {
		return parsed, nil
	}

	var choice interface{}
	if err := json.Unmarshal(r.ToolChoice, &choice); err != nil {
		return parsed, fmt.Errorf("invalid tool_choice: %w", err)
	}

	if str, ok := choice.(string); ok {
		switch str {
		case "auto", "none", "required", "any":
			parsed.Mode = str
			return parsed, nil
		default:
			return parsed, fmt.Errorf("invalid tool_choice value %q", str)
		}
	}

	obj, ok := choice.(map[string]interface{})
	if !ok {
		return parsed, errors.New("invalid tool_choice format")
	}

	switch objType, _ := obj["type"].(string); objType {
	case "function", "tool":
		name := parseToolChoiceName(obj)
		if name == "" {
			return parsed, errors.New("tool_choice function name is required")
		}
		parsed.Mode = "function"
		parsed.RequiredToolName = name
		return parsed, nil
	case "allowed_tools":
		tools, _ := obj["tools"].([]interface{})
		if len(tools) == 0 {
			return parsed, errors.New("allowed_tools requires at least one tool")
		}
		parsed.Mode = "allowed_tools"
		parsed.AllowedTools = make(map[string]struct{}, len(tools))
		for _, rawTool := range tools {
			tool, ok := rawTool.(map[string]interface{})
			if !ok {
				return parsed, errors.New("allowed_tools entries must be objects")
			}
			name := parseToolChoiceName(tool)
			if name == "" {
				return parsed, errors.New("allowed_tools entries require a tool name")
			}
			parsed.AllowedTools[name] = struct{}{}
		}
		return parsed, nil
	default:
		return parsed, errors.New("invalid tool_choice format")
	}
}

// DeclaredToolNames returns the set of tool names declared in the tools field.
func (r *ResponseRequest) DeclaredToolNames() (map[string]struct{}, error) {
	names := map[string]struct{}{}
	if r == nil || len(r.Tools) == 0 {
		return names, nil
	}

	var toolDefs []map[string]interface{}
	if err := json.Unmarshal(r.Tools, &toolDefs); err != nil {
		return nil, fmt.Errorf("invalid tools: %w", err)
	}

	for _, toolDef := range toolDefs {
		name := parseToolChoiceName(toolDef)
		if name != "" {
			names[name] = struct{}{}
		}
	}

	return names, nil
}

// parseToolChoiceName extracts a tool name from a tool definition or tool_choice object.
func parseToolChoiceName(obj map[string]interface{}) string {
	if name, ok := obj["name"].(string); ok {
		return name
	}
	if function, ok := obj["function"].(map[string]interface{}); ok {
		if name, ok := function["name"].(string); ok {
			return name
		}
	}
	return ""
}
