package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/auth"
	"github.com/ajac-zero/latticelm/internal/conversation"
)

func TestHandleModels(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		setupServer  func() *GatewayServer
		expectStatus int
		validate     func(t *testing.T, body string)
	}{
		{
			name:   "GET returns model list",
			method: http.MethodGet,
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addModel("gpt-4", "openai")
				registry.addModel("claude-3", "anthropic")
				registry.addProvider("openai", newMockProvider("openai"))
				registry.addProvider("anthropic", newMockProvider("anthropic"))
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			expectStatus: http.StatusOK,
			validate: func(t *testing.T, body string) {
				var resp api.ModelsResponse
				err := json.Unmarshal([]byte(body), &resp)
				require.NoError(t, err)
				assert.Equal(t, "list", resp.Object)
				assert.Len(t, resp.Data, 2)
			},
		},
		{
			name:   "POST returns 405",
			method: http.MethodPost,
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			expectStatus: http.StatusMethodNotAllowed,
		},
		{
			name:   "empty registry returns empty list",
			method: http.MethodGet,
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			expectStatus: http.StatusOK,
			validate: func(t *testing.T, body string) {
				var resp api.ModelsResponse
				err := json.Unmarshal([]byte(body), &resp)
				require.NoError(t, err)
				assert.Equal(t, "list", resp.Object)
				assert.Len(t, resp.Data, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			req := httptest.NewRequest(tt.method, "/v1/models", nil)
			rec := httptest.NewRecorder()

			server.handleModels(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code)
			if tt.validate != nil {
				tt.validate(t, rec.Body.String())
			}
		})
	}
}

func TestHandleResponses_Validation(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		body         string
		expectStatus int
		expectBody   string
	}{
		{
			name:         "GET returns 405",
			method:       http.MethodGet,
			body:         "",
			expectStatus: http.StatusMethodNotAllowed,
		},
		{
			name:         "invalid JSON returns 400",
			method:       http.MethodPost,
			body:         `{invalid json}`,
			expectStatus: http.StatusBadRequest,
			expectBody:   "invalid JSON payload",
		},
		{
			name:         "missing model returns 400",
			method:       http.MethodPost,
			body:         `{"input": "hello"}`,
			expectStatus: http.StatusBadRequest,
			expectBody:   "model is required",
		},
		{
			name:         "missing input returns 400",
			method:       http.MethodPost,
			body:         `{"model": "gpt-4"}`,
			expectStatus: http.StatusBadRequest,
			expectBody:   "input is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := newMockRegistry()
			server := New(registry, newMockConversationStore(), newMockLogger().asLogger())

			req := httptest.NewRequest(tt.method, "/v1/responses", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.handleResponses(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code)
			if tt.expectBody != "" {
				assert.Contains(t, rec.Body.String(), tt.expectBody)
			}
		})
	}
}

func TestHandleResponses_Sync_Success(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		setupMock   func(p *mockProvider)
		validate    func(t *testing.T, resp *api.Response, store *mockConversationStore)
	}{
		{
			name:        "simple text response",
			requestBody: `{"model": "gpt-4", "input": "hello", "store": true}`,
			setupMock: func(p *mockProvider) {
				p.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					return &api.ProviderResult{
						Model: "gpt-4-turbo",
						Text:  "Hello! How can I help you?",
						Usage: api.Usage{
							InputTokens:  5,
							OutputTokens: 10,
							TotalTokens:  15,
						},
					}, nil
				}
			},
			validate: func(t *testing.T, resp *api.Response, store *mockConversationStore) {
				assert.Equal(t, "response", resp.Object)
				assert.Equal(t, "completed", resp.Status)
				assert.Equal(t, "gpt-4-turbo", resp.Model)
				assert.Equal(t, "openai", resp.Provider)
				require.Len(t, resp.Output, 1)
				assert.Equal(t, "message", resp.Output[0].Type)
				assert.Equal(t, "completed", resp.Output[0].Status)
				assert.Equal(t, "assistant", resp.Output[0].Role)
				require.Len(t, resp.Output[0].Content, 1)
				assert.Equal(t, "output_text", resp.Output[0].Content[0].Type)
				assert.Equal(t, "Hello! How can I help you?", resp.Output[0].Content[0].Text)
				require.NotNil(t, resp.Usage)
				assert.Equal(t, 5, resp.Usage.InputTokens)
				assert.Equal(t, 10, resp.Usage.OutputTokens)
				assert.Equal(t, 15, resp.Usage.TotalTokens)
				assert.Equal(t, 1, store.Size())
			},
		},
		{
			name:        "response with tool calls",
			requestBody: `{"model": "gpt-4", "input": "what's the weather?", "tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`,
			setupMock: func(p *mockProvider) {
				p.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					return &api.ProviderResult{
						Model: "gpt-4",
						Text:  "Let me check that for you.",
						ToolCalls: []api.ToolCall{
							{
								ID:        "call_123",
								Name:      "get_weather",
								Arguments: `{"location":"San Francisco"}`,
							},
						},
						Usage: api.Usage{
							InputTokens:  10,
							OutputTokens: 20,
							TotalTokens:  30,
						},
					}, nil
				}
			},
			validate: func(t *testing.T, resp *api.Response, store *mockConversationStore) {
				assert.Equal(t, "completed", resp.Status)
				require.Len(t, resp.Output, 2)
				assert.Equal(t, "message", resp.Output[0].Type)
				assert.Equal(t, "Let me check that for you.", resp.Output[0].Content[0].Text)
				assert.Equal(t, "function_call", resp.Output[1].Type)
				assert.Equal(t, "completed", resp.Output[1].Status)
				assert.Equal(t, "call_123", resp.Output[1].CallID)
				assert.Equal(t, "get_weather", resp.Output[1].Name)
				assert.JSONEq(t, `{"location":"San Francisco"}`, resp.Output[1].Arguments)
			},
		},
		{
			name:        "response with multiple tool calls",
			requestBody: `{"model": "gpt-4", "input": "check NYC and LA weather", "tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`,
			setupMock: func(p *mockProvider) {
				p.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					return &api.ProviderResult{
						Model: "gpt-4",
						Text:  "Checking both cities.",
						ToolCalls: []api.ToolCall{
							{ID: "call_1", Name: "get_weather", Arguments: `{"location":"NYC"}`},
							{ID: "call_2", Name: "get_weather", Arguments: `{"location":"LA"}`},
						},
					}, nil
				}
			},
			validate: func(t *testing.T, resp *api.Response, store *mockConversationStore) {
				require.Len(t, resp.Output, 3)
				assert.Equal(t, "message", resp.Output[0].Type)
				assert.Equal(t, "function_call", resp.Output[1].Type)
				assert.Equal(t, "function_call", resp.Output[2].Type)
				assert.Equal(t, "call_1", resp.Output[1].CallID)
				assert.Equal(t, "call_2", resp.Output[2].CallID)
			},
		},
		{
			name:        "response with only tool calls (no text)",
			requestBody: `{"model": "gpt-4", "input": "search", "tools":[{"type":"function","name":"search","parameters":{"type":"object"}}]}`,
			setupMock: func(p *mockProvider) {
				p.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					return &api.ProviderResult{
						Model: "gpt-4",
						ToolCalls: []api.ToolCall{
							{ID: "call_xyz", Name: "search", Arguments: `{}`},
						},
					}, nil
				}
			},
			validate: func(t *testing.T, resp *api.Response, store *mockConversationStore) {
				require.Len(t, resp.Output, 1)
				assert.Equal(t, "function_call", resp.Output[0].Type)
				assert.NotNil(t, resp.Usage)
			},
		},
		{
			name:        "response echoes request parameters",
			requestBody: `{"model": "gpt-4", "input": "hi", "temperature": 0.7, "top_p": 0.9, "parallel_tool_calls": false}`,
			setupMock:   nil,
			validate: func(t *testing.T, resp *api.Response, store *mockConversationStore) {
				assert.Equal(t, 0.7, resp.Temperature)
				assert.Equal(t, 0.9, resp.TopP)
				assert.False(t, resp.ParallelToolCalls)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := newMockRegistry()
			provider := newMockProvider("openai")
			if tt.setupMock != nil {
				tt.setupMock(provider)
			}
			registry.addProvider("openai", provider)
			registry.addModel("gpt-4", "openai")

			store := newMockConversationStore()
			server := New(registry, store, newMockLogger().asLogger())

			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.handleResponses(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)

			var resp api.Response
			err := json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err)

			if tt.validate != nil {
				tt.validate(t, &resp, store)
			}
		})
	}
}

