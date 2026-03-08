package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ajac-zero/latticelm/internal/api"
)

// mockProvider is a test double that allows controlling Generate/GenerateStream behavior.
type mockProvider struct {
	name       string
	generateFn func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error)
	streamFn   func(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error)
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Generate(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
	return m.generateFn(ctx, messages, req)
}

func (m *mockProvider) GenerateStream(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
	return m.streamFn(ctx, messages, req)
}

// successStream returns a stream that completes successfully (no error).
func successStream() (<-chan *api.ProviderStreamDelta, <-chan error) {
	deltaChan := make(chan *api.ProviderStreamDelta)
	errChan := make(chan error, 1)
	go func() {
		close(deltaChan)
		errChan <- nil
		close(errChan)
	}()
	return deltaChan, errChan
}

// failStream returns a stream that completes with an error.
func failStream(err error) (<-chan *api.ProviderStreamDelta, <-chan error) {
	deltaChan := make(chan *api.ProviderStreamDelta)
	errChan := make(chan error, 1)
	go func() {
		close(deltaChan)
		errChan <- err
		close(errChan)
	}()
	return deltaChan, errChan
}

// drainStream consumes deltaChan and returns the error from errChan.
func drainStream(deltaChan <-chan *api.ProviderStreamDelta, errChan <-chan error) error {
	for range deltaChan {
	}
	return <-errChan
}

// tripCircuitBreaker forces the circuit breaker open by generating enough failures.
func tripCircuitBreaker(t *testing.T, cbp *CircuitBreakerProvider, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		dc, ec := cbp.GenerateStream(context.Background(), nil, nil)
		err := drainStream(dc, ec)
		assert.Error(t, err)
	}
}

func newTestCBProvider(mock *mockProvider, cfg CircuitBreakerConfig) *CircuitBreakerProvider {
	return NewCircuitBreakerProvider(mock, cfg)
}

// minimalTripConfig returns a CB config that trips quickly for tests.
func minimalTripConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxRequests:  1,
		Interval:     10 * time.Second,
		Timeout:      100 * time.Millisecond, // short open-state timeout for tests
		MinRequests:  2,
		FailureRatio: 0.5,
	}
}

func TestCircuitBreakerStream_OpenStateRejectsNewStreams(t *testing.T) {
	streamErr := errors.New("provider failure")
	mock := &mockProvider{
		name: "test",
		streamFn: func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
			return failStream(streamErr)
		},
	}

	cfg := minimalTripConfig()
	cbp := newTestCBProvider(mock, cfg)

	// Trip the circuit by exhausting MinRequests with failures.
	tripCircuitBreaker(t, cbp, int(cfg.MinRequests))

	// Circuit should now be open; further stream requests must be rejected.
	dc, ec := cbp.GenerateStream(context.Background(), nil, nil)
	err := drainStream(dc, ec)
	require.Error(t, err)
	assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "expected ErrOpenState, got: %v", err)
}

func TestCircuitBreakerGenerate_OpenStateRejectsRequests(t *testing.T) {
	generateErr := errors.New("provider failure")
	mock := &mockProvider{
		name: "test",
		generateFn: func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (*api.ProviderResult, error) {
			return nil, generateErr
		},
		streamFn: func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
			return failStream(generateErr)
		},
	}

	cfg := minimalTripConfig()
	cbp := newTestCBProvider(mock, cfg)

	// Trip via Generate failures.
	for i := 0; i < int(cfg.MinRequests); i++ {
		_, err := cbp.Generate(context.Background(), nil, nil)
		assert.Error(t, err)
	}

	// Circuit open: Generate must be rejected.
	_, err := cbp.Generate(context.Background(), nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "expected ErrOpenState, got: %v", err)
}

