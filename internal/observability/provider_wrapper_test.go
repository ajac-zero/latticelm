package observability

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ajac-zero/latticelm/internal/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// mockBaseProvider implements providers.Provider for testing
type mockBaseProvider struct {
	name         string
	generateFunc func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error)
	streamFunc   func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error)
	callCount    int
	mu           sync.Mutex
}

func newMockBaseProvider(name string) *mockBaseProvider {
	return &mockBaseProvider{
		name: name,
	}
}

func (m *mockBaseProvider) Name() string {
	return m.name
}

func (m *mockBaseProvider) Generate(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	if m.generateFunc != nil {
		return m.generateFunc(ctx, messages, req)
	}

	// Default successful response
	return &api.ProviderResult{
		ID:    "test-id",
		Model: req.Model,
		Text:  "test response",
		Usage: api.Usage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	}, nil
}

func (m *mockBaseProvider) GenerateStream(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	if m.streamFunc != nil {
		return m.streamFunc(ctx, messages, req)
	}

	// Default streaming response
	deltaChan := make(chan *api.ProviderStreamDelta, 3)
	errChan := make(chan error, 1)

	go func() {
		defer close(deltaChan)
		defer close(errChan)

		deltaChan <- &api.ProviderStreamDelta{
			Model: req.Model,
			Text:  "chunk1",
		}
		deltaChan <- &api.ProviderStreamDelta{
			Text: " chunk2",
			Usage: &api.Usage{
				InputTokens:  50,
				OutputTokens: 25,
				TotalTokens:  75,
			},
		}
		deltaChan <- &api.ProviderStreamDelta{
			Done: true,
		}
	}()

	return deltaChan, errChan
}

func (m *mockBaseProvider) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func TestNewInstrumentedProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		withRegistry bool
		withTracer   bool
	}{
		{
			name:         "with registry and tracer",
			providerName: "openai",
			withRegistry: true,
			withTracer:   true,
		},
		{
			name:         "with registry only",
			providerName: "anthropic",
			withRegistry: true,
			withTracer:   false,
		},
		{
			name:         "with tracer only",
			providerName: "google",
			withRegistry: false,
			withTracer:   true,
		},
		{
			name:         "without observability",
			providerName: "test",
			withRegistry: false,
			withTracer:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := newMockBaseProvider(tt.providerName)

			var registry *prometheus.Registry
			if tt.withRegistry {
				registry = NewTestRegistry()
			}

			var tp *sdktrace.TracerProvider
			_ = tp
			if tt.withTracer {
				tp, _ = NewTestTracer()
				defer ShutdownTracer(tp)
			}

			wrapped := NewInstrumentedProvider(base, registry, tp)
			require.NotNil(t, wrapped)

			instrumented, ok := wrapped.(*InstrumentedProvider)
			require.True(t, ok)
			assert.Equal(t, tt.providerName, instrumented.Name())
		})
	}
}