func TestHandleResponses_Sync_ConversationHistory(t *testing.T) {
	tests := []struct {
		name         string
		setupServer  func() *GatewayServer
		requestBody  string
		expectStatus int
		expectBody   string
		validate     func(t *testing.T, provider *mockProvider)
	}{
		{
			name: "without previous_response_id",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				provider := newMockProvider("openai")
				registry.addProvider("openai", provider)
				registry.addModel("gpt-4", "openai")
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			requestBody:  `{"model": "gpt-4", "input": "hello"}`,
			expectStatus: http.StatusOK,
			validate: func(t *testing.T, provider *mockProvider) {
				assert.Equal(t, 1, provider.getGenerateCalled())
			},
		},
		{
			name: "with valid previous_response_id",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				provider := newMockProvider("openai")
				provider.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					// Should receive history + new message
					if len(messages) != 2 {
						return nil, fmt.Errorf("expected 2 messages, got %d", len(messages))
					}
					return &api.ProviderResult{
						Model: req.Model,
						Text:  "response",
					}, nil
				}
				registry.addProvider("openai", provider)
				registry.addModel("gpt-4", "openai")

				store := newMockConversationStore()
				store.setConversation("prev-123", &conversation.Conversation{
					ID:    "prev-123",
					Model: "gpt-4",
					Messages: []api.Message{
						{
							Role:    "user",
							Content: []api.ContentBlock{{Type: "input_text", Text: "previous message"}},
						},
					},
				})
				return New(registry, store, newMockLogger().asLogger())
			},
			requestBody:  `{"model": "gpt-4", "input": "new message", "previous_response_id": "prev-123"}`,
			expectStatus: http.StatusOK,
		},
		{
			name: "with instructions prepends developer message",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				provider := newMockProvider("openai")
				provider.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					// Should have developer message first
					if len(messages) < 1 || messages[0].Role != "developer" {
						return nil, fmt.Errorf("expected developer message first")
					}
					if messages[0].Content[0].Text != "Be helpful" {
						return nil, fmt.Errorf("unexpected instructions: %s", messages[0].Content[0].Text)
					}
					return &api.ProviderResult{
						Model: req.Model,
						Text:  "response",
					}, nil
				}
				registry.addProvider("openai", provider)
				registry.addModel("gpt-4", "openai")
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			requestBody:  `{"model": "gpt-4", "input": "hello", "instructions": "Be helpful"}`,
			expectStatus: http.StatusOK,
		},
		{
			name: "inherits previous request context",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				provider := newMockProvider("openai")
				provider.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					if len(messages) < 1 || messages[0].Role != "developer" {
						return nil, fmt.Errorf("expected inherited developer message first")
					}
					if len(req.Tools) == 0 {
						return nil, fmt.Errorf("expected inherited tools")
					}
					return &api.ProviderResult{
						Model: req.Model,
						Text:  "response",
					}, nil
				}
				registry.addProvider("openai", provider)
				registry.addModel("gpt-4", "openai")

				store := newMockConversationStore()
				store.setConversation("prev-with-config", &conversation.Conversation{
					ID:    "prev-with-config",
					Model: "gpt-4",
					Messages: []api.Message{
						{
							Role:    "user",
							Content: []api.ContentBlock{{Type: "input_text", Text: "previous message"}},
						},
					},
					Request: &api.ResponseRequest{
						Model:        "gpt-4",
						Instructions: stringPtr("Use the provided tools"),
						Tools:        json.RawMessage(`[{"type":"function","name":"search","parameters":{"type":"object"}}]`),
					},
				})
				return New(registry, store, newMockLogger().asLogger())
			},
			requestBody:  `{"model": "gpt-4", "input": "new message", "previous_response_id": "prev-with-config"}`,
			expectStatus: http.StatusOK,
		},
		{
			name: "nonexistent conversation returns 404",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				provider := newMockProvider("openai")
				registry.addProvider("openai", provider)
				registry.addModel("gpt-4", "openai")
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			requestBody:  `{"model": "gpt-4", "input": "hello", "previous_response_id": "nonexistent"}`,
			expectStatus: http.StatusNotFound,
			expectBody:   "conversation not found",
		},
		{
			name: "conversation store error returns 500",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				provider := newMockProvider("openai")
				registry.addProvider("openai", provider)
				registry.addModel("gpt-4", "openai")

				store := newMockConversationStore()
				store.getErr = fmt.Errorf("database error")
				return New(registry, store, newMockLogger().asLogger())
			},
			requestBody:  `{"model": "gpt-4", "input": "hello", "previous_response_id": "any"}`,
			expectStatus: http.StatusInternalServerError,
			expectBody:   "error retrieving conversation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()

			// Get the provider for validation if needed
			var provider *mockProvider
			if registry, ok := server.registry.(*mockRegistry); ok {
				if p, exists := registry.Get("openai"); exists {
					provider = p.(*mockProvider)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.handleResponses(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code)
			if tt.expectBody != "" {
				assert.Contains(t, rec.Body.String(), tt.expectBody)
			}
			if tt.validate != nil && provider != nil {
				tt.validate(t, provider)
			}
		})
	}
}