func TestCircuitBreakerStream_HalfOpenAllowsLimitedProbe(t *testing.T) {
	// blocker controls when the probe stream completes.
	blocker := make(chan struct{})

	mock := &mockProvider{
		name: "test",
		streamFn: func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
			return failStream(errors.New("failure"))
		},
	}

	cfg := minimalTripConfig() // MaxRequests=1 in half-open
	cbp := newTestCBProvider(mock, cfg)

	// Trip circuit with enough failures.
	tripCircuitBreaker(t, cbp, int(cfg.MinRequests))

	// Wait for open -> half-open transition.
	time.Sleep(cfg.Timeout + 20*time.Millisecond)

	// Switch to a blocking success stream so the probe stays in-flight.
	mock.streamFn = func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
		deltaChan := make(chan *api.ProviderStreamDelta)
		errChan := make(chan error, 1)
		go func() {
			<-blocker // wait until test allows completion
			close(deltaChan)
			errChan <- nil
			close(errChan)
		}()
		return deltaChan, errChan
	}

	// Start probe — MaxRequests=1 so this consumes the half-open slot.
	dc1, ec1 := cbp.GenerateStream(context.Background(), nil, nil)

	// While probe is still in-flight, a concurrent stream must be rejected.
	dc2, ec2 := cbp.GenerateStream(context.Background(), nil, nil)
	err2 := drainStream(dc2, ec2)
	assert.True(t, errors.Is(err2, gobreaker.ErrTooManyRequests) || errors.Is(err2, gobreaker.ErrOpenState),
		"expected rejection in half-open when MaxRequests=1 is exhausted by probe, got: %v", err2)

	// Allow probe to complete and clean up.
	close(blocker)
	err1 := drainStream(dc1, ec1)
	assert.NoError(t, err1)
}

func TestCircuitBreakerStream_SuccessResetsCircuit(t *testing.T) {
	mock := &mockProvider{
		name: "test",
		streamFn: func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
			return failStream(errors.New("failure"))
		},
	}

	cfg := minimalTripConfig()
	cbp := newTestCBProvider(mock, cfg)

	// Trip the circuit.
	tripCircuitBreaker(t, cbp, int(cfg.MinRequests))

	// Verify circuit is open.
	dc, ec := cbp.GenerateStream(context.Background(), nil, nil)
	err := drainStream(dc, ec)
	assert.True(t, errors.Is(err, gobreaker.ErrOpenState))

	// Wait for open -> half-open transition.
	time.Sleep(cfg.Timeout + 20*time.Millisecond)

	// Probe succeeds — circuit should close.
	mock.streamFn = func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
		return successStream()
	}
	dc, ec = cbp.GenerateStream(context.Background(), nil, nil)
	err = drainStream(dc, ec)
	require.NoError(t, err)

	// Circuit should now be closed — subsequent requests pass through.
	dc, ec = cbp.GenerateStream(context.Background(), nil, nil)
	err = drainStream(dc, ec)
	assert.NoError(t, err, "circuit should be closed after successful probe")
}

func TestCircuitBreakerStream_FailedStreamCountsAsFailure(t *testing.T) {
	streamErr := errors.New("stream error")
	mock := &mockProvider{
		name: "test",
		streamFn: func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
			return failStream(streamErr)
		},
	}

	cfg := minimalTripConfig()
	cbp := newTestCBProvider(mock, cfg)

	// Failures should accumulate. After MinRequests, circuit trips.
	for i := 0; i < int(cfg.MinRequests); i++ {
		dc, ec := cbp.GenerateStream(context.Background(), nil, nil)
		err := drainStream(dc, ec)
		assert.ErrorIs(t, err, streamErr)
	}

	// Circuit should be open now.
	dc, ec := cbp.GenerateStream(context.Background(), nil, nil)
	err := drainStream(dc, ec)
	assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "expected circuit to be open after repeated stream failures")
}

func TestCircuitBreakerStream_SuccessfulStreamCountsAsSuccess(t *testing.T) {
	mock := &mockProvider{
		name: "test",
		streamFn: func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
			return successStream()
		},
	}

	cfg := minimalTripConfig()
	cbp := newTestCBProvider(mock, cfg)

	// Many successful streams should keep the circuit closed.
	for i := 0; i < 10; i++ {
		dc, ec := cbp.GenerateStream(context.Background(), nil, nil)
		err := drainStream(dc, ec)
		assert.NoError(t, err)
	}

	// Circuit must remain closed.
	assert.Equal(t, gobreaker.StateClosed, cbp.cb.State())
}

func TestNewCircuitBreakerProvider_PerProviderConfig(t *testing.T) {
	mock := &mockProvider{
		name: "custom",
		generateFn: func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (*api.ProviderResult, error) {
			return &api.ProviderResult{}, nil
		},
		streamFn: func(_ context.Context, _ []api.Message, _ *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
			return successStream()
		},
	}

	cfg := CircuitBreakerConfig{
		MaxRequests:  5,
		Interval:     15 * time.Second,
		Timeout:      45 * time.Second,
		MinRequests:  10,
		FailureRatio: 0.8,
	}

	cbp := newTestCBProvider(mock, cfg)
	require.NotNil(t, cbp)
	assert.Equal(t, "custom", cbp.Name())
	assert.Equal(t, gobreaker.StateClosed, cbp.cb.State())
}
