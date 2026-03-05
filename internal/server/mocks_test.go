package server

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"unsafe"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/ajac-zero/latticelm/internal/config"
	"github.com/ajac-zero/latticelm/internal/conversation"
	"github.com/ajac-zero/latticelm/internal/providers"
)

// mockProvider implements providers.Provider for testing
type mockProvider struct {
	name           string
	generateFunc   func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error)
	streamFunc     func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error)
	generateCalled int
	streamCalled   int
	mu             sync.Mutex
}

func newMockProvider(name string) *mockProvider {
	return &mockProvider{
		name: name,
	}
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Generate(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
	m.mu.Lock()
	m.generateCalled++
	m.mu.Unlock()

	if m.generateFunc != nil {
		return m.generateFunc(ctx, messages, req)
	}
	return &api.ProviderResult{
		ID:    "mock-id",
		Model: req.Model,
		Text:  "mock response",
		Usage: api.Usage{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	}, nil
}

func (m *mockProvider) GenerateStream(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
	m.mu.Lock()
	m.streamCalled++
	m.mu.Unlock()

	if m.streamFunc != nil {
		return m.streamFunc(ctx, messages, req)
	}

	// Default behavior: send a simple text stream
	deltaChan := make(chan *api.ProviderStreamDelta, 3)
	errChan := make(chan error, 1)

	go func() {
		defer close(deltaChan)
		defer close(errChan)

		deltaChan <- &api.ProviderStreamDelta{
			Model: req.Model,
			Text:  "Hello",
		}
		deltaChan <- &api.ProviderStreamDelta{
			Text: " world",
		}
		deltaChan <- &api.ProviderStreamDelta{
			Done: true,
		}
	}()

	return deltaChan, errChan
}

func (m *mockProvider) getGenerateCalled() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.generateCalled
}

func (m *mockProvider) getStreamCalled() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streamCalled
}

// buildTestRegistry creates a providers.Registry for testing with mock providers
// Uses reflection to inject mock providers into the registry
func buildTestRegistry(mockProviders map[string]providers.Provider, modelConfigs []config.ModelEntry) *providers.Registry {
	// Create empty registry
	reg := &providers.Registry{}

	// Use reflection to set private fields
	regValue := reflect.ValueOf(reg).Elem()

	// Set providers field
	providersField := regValue.FieldByName("providers")
	providersPtr := unsafe.Pointer(providersField.UnsafeAddr())
	*(*map[string]providers.Provider)(providersPtr) = mockProviders

	// Set modelList field
	modelListField := regValue.FieldByName("modelList")
	modelListPtr := unsafe.Pointer(modelListField.UnsafeAddr())
	*(*[]config.ModelEntry)(modelListPtr) = modelConfigs

	// Set models map (model name -> provider name)
	modelsField := regValue.FieldByName("models")
	modelsPtr := unsafe.Pointer(modelsField.UnsafeAddr())
	modelsMap := make(map[string]string)
	for _, m := range modelConfigs {
		modelsMap[m.Name] = m.Provider
	}
	*(*map[string]string)(modelsPtr) = modelsMap

	// Set providerModelIDs map
	providerModelIDsField := regValue.FieldByName("providerModelIDs")
	providerModelIDsPtr := unsafe.Pointer(providerModelIDsField.UnsafeAddr())
	providerModelIDsMap := make(map[string]string)
	for _, m := range modelConfigs {
		if m.ProviderModelID != "" {
			providerModelIDsMap[m.Name] = m.ProviderModelID
		}
	}
	*(*map[string]string)(providerModelIDsPtr) = providerModelIDsMap

	return reg
}

// mockConversationStore implements conversation.Store for testing
type mockConversationStore struct {
	conversations map[string]*conversation.Conversation
	createErr     error
	getErr        error
	appendErr     error
	deleteErr     error
	mu            sync.Mutex
}

func newMockConversationStore() *mockConversationStore {
	return &mockConversationStore{
		conversations: make(map[string]*conversation.Conversation),
	}
}

func (m *mockConversationStore) Get(ctx context.Context, id string) (*conversation.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getErr != nil {
		return nil, m.getErr
	}
	conv, ok := m.conversations[id]
	if !ok {
		return nil, nil
	}
	return conv, nil
}

func (m *mockConversationStore) Create(ctx context.Context, id string, model string, messages []api.Message) (*conversation.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createErr != nil {
		return nil, m.createErr
	}

	conv := &conversation.Conversation{
		ID:       id,
		Model:    model,
		Messages: messages,
	}
	m.conversations[id] = conv
	return conv, nil
}

func (m *mockConversationStore) Append(ctx context.Context, id string, messages ...api.Message) (*conversation.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.appendErr != nil {
		return nil, m.appendErr
	}

	conv, ok := m.conversations[id]
	if !ok {
		return nil, nil
	}
	conv.Messages = append(conv.Messages, messages...)
	return conv, nil
}

func (m *mockConversationStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.conversations, id)
	return nil
}

func (m *mockConversationStore) Size() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.conversations)
}

func (m *mockConversationStore) Close() error {
	return nil
}

func (m *mockConversationStore) setConversation(id string, conv *conversation.Conversation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conversations[id] = conv
}

// mockLogger captures log output for testing
type mockLogger struct {
	logs []string
	mu   sync.Mutex
}

func newMockLogger() *mockLogger {
	return &mockLogger{
		logs: []string{},
	}
}

func (m *mockLogger) Printf(format string, args ...interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, fmt.Sprintf(format, args...))
}

func (m *mockLogger) getLogs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.logs...)
}

func (m *mockLogger) asLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(m, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

func (m *mockLogger) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, string(p))
	return len(p), nil
}

// mockRegistry is a simple mock for providers.Registry
type mockRegistry struct {
	providers map[string]providers.Provider
	models    map[string]string // model name -> provider name
	mu        sync.RWMutex
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		providers: make(map[string]providers.Provider),
		models:    make(map[string]string),
	}
}

func (m *mockRegistry) Get(name string) (providers.Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[name]
	return p, ok
}

func (m *mockRegistry) Default(model string) (providers.Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	providerName, ok := m.models[model]
	if !ok {
		return nil, fmt.Errorf("no provider configured for model %s", model)
	}

	p, ok := m.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %s not found", providerName)
	}
	return p, nil
}

func (m *mockRegistry) Models() []struct{ Provider, Model string } {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var models []struct{ Provider, Model string }
	for modelName, providerName := range m.models {
		models = append(models, struct{ Provider, Model string }{
			Model:    modelName,
			Provider: providerName,
		})
	}
	return models
}

func (m *mockRegistry) ResolveModelID(model string) string {
	// Simple implementation - just return the model name as-is
	return model
}

func (m *mockRegistry) addProvider(name string, provider providers.Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[name] = provider
}

func (m *mockRegistry) addModel(model, provider string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.models[model] = provider
}