func TestHandleResponses_ToolChoiceEnforcement(t *testing.T) {
	registry := newMockRegistry()
	provider := newMockProvider("openai")
	provider.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
		return &api.ProviderResult{
			Model: "gpt-4",
			ToolCalls: []api.ToolCall{
				{ID: "call_1", Name: "search", Arguments: `{}`},
			},
		}, nil
	}
	registry.addProvider("openai", provider)
	registry.addModel("gpt-4", "openai")

	server := New(registry, newMockConversationStore(), newMockLogger().asLogger())
	body := `{"model":"gpt-4","input":"hello","tool_choice":"none","tools":[{"type":"function","name":"search","parameters":{"type":"object"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleResponses(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "tool_choice was none")
}

func TestHandleResponses_Sync_ProviderErrors(t *testing.T) {
	tests := []struct {
		name         string
		setupMock    func(p *mockProvider)
		expectStatus int
		expectBody   string
	}{
		{
			name: "provider returns error",
			setupMock: func(p *mockProvider) {
				p.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					return nil, fmt.Errorf("rate limit exceeded")
				}
			},
			expectStatus: http.StatusInternalServerError,
			expectBody:   "provider error",
		},
		{
			name: "provider not configured",
			setupMock: func(p *mockProvider) {
				// Don't set up this provider, request will use explicit provider
			},
			expectStatus: http.StatusBadRequest,
			expectBody:   "provider nonexistent not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := newMockRegistry()
			provider := newMockProvider("openai")
			if tt.setupMock != nil {
				tt.setupMock(provider)
			}
			registry.addProvider("openai", provider)
			registry.addModel("gpt-4", "openai")

			server := New(registry, newMockConversationStore(), newMockLogger().asLogger())

			body := `{"model": "gpt-4", "input": "hello"}`
			if tt.name == "provider not configured" {
				body = `{"model": "gpt-4", "input": "hello", "provider": "nonexistent"}`
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.handleResponses(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code)
			if tt.expectBody != "" {
				assert.Contains(t, rec.Body.String(), tt.expectBody)
			}
		})
	}
}

func TestHandleResponses_Stream_Success(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		setupMock   func(p *mockProvider)
		validate    func(t *testing.T, events []api.StreamEvent)
	}{
		{
			name:        "simple text streaming",
			requestBody: `{"model": "gpt-4", "input": "hello", "stream": true}`,
			setupMock: func(p *mockProvider) {
				p.streamFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
					deltaChan := make(chan *api.ProviderStreamDelta)
					errChan := make(chan error, 1)
					go func() {
						defer close(deltaChan)
						defer close(errChan)
						deltaChan <- &api.ProviderStreamDelta{Model: "gpt-4-turbo", Text: "Hello"}
						deltaChan <- &api.ProviderStreamDelta{Text: " there"}
						deltaChan <- &api.ProviderStreamDelta{Done: true}
					}()
					return deltaChan, errChan
				}
			},
			validate: func(t *testing.T, events []api.StreamEvent) {
				require.GreaterOrEqual(t, len(events), 7)
				assert.Equal(t, "response.in_progress", events[0].Type)
				assert.Equal(t, "response.output_item.added", events[1].Type)
				assert.Equal(t, "response.content_part.added", events[2].Type)

				// Find text deltas
				var textDeltas []string
				for _, e := range events {
					if e.Type == "response.output_text.delta" {
						textDeltas = append(textDeltas, e.Delta)
					}
				}
				assert.Equal(t, []string{"Hello", " there"}, textDeltas)

				// Last event should be response.completed
				lastEvent := events[len(events)-1]
				assert.Equal(t, "response.completed", lastEvent.Type)
				require.NotNil(t, lastEvent.Response)
				assert.Equal(t, "completed", lastEvent.Response.Status)
				assert.Equal(t, "gpt-4-turbo", lastEvent.Response.Model)
			},
		},
		{
			name:        "streaming with tool calls",
			requestBody: `{"model": "gpt-4", "input": "weather?", "stream": true, "tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`,
			setupMock: func(p *mockProvider) {
				p.streamFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
					deltaChan := make(chan *api.ProviderStreamDelta)
					errChan := make(chan error, 1)
					go func() {
						defer close(deltaChan)
						defer close(errChan)
						deltaChan <- &api.ProviderStreamDelta{Model: "gpt-4", Text: "Let me check"}
						deltaChan <- &api.ProviderStreamDelta{
							ToolCallDelta: &api.ToolCallDelta{
								Index: 0,
								ID:    "call_abc",
								Name:  "get_weather",
							},
						}
						deltaChan <- &api.ProviderStreamDelta{
							ToolCallDelta: &api.ToolCallDelta{
								Index:     0,
								Arguments: `{"location":"NYC"}`,
							},
						}
						deltaChan <- &api.ProviderStreamDelta{Done: true}
					}()
					return deltaChan, errChan
				}
			},
			validate: func(t *testing.T, events []api.StreamEvent) {
				// Find tool call events
				var toolCallAdded bool
				var argsDeltas []string
				for _, e := range events {
					if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "function_call" {
						toolCallAdded = true
						assert.Equal(t, "call_abc", e.Item.CallID)
						assert.Equal(t, "get_weather", e.Item.Name)
					}
					if e.Type == "response.function_call_arguments.delta" {
						argsDeltas = append(argsDeltas, e.Delta)
					}
				}
				assert.True(t, toolCallAdded, "should have tool call added event")
				assert.Equal(t, []string{`{"location":"NYC"}`}, argsDeltas)
			},
		},
		{
			name:        "streaming with multiple tool calls",
			requestBody: `{"model": "gpt-4", "input": "check multiple", "stream": true, "tools":[{"type":"function","name":"tool_a","parameters":{"type":"object"}},{"type":"function","name":"tool_b","parameters":{"type":"object"}}]}`,
			setupMock: func(p *mockProvider) {
				p.streamFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
					deltaChan := make(chan *api.ProviderStreamDelta)
					errChan := make(chan error, 1)
					go func() {
						defer close(deltaChan)
						defer close(errChan)
						// First tool call
						deltaChan <- &api.ProviderStreamDelta{
							ToolCallDelta: &api.ToolCallDelta{
								Index: 0,
								ID:    "call_1",
								Name:  "tool_a",
							},
						}
						deltaChan <- &api.ProviderStreamDelta{
							ToolCallDelta: &api.ToolCallDelta{
								Index:     0,
								Arguments: `{"a":1}`,
							},
						}
						// Second tool call
						deltaChan <- &api.ProviderStreamDelta{
							ToolCallDelta: &api.ToolCallDelta{
								Index: 1,
								ID:    "call_2",
								Name:  "tool_b",
							},
						}
						deltaChan <- &api.ProviderStreamDelta{
							ToolCallDelta: &api.ToolCallDelta{
								Index:     1,
								Arguments: `{"b":2}`,
							},
						}
						deltaChan <- &api.ProviderStreamDelta{Done: true}
					}()
					return deltaChan, errChan
				}
			},
			validate: func(t *testing.T, events []api.StreamEvent) {
				var toolCallCount int
				for _, e := range events {
					if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "function_call" {
						toolCallCount++
					}
				}
				assert.Equal(t, 2, toolCallCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := newMockRegistry()
			provider := newMockProvider("openai")
			if tt.setupMock != nil {
				tt.setupMock(provider)
			}
			registry.addProvider("openai", provider)
			registry.addModel("gpt-4", "openai")

			server := New(registry, newMockConversationStore(), newMockLogger().asLogger())

			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rec := newFlushableRecorder()

			server.handleResponses(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Body.String(), "data: [DONE]")

			events, err := parseSSEEvents(rec.Body)
			require.NoError(t, err)

			if tt.validate != nil {
				tt.validate(t, events)
			}
		})
	}
}

