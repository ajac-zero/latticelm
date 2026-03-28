package openai

import (
	"encoding/json"
	"fmt"
	"sort"

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
func buildOAIMessages(messages []api.Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	oaiMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			content, err := buildOAIUserContent(msg.Content)
			if err != nil {
				return nil, fmt.Errorf("convert user message: %w", err)
			}
			oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
				OfUser: &openai.ChatCompletionUserMessageParam{Content: content},
			})
		case "assistant":
			content, hasContent, err := buildOAIAssistantContent(msg.Content)
			if err != nil {
				return nil, fmt.Errorf("convert assistant message: %w", err)
			}
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
				if hasContent {
					msgParam.Content = content
				}
				oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &msgParam,
				})
			} else {
				if !hasContent {
					content.OfString = openai.String("")
				}
				oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{Content: content},
				})
			}
		case "system":
			content, err := buildOAISystemContent(msg.Content)
			if err != nil {
				return nil, fmt.Errorf("convert system message: %w", err)
			}
			oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
				OfSystem: &openai.ChatCompletionSystemMessageParam{Content: content},
			})
		case "developer":
			content, err := buildOAIDeveloperContent(msg.Content)
			if err != nil {
				return nil, fmt.Errorf("convert developer message: %w", err)
			}
			oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
				OfDeveloper: &openai.ChatCompletionDeveloperMessageParam{Content: content},
			})
		case "tool":
			content, err := buildOAIToolContent(msg.Content)
			if err != nil {
				return nil, fmt.Errorf("convert tool message: %w", err)
			}
			oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
				OfTool: &openai.ChatCompletionToolMessageParam{
					Content:    content,
					ToolCallID: sanitizeToolCallID(msg.CallID),
				},
			})
		}
	}
	return oaiMessages, nil
}

func buildOAIUserContent(blocks []api.ContentBlock) (openai.ChatCompletionUserMessageParamContentUnion, error) {
	textParts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(blocks))
	textOnly := true

	for _, block := range blocks {
		switch block.Type {
		case "text", "input_text", "output_text":
			textParts = append(textParts, openai.TextContentPart(block.Text))
		case "input_image":
			textOnly = false
			textParts = append(textParts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
				URL:    block.ImageURL,
				Detail: block.Detail,
			}))
		case "input_file":
			textOnly = false
			if block.FileURL != "" {
				return openai.ChatCompletionUserMessageParamContentUnion{}, fmt.Errorf("input_file with file_url is not supported by OpenAI chat completions")
			}
			textParts = append(textParts, openai.FileContentPart(openai.ChatCompletionContentPartFileFileParam{
				FileData: openai.String(block.FileData),
				Filename: openai.String(block.Filename),
			}))
		case "input_video":
			return openai.ChatCompletionUserMessageParamContentUnion{}, fmt.Errorf("input_video is not supported by OpenAI chat completions")
		case "refusal":
			return openai.ChatCompletionUserMessageParamContentUnion{}, fmt.Errorf("refusal content is not valid in user messages")
		default:
			return openai.ChatCompletionUserMessageParamContentUnion{}, fmt.Errorf("unsupported user content block type %q", block.Type)
		}
	}

	if len(textParts) == 1 && textOnly && textParts[0].OfText != nil {
		return openai.ChatCompletionUserMessageParamContentUnion{
			OfString: openai.String(textParts[0].OfText.Text),
		}, nil
	}

	return openai.ChatCompletionUserMessageParamContentUnion{
		OfArrayOfContentParts: textParts,
	}, nil
}

func buildOAIAssistantContent(blocks []api.ContentBlock) (openai.ChatCompletionAssistantMessageParamContentUnion, bool, error) {
	if len(blocks) == 0 {
		return openai.ChatCompletionAssistantMessageParamContentUnion{}, false, nil
	}

	parts := make([]openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion, 0, len(blocks))
	textOnly := true
	for _, block := range blocks {
		switch block.Type {
		case "text", "input_text", "output_text":
			parts = append(parts, openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
				OfText: &openai.ChatCompletionContentPartTextParam{Text: block.Text},
			})
		case "refusal":
			textOnly = false
			parts = append(parts, openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
				OfRefusal: &openai.ChatCompletionContentPartRefusalParam{Refusal: block.Refusal},
			})
		case "encrypted_reasoning":
			continue
		default:
			return openai.ChatCompletionAssistantMessageParamContentUnion{}, false, fmt.Errorf("unsupported assistant content block type %q", block.Type)
		}
	}
	if len(parts) == 0 {
		return openai.ChatCompletionAssistantMessageParamContentUnion{}, false, nil
	}

	if len(parts) == 1 && textOnly && parts[0].OfText != nil {
		return openai.ChatCompletionAssistantMessageParamContentUnion{
			OfString: openai.String(parts[0].OfText.Text),
		}, true, nil
	}

	return openai.ChatCompletionAssistantMessageParamContentUnion{
		OfArrayOfContentParts: parts,
	}, true, nil
}