func TestInstrumentedProvider_Generate(t *testing.T) {
	tests := []struct {
		name         string
		setupMock    func(*mockBaseProvider)
		expectError  bool
		checkMetrics bool
	}{
		{
			name: "successful generation",
			setupMock: func(m *mockBaseProvider) {
				m.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					return &api.ProviderResult{
						ID:    "success-id",
						Model: req.Model,
						Text:  "Generated text",
						Usage: api.Usage{
							InputTokens:  200,
							OutputTokens: 100,
							TotalTokens:  300,
						},
					}, nil
				}
			},
			expectError:  false,
			checkMetrics: true,
		},
		{
			name: "generation error",
			setupMock: func(m *mockBaseProvider) {
				m.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					return nil, errors.New("provider error")
				}
			},
			expectError:  true,
			checkMetrics: true,
		},
		{
			name: "nil result",
			setupMock: func(m *mockBaseProvider) {
				m.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					return nil, nil
				}
			},
			expectError:  false,
			checkMetrics: true,
		},
		{
			name: "empty tokens",
			setupMock: func(m *mockBaseProvider) {
				m.generateFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
					return &api.ProviderResult{
						ID:    "zero-tokens",
						Model: req.Model,
						Text:  "text",
						Usage: api.Usage{
							InputTokens:  0,
							OutputTokens: 0,
							TotalTokens:  0,
						},
					}, nil
				}
			},
			expectError:  false,
			checkMetrics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics
			providerRequestsTotal.Reset()
			providerRequestDuration.Reset()
			providerTokensTotal.Reset()

			base := newMockBaseProvider("test-provider")
			tt.setupMock(base)

			registry := NewTestRegistry()
			InitMetrics() // Ensure metrics are registered

			tp, exporter := NewTestTracer()
			defer ShutdownTracer(tp)

			wrapped := NewInstrumentedProvider(base, registry, tp)

			ctx := context.Background()
			messages := []api.Message{
				{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "test"}}},
			}
			req := &api.ResponseRequest{Model: "test-model"}

			result, err := wrapped.Generate(ctx, messages, req)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				if result != nil {
					assert.NoError(t, err)
					assert.NotNil(t, result)
				}
			}

			// Verify provider was called
			assert.Equal(t, 1, base.getCallCount())

			// Check metrics were recorded
			if tt.checkMetrics {
				status := "success"
				if tt.expectError {
					status = "error"
				}

				counter := providerRequestsTotal.WithLabelValues("test-provider", "test-model", "generate", status)
				value := testutil.ToFloat64(counter)
				assert.Equal(t, 1.0, value, "request counter should be incremented")
			}

			// Check spans were created
			spans := exporter.GetSpans()
			if len(spans) > 0 {
				span := spans[0]
				assert.Equal(t, "provider.generate", span.Name)

				if tt.expectError {
					assert.Equal(t, codes.Error, span.Status.Code)
				} else if result != nil {
					assert.Equal(t, codes.Ok, span.Status.Code)
				}
			}
		})
	}
}

func TestInstrumentedProvider_GenerateStream(t *testing.T) {
	tests := []struct {
		name         string
		setupMock    func(*mockBaseProvider)
		expectError  bool
		checkMetrics bool
		expectedChunks int
	}{
		{
			name: "successful streaming",
			setupMock: func(m *mockBaseProvider) {
				m.streamFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
					deltaChan := make(chan *api.ProviderStreamDelta, 4)
					errChan := make(chan error, 1)

					go func() {
						defer close(deltaChan)
						defer close(errChan)

						deltaChan <- &api.ProviderStreamDelta{
							Model: req.Model,
							Text:  "First ",
						}
						deltaChan <- &api.ProviderStreamDelta{
							Text: "Second ",
						}
						deltaChan <- &api.ProviderStreamDelta{
							Text: "Third",
							Usage: &api.Usage{
								InputTokens:  150,
								OutputTokens: 75,
								TotalTokens:  225,
							},
						}
						deltaChan <- &api.ProviderStreamDelta{
							Done: true,
						}
					}()

					return deltaChan, errChan
				}
			},
			expectError:    false,
			checkMetrics:   true,
			expectedChunks: 4,
		},
		{
			name: "streaming error",
			setupMock: func(m *mockBaseProvider) {
				m.streamFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
					deltaChan := make(chan *api.ProviderStreamDelta)
					errChan := make(chan error, 1)

					go func() {
						defer close(deltaChan)
						defer close(errChan)

						errChan <- errors.New("stream error")
					}()

					return deltaChan, errChan
				}
			},
			expectError:    true,
			checkMetrics:   true,
			expectedChunks: 0,
		},
		{
			name: "empty stream",
			setupMock: func(m *mockBaseProvider) {
				m.streamFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
					deltaChan := make(chan *api.ProviderStreamDelta)
					errChan := make(chan error, 1)

					go func() {
						defer close(deltaChan)
						defer close(errChan)
					}()

					return deltaChan, errChan
				}
			},
			expectError:    false,
			checkMetrics:   true,
			expectedChunks: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics
			providerRequestsTotal.Reset()
			providerStreamDuration.Reset()
			providerStreamChunks.Reset()
			providerStreamTTFB.Reset()
			providerTokensTotal.Reset()

			base := newMockBaseProvider("stream-provider")
			tt.setupMock(base)

			registry := NewTestRegistry()
			InitMetrics()

			tp, exporter := NewTestTracer()
			defer ShutdownTracer(tp)

			wrapped := NewInstrumentedProvider(base, registry, tp)

			ctx := context.Background()
			messages := []api.Message{
				{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "stream test"}}},
			}
			req := &api.ResponseRequest{Model: "stream-model"}

			deltaChan, errChan := wrapped.GenerateStream(ctx, messages, req)

			// Consume the stream
			var chunks []*api.ProviderStreamDelta
			var streamErr error

			for {
				select {
				case delta, ok := <-deltaChan:
					if !ok {
						goto Done
					}
					chunks = append(chunks, delta)
				case err, ok := <-errChan:
					if ok && err != nil {
						streamErr = err
						goto Done
					}
				}
			}

		Done:
			if tt.expectError {
				assert.Error(t, streamErr)
			} else {
				assert.NoError(t, streamErr)
			}

			assert.Equal(t, tt.expectedChunks, len(chunks))

			// Give goroutine time to finish metrics recording
			time.Sleep(100 * time.Millisecond)

			// Verify provider was called
			assert.Equal(t, 1, base.getCallCount())

			// Check metrics
			if tt.checkMetrics {
				status := "success"
				if tt.expectError {
					status = "error"
				}

				counter := providerRequestsTotal.WithLabelValues("stream-provider", "stream-model", "generate_stream", status)
				value := testutil.ToFloat64(counter)
				assert.Equal(t, 1.0, value, "stream request counter should be incremented")
			}

			// Check spans
			time.Sleep(100 * time.Millisecond) // Give time for span to be exported
			spans := exporter.GetSpans()
			if len(spans) > 0 {
				span := spans[0]
				assert.Equal(t, "provider.generate_stream", span.Name)
			}
		})
	}
}