func TestHandleResponses_Stream_Errors(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(p *mockProvider)
		validate  func(t *testing.T, events []api.StreamEvent)
	}{
		{
			name: "stream error returns failed event",
			setupMock: func(p *mockProvider) {
				p.streamFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
					deltaChan := make(chan *api.ProviderStreamDelta)
					errChan := make(chan error, 1)
					go func() {
						defer close(deltaChan)
						defer close(errChan)
						errChan <- fmt.Errorf("stream error occurred")
					}()
					return deltaChan, errChan
				}
			},
			validate: func(t *testing.T, events []api.StreamEvent) {
				// Should have initial events and then failed event
				var foundFailed bool
				for _, e := range events {
					if e.Type == "response.failed" {
						foundFailed = true
						require.NotNil(t, e.Response)
						assert.Equal(t, "failed", e.Response.Status)
						require.NotNil(t, e.Response.Error)
						assert.Contains(t, e.Response.Error.Message, "stream error")
					}
				}
				assert.True(t, foundFailed)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := newMockRegistry()
			provider := newMockProvider("openai")
			if tt.setupMock != nil {
				tt.setupMock(provider)
			}
			registry.addProvider("openai", provider)
			registry.addModel("gpt-4", "openai")

			server := New(registry, newMockConversationStore(), newMockLogger().asLogger())

			body := `{"model": "gpt-4", "input": "hello", "stream": true}`
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := newFlushableRecorder()

			server.handleResponses(rec, req)
			assert.Contains(t, rec.Body.String(), "data: [DONE]")

			events, err := parseSSEEvents(rec.Body)
			require.NoError(t, err)

			if tt.validate != nil {
				tt.validate(t, events)
			}
		})
	}
}