func buildOAISystemContent(blocks []api.ContentBlock) (openai.ChatCompletionSystemMessageParamContentUnion, error) {
	parts, err := buildOAITextOnlyParts(blocks, "system")
	if err != nil {
		return openai.ChatCompletionSystemMessageParamContentUnion{}, err
	}
	if len(parts) == 1 {
		return openai.ChatCompletionSystemMessageParamContentUnion{
			OfString: openai.String(parts[0].Text),
		}, nil
	}
	return openai.ChatCompletionSystemMessageParamContentUnion{
		OfArrayOfContentParts: parts,
	}, nil
}

func buildOAIDeveloperContent(blocks []api.ContentBlock) (openai.ChatCompletionDeveloperMessageParamContentUnion, error) {
	parts, err := buildOAITextOnlyParts(blocks, "developer")
	if err != nil {
		return openai.ChatCompletionDeveloperMessageParamContentUnion{}, err
	}
	if len(parts) == 1 {
		return openai.ChatCompletionDeveloperMessageParamContentUnion{
			OfString: openai.String(parts[0].Text),
		}, nil
	}
	return openai.ChatCompletionDeveloperMessageParamContentUnion{
		OfArrayOfContentParts: parts,
	}, nil
}

func buildOAIToolContent(blocks []api.ContentBlock) (openai.ChatCompletionToolMessageParamContentUnion, error) {
	parts, err := buildOAITextOnlyParts(blocks, "tool")
	if err != nil {
		return openai.ChatCompletionToolMessageParamContentUnion{}, err
	}
	if len(parts) == 1 {
		return openai.ChatCompletionToolMessageParamContentUnion{
			OfString: openai.String(parts[0].Text),
		}, nil
	}
	return openai.ChatCompletionToolMessageParamContentUnion{
		OfArrayOfContentParts: parts,
	}, nil
}

func buildOAITextOnlyParts(blocks []api.ContentBlock, role string) ([]openai.ChatCompletionContentPartTextParam, error) {
	parts := make([]openai.ChatCompletionContentPartTextParam, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "encrypted_reasoning" {
			continue
		}
		text, ok := block.TextValue()
		if !ok || block.Type == "refusal" {
			return nil, fmt.Errorf("%s messages only support text content; found %q", role, block.Type)
		}
		parts = append(parts, openai.ChatCompletionContentPartTextParam{Text: text})
	}
	return parts, nil
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

	parsed, err := req.ParseToolChoice()
	if err != nil {
		return result, err
	}
	if len(req.ToolChoice) == 0 {
		return result, nil
	}

	switch parsed.Mode {
	case "auto", "none", "required", "any":
		mode := parsed.Mode
		if mode == "any" {
			mode = "required"
		}
		result.OfAuto = openai.String(mode)
		return result, nil
	case "function":
		return openai.ToolChoiceOptionFunctionToolChoice(
			openai.ChatCompletionNamedToolChoiceFunctionParam{
				Name: parsed.RequiredToolName,
			},
		), nil
	case "allowed_tools":
		names := make([]string, 0, len(parsed.AllowedTools))
		for name := range parsed.AllowedTools {
			names = append(names, name)
		}
		sort.Strings(names)
		tools := make([]map[string]any, 0, len(names))
		for _, name := range names {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": name,
				},
			})
		}
		return openai.ToolChoiceOptionAllowedTools(openai.ChatCompletionAllowedToolsParam{
			Mode:  openai.ChatCompletionAllowedToolsModeAuto,
			Tools: tools,
		}), nil
	default:
		return result, fmt.Errorf("invalid tool_choice format")
	}
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