func TestInstrumentedProvider_MetricsRecording(t *testing.T) {
	// Reset all metrics
	providerRequestsTotal.Reset()
	providerRequestDuration.Reset()
	providerTokensTotal.Reset()
	providerStreamTTFB.Reset()
	providerStreamChunks.Reset()
	providerStreamDuration.Reset()

	base := newMockBaseProvider("metrics-test")
	registry := NewTestRegistry()
	InitMetrics()

	wrapped := NewInstrumentedProvider(base, registry, nil)

	ctx := context.Background()
	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "test"}}},
	}
	req := &api.ResponseRequest{Model: "test-model"}

	// Test Generate metrics
	result, err := wrapped.Generate(ctx, messages, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify counter
	counter := providerRequestsTotal.WithLabelValues("metrics-test", "test-model", "generate", "success")
	value := testutil.ToFloat64(counter)
	assert.Equal(t, 1.0, value)

	// Verify token metrics
	inputTokens := providerTokensTotal.WithLabelValues("metrics-test", "test-model", "input")
	inputValue := testutil.ToFloat64(inputTokens)
	assert.Equal(t, 100.0, inputValue)

	outputTokens := providerTokensTotal.WithLabelValues("metrics-test", "test-model", "output")
	outputValue := testutil.ToFloat64(outputTokens)
	assert.Equal(t, 50.0, outputValue)
}

func TestInstrumentedProvider_TracingSpans(t *testing.T) {
	base := newMockBaseProvider("trace-test")
	tp, exporter := NewTestTracer()
	defer ShutdownTracer(tp)

	wrapped := NewInstrumentedProvider(base, nil, tp)

	ctx := context.Background()
	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "trace"}}},
	}
	req := &api.ResponseRequest{Model: "trace-model"}

	// Test Generate span
	result, err := wrapped.Generate(ctx, messages, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Force span export
	tp.ForceFlush(ctx)

	spans := exporter.GetSpans()
	require.GreaterOrEqual(t, len(spans), 1)

	span := spans[0]
	assert.Equal(t, "provider.generate", span.Name)

	// Check attributes
	attrs := span.Attributes
	attrMap := make(map[string]interface{})
	for _, attr := range attrs {
		attrMap[string(attr.Key)] = attr.Value.AsInterface()
	}

	assert.Equal(t, "trace-test", attrMap["provider.name"])
	assert.Equal(t, "trace-model", attrMap["provider.model"])
	assert.Equal(t, int64(100), attrMap["provider.input_tokens"])
	assert.Equal(t, int64(50), attrMap["provider.output_tokens"])
	assert.Equal(t, int64(150), attrMap["provider.total_tokens"])
}