func TestResolveProvider(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *GatewayServer
		request     api.ResponseRequest
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, provider any)
	}{
		{
			name: "explicit provider selection",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addProvider("openai", newMockProvider("openai"))
				registry.addProvider("anthropic", newMockProvider("anthropic"))
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			request: api.ResponseRequest{
				Model:    "gpt-4",
				Provider: "anthropic",
			},
			validate: func(t *testing.T, provider any) {
				assert.Equal(t, "anthropic", provider.(*mockProvider).Name())
			},
		},
		{
			name: "default by model name",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addProvider("openai", newMockProvider("openai"))
				registry.addModel("gpt-4", "openai")
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			request: api.ResponseRequest{
				Model: "gpt-4",
			},
			validate: func(t *testing.T, provider any) {
				assert.Equal(t, "openai", provider.(*mockProvider).Name())
			},
		},
		{
			name: "provider not found returns error",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addProvider("openai", newMockProvider("openai"))
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			request: api.ResponseRequest{
				Model:    "gpt-4",
				Provider: "nonexistent",
			},
			expectError: true,
			errorMsg:    "provider nonexistent not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			provider, err := server.resolveProvider(&tt.request)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, provider)
			if tt.validate != nil {
				tt.validate(t, provider)
			}
		})
	}
}

func TestGenerateID(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{
			name:   "resp_ prefix",
			prefix: "resp_",
		},
		{
			name:   "msg_ prefix",
			prefix: "msg_",
		},
		{
			name:   "item_ prefix",
			prefix: "item_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := generateID(tt.prefix)
			assert.True(t, strings.HasPrefix(id, tt.prefix))
			assert.Len(t, id, len(tt.prefix)+24)

			// Generate another to ensure uniqueness
			id2 := generateID(tt.prefix)
			assert.NotEqual(t, id, id2)
		})
	}
}

func TestBuildResponse(t *testing.T) {
	tests := []struct {
		name     string
		request  *api.ResponseRequest
		result   *api.ProviderResult
		provider string
		id       string
		validate func(t *testing.T, resp *api.Response)
	}{
		{
			name: "minimal response structure",
			request: &api.ResponseRequest{
				Model: "gpt-4",
			},
			result: &api.ProviderResult{
				Model: "gpt-4-turbo",
				Text:  "Hello",
			},
			provider: "openai",
			id:       "resp_123",
			validate: func(t *testing.T, resp *api.Response) {
				assert.Equal(t, "resp_123", resp.ID)
				assert.Equal(t, "response", resp.Object)
				assert.Equal(t, "completed", resp.Status)
				assert.Equal(t, "gpt-4-turbo", resp.Model)
				assert.Equal(t, "openai", resp.Provider)
				assert.NotNil(t, resp.CompletedAt)
				assert.Len(t, resp.Output, 1)
				assert.Equal(t, "message", resp.Output[0].Type)
			},
		},
		{
			name: "response with tool calls",
			request: &api.ResponseRequest{
				Model: "gpt-4",
			},
			result: &api.ProviderResult{
				Model: "gpt-4",
				Text:  "Let me check",
				ToolCalls: []api.ToolCall{
					{ID: "call_1", Name: "get_weather", Arguments: `{"location":"NYC"}`},
				},
			},
			provider: "openai",
			id:       "resp_456",
			validate: func(t *testing.T, resp *api.Response) {
				assert.Len(t, resp.Output, 2)
				assert.Equal(t, "message", resp.Output[0].Type)
				assert.Equal(t, "function_call", resp.Output[1].Type)
				assert.Equal(t, "call_1", resp.Output[1].CallID)
				assert.Equal(t, "get_weather", resp.Output[1].Name)
			},
		},
		{
			name: "parameter echoing with defaults",
			request: &api.ResponseRequest{
				Model: "gpt-4",
			},
			result: &api.ProviderResult{
				Model: "gpt-4",
				Text:  "response",
			},
			provider: "openai",
			id:       "resp_789",
			validate: func(t *testing.T, resp *api.Response) {
				assert.Equal(t, 1.0, resp.Temperature)
				assert.Equal(t, 1.0, resp.TopP)
				assert.Equal(t, 0.0, resp.PresencePenalty)
				assert.Equal(t, 0.0, resp.FrequencyPenalty)
				assert.Equal(t, 0, resp.TopLogprobs)
				assert.True(t, resp.ParallelToolCalls)
				assert.False(t, resp.Store)
				assert.False(t, resp.Background)
				assert.Equal(t, "disabled", resp.Truncation)
				assert.Equal(t, "default", resp.ServiceTier)
			},
		},
		{
			name: "parameter echoing with custom values",
			request: &api.ResponseRequest{
				Model:             "gpt-4",
				Temperature:       floatPtr(0.7),
				TopP:              floatPtr(0.9),
				PresencePenalty:   floatPtr(0.5),
				FrequencyPenalty:  floatPtr(0.3),
				TopLogprobs:       intPtr(5),
				ParallelToolCalls: boolPtr(false),
				Store:             boolPtr(false),
				Background:        boolPtr(true),
				Truncation:        stringPtr("auto"),
				ServiceTier:       stringPtr("premium"),
			},
			result: &api.ProviderResult{
				Model: "gpt-4",
				Text:  "response",
			},
			provider: "openai",
			id:       "resp_custom",
			validate: func(t *testing.T, resp *api.Response) {
				assert.Equal(t, 0.7, resp.Temperature)
				assert.Equal(t, 0.9, resp.TopP)
				assert.Equal(t, 0.5, resp.PresencePenalty)
				assert.Equal(t, 0.3, resp.FrequencyPenalty)
				assert.Equal(t, 5, resp.TopLogprobs)
				assert.False(t, resp.ParallelToolCalls)
				assert.False(t, resp.Store)
				assert.True(t, resp.Background)
				assert.Equal(t, "auto", resp.Truncation)
				assert.Equal(t, "premium", resp.ServiceTier)
			},
		},
		{
			name: "usage included when text present",
			request: &api.ResponseRequest{
				Model: "gpt-4",
			},
			result: &api.ProviderResult{
				Model: "gpt-4",
				Text:  "response",
				Usage: api.Usage{
					InputTokens:  10,
					OutputTokens: 20,
					TotalTokens:  30,
				},
			},
			provider: "openai",
			id:       "resp_usage",
			validate: func(t *testing.T, resp *api.Response) {
				require.NotNil(t, resp.Usage)
				assert.Equal(t, 10, resp.Usage.InputTokens)
				assert.Equal(t, 20, resp.Usage.OutputTokens)
				assert.Equal(t, 30, resp.Usage.TotalTokens)
			},
		},
		{
			name: "usage always included",
			request: &api.ResponseRequest{
				Model: "gpt-4",
			},
			result: &api.ProviderResult{
				Model: "gpt-4",
				ToolCalls: []api.ToolCall{
					{ID: "call_1", Name: "func", Arguments: "{}"},
				},
			},
			provider: "openai",
			id:       "resp_no_usage",
			validate: func(t *testing.T, resp *api.Response) {
				assert.NotNil(t, resp.Usage)
			},
		},
		{
			name: "instructions prepended",
			request: &api.ResponseRequest{
				Model:        "gpt-4",
				Instructions: stringPtr("Be helpful"),
			},
			result: &api.ProviderResult{
				Model: "gpt-4",
				Text:  "response",
			},
			provider: "openai",
			id:       "resp_instr",
			validate: func(t *testing.T, resp *api.Response) {
				require.NotNil(t, resp.Instructions)
				assert.Equal(t, "Be helpful", *resp.Instructions)
			},
		},
		{
			name: "previous_response_id included",
			request: &api.ResponseRequest{
				Model:              "gpt-4",
				PreviousResponseID: stringPtr("prev_123"),
			},
			result: &api.ProviderResult{
				Model: "gpt-4",
				Text:  "response",
			},
			provider: "openai",
			id:       "resp_prev",
			validate: func(t *testing.T, resp *api.Response) {
				require.NotNil(t, resp.PreviousResponseID)
				assert.Equal(t, "prev_123", *resp.PreviousResponseID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := New(newMockRegistry(), newMockConversationStore(), newMockLogger().asLogger())
			resp := server.buildResponse(tt.request, tt.result, tt.provider, tt.id)

			require.NotNil(t, resp)
			if tt.validate != nil {
				tt.validate(t, resp)
			}
		})
	}
}

func TestHandleResponses_StoresReplayState(t *testing.T) {
	registry := newMockRegistry()
	provider := newMockProvider("anthropic")
	provider.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
		return &api.ProviderResult{
			ID:    "anth_resp_123",
			Model: req.Model,
			Text:  "final answer",
			ReplayMessage: &api.Message{
				Role: "assistant",
				Content: []api.ContentBlock{
					{Type: "anthropic_thinking", Text: "thought", Signature: "sig_123"},
					{Type: "output_text", Text: "final answer"},
				},
			},
		}, nil
	}
	registry.addProvider("anthropic", provider)
	registry.addModel("claude-3", "anthropic")

	store := newMockConversationStore()
	server := New(registry, store, newMockLogger().asLogger())

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"claude-3","input":"hello","store":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleResponses(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Output, 1)

	require.Len(t, store.conversations, 1)
	var stored *conversation.Conversation
	for _, conv := range store.conversations {
		stored = conv
	}
	require.NotNil(t, stored)
	require.NotNil(t, stored.Replay)
	assert.Equal(t, "anthropic", stored.Replay.Provider)
	assert.Equal(t, "anth_resp_123", stored.Replay.ProviderResponseID)
	require.Len(t, stored.Replay.Items, 1)
	assert.Equal(t, resp.Output[0].ID, stored.Replay.Items[0].ID)
	require.NotNil(t, stored.Replay.Items[0].Message)
	assert.Equal(t, "anthropic_thinking", stored.Replay.Items[0].Message.Content[0].Type)
}

func TestHandleResponses_RehydratesReplayStateForSameProvider(t *testing.T) {
	registry := newMockRegistry()
	provider := newMockProvider("anthropic")
	provider.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
		require.Len(t, messages, 3)
		require.Len(t, messages[1].Content, 2)
		assert.Equal(t, "anthropic_thinking", messages[1].Content[0].Type)
		assert.Equal(t, "output_text", messages[1].Content[1].Type)
		return &api.ProviderResult{Model: req.Model, Text: "ok"}, nil
	}
	registry.addProvider("anthropic", provider)
	registry.addModel("claude-3", "anthropic")

	store := newMockConversationStore()
	store.setConversation("prev-123", &conversation.Conversation{
		ID:    "prev-123",
		Model: "claude-3",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hello"}}},
			{Role: "assistant", Content: []api.ContentBlock{{Type: "output_text", Text: "portable"}}},
		},
		Replay: &api.ReplayState{
			Provider: "anthropic",
			Items: []api.ReplayItem{{
				ID:             "msg_123",
				OutputItemType: "message",
				MessageIndex:   1,
				Message: &api.Message{
					Role: "assistant",
					Content: []api.ContentBlock{
						{Type: "anthropic_thinking", Text: "chain", Signature: "sig_123"},
						{Type: "output_text", Text: "portable"},
					},
				},
			}},
		},
	})

	server := New(registry, store, newMockLogger().asLogger())
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"claude-3","input":"follow up","previous_response_id":"prev-123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleResponses(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleResponses_DoesNotRehydrateReplayStateAcrossProviders(t *testing.T) {
	registry := newMockRegistry()
	provider := newMockProvider("openai")
	provider.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
		require.Len(t, messages, 3)
		require.Len(t, messages[1].Content, 1)
		assert.Equal(t, "output_text", messages[1].Content[0].Type)
		return &api.ProviderResult{Model: req.Model, Text: "ok"}, nil
	}
	registry.addProvider("openai", provider)
	registry.addModel("gpt-4", "openai")

	store := newMockConversationStore()
	store.setConversation("prev-123", &conversation.Conversation{
		ID:    "prev-123",
		Model: "gpt-4",
		Messages: []api.Message{
			{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "hello"}}},
			{Role: "assistant", Content: []api.ContentBlock{{Type: "output_text", Text: "portable"}}},
		},
		Replay: &api.ReplayState{
			Provider: "anthropic",
			Items: []api.ReplayItem{{
				ID:             "msg_123",
				OutputItemType: "message",
				MessageIndex:   1,
				Message: &api.Message{
					Role: "assistant",
					Content: []api.ContentBlock{
						{Type: "anthropic_thinking", Text: "chain", Signature: "sig_123"},
						{Type: "output_text", Text: "portable"},
					},
				},
			}},
		},
	})

	server := New(registry, store, newMockLogger().asLogger())
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-4","input":"follow up","previous_response_id":"prev-123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.handleResponses(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestSendSSE(t *testing.T) {
	server := New(newMockRegistry(), newMockConversationStore(), newMockLogger().asLogger())
	rec := newFlushableRecorder()
	seq := 0

	event := &api.StreamEvent{
		Type: "test.event",
	}

	server.sendSSE(rec, rec, &seq, "test.event", event)

	assert.Equal(t, 1, seq)
	assert.Equal(t, 0, event.SequenceNumber)
	body := rec.Body.String()
	assert.Contains(t, body, "event: test.event")
	assert.Contains(t, body, "data:")
	assert.Contains(t, body, `"type":"test.event"`)
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func floatPtr(f float64) *float64 {
	return &f
}

func boolPtr(b bool) *bool {
	return &b
}

// flushableRecorder wraps httptest.ResponseRecorder to support Flusher interface
type flushableRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func newFlushableRecorder() *flushableRecorder {
	return &flushableRecorder{
		ResponseRecorder: httptest.NewRecorder(),
	}
}

func (f *flushableRecorder) Flush() {
	f.flushed++
}

// parseSSEEvents parses Server-Sent Events from a reader
func parseSSEEvents(body io.Reader) ([]api.StreamEvent, error) {
	var events []api.StreamEvent
	scanner := bufio.NewScanner(body)

	var currentEvent string
	var currentData bytes.Buffer

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Empty line marks end of event
			if currentEvent != "" && currentData.Len() > 0 {
				var event api.StreamEvent
				if err := json.Unmarshal(currentData.Bytes(), &event); err != nil {
					return nil, fmt.Errorf("failed to parse event data: %w", err)
				}
				events = append(events, event)
				currentEvent = ""
				currentData.Reset()
			}
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			currentData.WriteString(data)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func TestHandleResponseByID_Get(t *testing.T) {
	tests := []struct {
		name         string
		setupServer  func() *GatewayServer
		setupStore   func(store *mockConversationStore)
		responseID   string
		principal    *auth.Principal
		expectStatus int
		validate     func(t *testing.T, resp *api.Response)
	}{
		{
			name: "retrieve existing response",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addProvider("openai", newMockProvider("openai"))
				registry.addModel("gpt-4", "openai")
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			setupStore: func(store *mockConversationStore) {
				store.setConversation("resp_123", &conversation.Conversation{
					ID:        "resp_123",
					Model:     "gpt-4-turbo",
					CreatedAt: mustParseTime("2024-01-15T10:00:00Z"),
					UpdatedAt: mustParseTime("2024-01-15T10:00:05Z"),
					Messages: []api.Message{
						{Role: "user", Content: []api.ContentBlock{{Type: "input_text", Text: "Hello"}}},
						{Role: "assistant", Content: []api.ContentBlock{{Type: "output_text", Text: "Hi there!"}}},
					},
					Request: &api.ResponseRequest{
						Model:        "gpt-4",
						Instructions: stringPtr("Be helpful"),
						Temperature:  floatPtr(0.7),
					},
				})
			},
			responseID:   "resp_123",
			expectStatus: http.StatusOK,
			validate: func(t *testing.T, resp *api.Response) {
				assert.Equal(t, "resp_123", resp.ID)
				assert.Equal(t, "response", resp.Object)
				assert.Equal(t, "completed", resp.Status)
				assert.Equal(t, "gpt-4-turbo", resp.Model)
				assert.Equal(t, "Be helpful", *resp.Instructions)
				assert.Equal(t, 0.7, resp.Temperature)
				assert.Len(t, resp.Output, 1)
				assert.Equal(t, "message", resp.Output[0].Type)
				assert.Equal(t, "assistant", resp.Output[0].Role)
				assert.Equal(t, "Hi there!", resp.Output[0].Content[0].Text)
			},
		},
		{
			name: "retrieve response with tool calls",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addProvider("openai", newMockProvider("openai"))
				registry.addModel("gpt-4", "openai")
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			setupStore: func(store *mockConversationStore) {
				store.setConversation("resp_tools", &conversation.Conversation{
					ID:        "resp_tools",
					Model:     "gpt-4",
					CreatedAt: mustParseTime("2024-01-15T10:00:00Z"),
					UpdatedAt: mustParseTime("2024-01-15T10:00:05Z"),
					Messages: []api.Message{
						{Role: "assistant", ToolCalls: []api.ToolCall{
							{ID: "call_1", Name: "get_weather", Arguments: `{"location":"NYC"}`},
						}},
					},
				})
			},
			responseID:   "resp_tools",
			expectStatus: http.StatusOK,
			validate: func(t *testing.T, resp *api.Response) {
				assert.Len(t, resp.Output, 1)
				assert.Equal(t, "function_call", resp.Output[0].Type)
				assert.Equal(t, "call_1", resp.Output[0].CallID)
				assert.Equal(t, "get_weather", resp.Output[0].Name)
				assert.JSONEq(t, `{"location":"NYC"}`, resp.Output[0].Arguments)
			},
		},
		{
			name: "retrieve non-existent response returns 404",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addProvider("openai", newMockProvider("openai"))
				return New(registry, newMockConversationStore(), newMockLogger().asLogger())
			},
			setupStore:   func(store *mockConversationStore) {},
			responseID:   "nonexistent",
			expectStatus: http.StatusNotFound,
		},
		{
			name: "retrieve with ownership enforcement - same user",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addProvider("openai", newMockProvider("openai"))
				return New(registry, newMockConversationStore(), newMockLogger().asLogger(), WithAdminConfig(auth.AdminConfig{Enabled: true}))
			},
			setupStore: func(store *mockConversationStore) {
				store.setConversation("resp_owned", &conversation.Conversation{
					ID:        "resp_owned",
					Model:     "gpt-4",
					OwnerIss:  "https://auth.example.com",
					OwnerSub:  "user-1",
					TenantID:  "tenant-a",
					CreatedAt: mustParseTime("2024-01-15T10:00:00Z"),
					UpdatedAt: mustParseTime("2024-01-15T10:00:05Z"),
					Messages: []api.Message{
						{Role: "assistant", Content: []api.ContentBlock{{Type: "output_text", Text: "response"}}},
					},
				})
			},
			responseID: "resp_owned",
			principal: &auth.Principal{
				Issuer:   "https://auth.example.com",
				Subject:  "user-1",
				TenantID: "tenant-a",
			},
			expectStatus: http.StatusOK,
		},
		{
			name: "retrieve with ownership enforcement - different user denied",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addProvider("openai", newMockProvider("openai"))
				return New(registry, newMockConversationStore(), newMockLogger().asLogger(), WithAdminConfig(auth.AdminConfig{Enabled: true}))
			},
			setupStore: func(store *mockConversationStore) {
				store.setConversation("resp_denied", &conversation.Conversation{
					ID:        "resp_denied",
					Model:     "gpt-4",
					OwnerIss:  "https://auth.example.com",
					OwnerSub:  "user-1",
					TenantID:  "tenant-a",
					CreatedAt: mustParseTime("2024-01-15T10:00:00Z"),
					UpdatedAt: mustParseTime("2024-01-15T10:00:05Z"),
					Messages: []api.Message{
						{Role: "assistant", Content: []api.ContentBlock{{Type: "output_text", Text: "response"}}},
					},
				})
			},
			responseID: "resp_denied",
			principal: &auth.Principal{
				Issuer:   "https://auth.example.com",
				Subject:  "user-2",
				TenantID: "tenant-a",
			},
			expectStatus: http.StatusNotFound,
		},
		{
			name: "admin can retrieve any response",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addProvider("openai", newMockProvider("openai"))
				return New(registry, newMockConversationStore(), newMockLogger().asLogger(), WithAdminConfig(auth.AdminConfig{
					Enabled:       true,
					AllowedValues: []string{"admin"},
				}))
			},
			setupStore: func(store *mockConversationStore) {
				store.setConversation("resp_admin", &conversation.Conversation{
					ID:        "resp_admin",
					Model:     "gpt-4",
					OwnerIss:  "https://auth.example.com",
					OwnerSub:  "user-1",
					TenantID:  "tenant-a",
					CreatedAt: mustParseTime("2024-01-15T10:00:00Z"),
					UpdatedAt: mustParseTime("2024-01-15T10:00:05Z"),
					Messages: []api.Message{
						{Role: "assistant", Content: []api.ContentBlock{{Type: "output_text", Text: "response"}}},
					},
				})
			},
			responseID: "resp_admin",
			principal: &auth.Principal{
				Issuer:   "https://auth.example.com",
				Subject:  "admin-user",
				TenantID: "tenant-admin",
				Roles:    []string{"admin"},
			},
			expectStatus: http.StatusOK,
		},
		{
			name: "store error returns 500",
			setupServer: func() *GatewayServer {
				registry := newMockRegistry()
				registry.addProvider("openai", newMockProvider("openai"))
				store := newMockConversationStore()
				store.getErr = fmt.Errorf("database error")
				return New(registry, store, newMockLogger().asLogger())
			},
			setupStore:   func(store *mockConversationStore) {},
			responseID:   "any",
			expectStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()

			// Get the store from the server and configure it
			store := server.convs.(*mockConversationStore)
			tt.setupStore(store)

			req := httptest.NewRequest(http.MethodGet, "/v1/responses/"+tt.responseID, nil)
			if tt.principal != nil {
				req = req.WithContext(auth.ContextWithPrincipal(req.Context(), tt.principal))
			}
			rec := httptest.NewRecorder()

			server.handleResponseByID(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code)

			if tt.validate != nil && rec.Code == http.StatusOK {
				var resp api.Response
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				tt.validate(t, &resp)
			}
		})
	}
}