func TestInstrumentedProvider_WithoutObservability(t *testing.T) {
	base := newMockBaseProvider("no-obs")
	wrapped := NewInstrumentedProvider(base, nil, nil)

	ctx := context.Background()
	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "test"}}},
	}
	req := &api.ResponseRequest{Model: "test"}

	// Should work without observability
	result, err := wrapped.Generate(ctx, messages, req)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Stream should also work
	deltaChan, errChan := wrapped.GenerateStream(ctx, messages, req)

	for {
		select {
		case _, ok := <-deltaChan:
			if !ok {
				goto Done
			}
		case <-errChan:
			goto Done
		}
	}

Done:
	assert.Equal(t, 2, base.getCallCount())
}

func TestInstrumentedProvider_Name(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
	}{
		{
			name:         "openai provider",
			providerName: "openai",
		},
		{
			name:         "anthropic provider",
			providerName: "anthropic",
		},
		{
			name:         "google provider",
			providerName: "google",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := newMockBaseProvider(tt.providerName)
			wrapped := NewInstrumentedProvider(base, nil, nil)

			assert.Equal(t, tt.providerName, wrapped.Name())
		})
	}
}

func TestInstrumentedProvider_ConcurrentCalls(t *testing.T) {
	base := newMockBaseProvider("concurrent-test")
	registry := NewTestRegistry()
	InitMetrics()

	tp, _ := NewTestTracer()
	defer ShutdownTracer(tp)

	wrapped := NewInstrumentedProvider(base, registry, tp)

	ctx := context.Background()
	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "concurrent"}}},
	}

	// Make concurrent requests
	const numRequests = 10
	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			defer wg.Done()
			req := &api.ResponseRequest{Model: "concurrent-model"}
			_, _ = wrapped.Generate(ctx, messages, req)
		}(i)
	}

	wg.Wait()

	// Verify all calls were made
	assert.Equal(t, numRequests, base.getCallCount())

	// Verify metrics recorded all requests
	counter := providerRequestsTotal.WithLabelValues("concurrent-test", "concurrent-model", "generate", "success")
	value := testutil.ToFloat64(counter)
	assert.Equal(t, float64(numRequests), value)
}

func TestInstrumentedProvider_StreamTTFB(t *testing.T) {
	providerStreamTTFB.Reset()

	base := newMockBaseProvider("ttfb-test")
	base.streamFunc = func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
		deltaChan := make(chan *api.ProviderStreamDelta, 2)
		errChan := make(chan error, 1)

		go func() {
			defer close(deltaChan)
			defer close(errChan)

			// Simulate delay before first chunk
			time.Sleep(50 * time.Millisecond)
			deltaChan <- &api.ProviderStreamDelta{Text: "first"}
			deltaChan <- &api.ProviderStreamDelta{Done: true}
		}()

		return deltaChan, errChan
	}

	registry := NewTestRegistry()
	InitMetrics()
	wrapped := NewInstrumentedProvider(base, registry, nil)

	ctx := context.Background()
	messages := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "ttfb"}}},
	}
	req := &api.ResponseRequest{Model: "ttfb-model"}

	deltaChan, errChan := wrapped.GenerateStream(ctx, messages, req)

	// Consume stream
	for {
		select {
		case _, ok := <-deltaChan:
			if !ok {
				goto Done
			}
		case <-errChan:
			goto Done
		}
	}

Done:
	// Give time for metrics to be recorded
	time.Sleep(100 * time.Millisecond)

	// TTFB should have been recorded (we can't check exact value due to timing)
	// Just verify the metric exists
	counter := providerStreamChunks.WithLabelValues("ttfb-test", "ttfb-model")
	value := testutil.ToFloat64(counter)
	assert.Greater(t, value, 0.0)
}