func TestHandleResponseByID_Delete(t *testing.T) {
	tests := []struct {
		name         string
		setupStore   func(store *mockConversationStore)
		responseID   string
		expectStatus int
		expectSize   int
	}{
		{
			name: "delete existing response",
			setupStore: func(store *mockConversationStore) {
				store.setConversation("resp_del", &conversation.Conversation{
					ID:    "resp_del",
					Model: "gpt-4",
					Messages: []api.Message{
						{Role: "assistant", Content: []api.ContentBlock{{Type: "output_text", Text: "response"}}},
					},
				})
			},
			responseID:   "resp_del",
			expectStatus: http.StatusNoContent,
			expectSize:   0,
		},
		{
			name:         "delete non-existent response returns 404",
			setupStore:   func(store *mockConversationStore) {},
			responseID:   "nonexistent",
			expectStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := newMockRegistry()
			registry.addProvider("openai", newMockProvider("openai"))
			store := newMockConversationStore()
			tt.setupStore(store)
			server := New(registry, store, newMockLogger().asLogger())

			req := httptest.NewRequest(http.MethodDelete, "/v1/responses/"+tt.responseID, nil)
			rec := httptest.NewRecorder()

			server.handleResponseByID(rec, req)

			assert.Equal(t, tt.expectStatus, rec.Code)
			if tt.expectSize > 0 || tt.name == "delete existing response" {
				assert.Equal(t, tt.expectSize, store.Size())
			}
		})
	}
}

func TestHandleResponseByID_MethodNotAllowed(t *testing.T) {
	registry := newMockRegistry()
	registry.addProvider("openai", newMockProvider("openai"))
	server := New(registry, newMockConversationStore(), newMockLogger().asLogger())

	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/v1/responses/resp_123", nil)
			rec := httptest.NewRecorder()

			server.handleResponseByID(rec, req)

			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
