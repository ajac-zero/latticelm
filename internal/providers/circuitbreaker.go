package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/sony/gobreaker"

	"github.com/ajac-zero/latticelm/internal/api"
)

// CircuitBreakerProvider wraps a Provider with circuit breaker functionality.
type CircuitBreakerProvider struct {
	provider Provider
	cb       *gobreaker.TwoStepCircuitBreaker
}

// CircuitBreakerConfig holds configuration for the circuit breaker.
type CircuitBreakerConfig struct {
	// MaxRequests is the maximum number of requests allowed to pass through
	// when the circuit breaker is half-open. Default: 3
	MaxRequests uint32

	// Interval is the cyclic period of the closed state for the circuit breaker
	// to clear the internal Counts. Default: 30s
	Interval time.Duration

	// Timeout is the period of the open state, after which the state becomes half-open.
	// Default: 60s
	Timeout time.Duration

	// MinRequests is the minimum number of requests needed before evaluating failure ratio.
	// Default: 5
	MinRequests uint32

	// FailureRatio is the ratio of failures that will trip the circuit breaker.
	// Default: 0.5 (50%)
	FailureRatio float64

	// OnStateChange is an optional callback invoked when circuit breaker state changes.
	// Parameters: provider name, from state, to state
	OnStateChange func(provider, from, to string)
}

// DefaultCircuitBreakerConfig returns a sensible default configuration.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxRequests:  3,
		Interval:     30 * time.Second,
		Timeout:      60 * time.Second,
		MinRequests:  5,
		FailureRatio: 0.5,
	}
}

// NewCircuitBreakerProvider wraps a provider with circuit breaker functionality.
func NewCircuitBreakerProvider(provider Provider, cfg CircuitBreakerConfig) *CircuitBreakerProvider {
	providerName := provider.Name()

	settings := gobreaker.Settings{
		Name:        fmt.Sprintf("%s-circuit-breaker", providerName),
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Only trip if we have enough requests to be statistically meaningful
			if counts.Requests < cfg.MinRequests {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cfg.FailureRatio
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// Call the callback if provided
			if cfg.OnStateChange != nil {
				cfg.OnStateChange(providerName, from.String(), to.String())
			}
		},
	}

	return &CircuitBreakerProvider{
		provider: provider,
		cb:       gobreaker.NewTwoStepCircuitBreaker(settings),
	}
}

// Name returns the underlying provider name.
func (p *CircuitBreakerProvider) Name() string {
	return p.provider.Name()
}

// Generate wraps the provider's Generate method with circuit breaker protection.
func (p *CircuitBreakerProvider) Generate(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (*api.ProviderResult, error) {
	done, err := p.cb.Allow()
	if err != nil {
		return nil, err
	}

	result, providerErr := p.provider.Generate(ctx, messages, req)
	done(providerErr == nil)
	return result, providerErr
}

// GenerateStream wraps the provider's GenerateStream method with circuit breaker protection.
// It uses TwoStepCircuitBreaker.Allow() to properly gate stream initiation (including
// half-open concurrency control), then records success or failure when the stream completes.
func (p *CircuitBreakerProvider) GenerateStream(ctx context.Context, messages []api.Message, req *api.ResponseRequest) (<-chan *api.ProviderStreamDelta, <-chan error) {
	done, err := p.cb.Allow()
	if err != nil {
		errChan := make(chan error, 1)
		deltaChan := make(chan *api.ProviderStreamDelta)
		errChan <- err
		close(deltaChan)
		close(errChan)
		return deltaChan, errChan
	}

	deltaChan, errChan := p.provider.GenerateStream(ctx, messages, req)

	wrappedErrChan := make(chan error, 1)
	go func() {
		defer close(wrappedErrChan)
		if streamErr := <-errChan; streamErr != nil {
			done(false)
			wrappedErrChan <- streamErr
		} else {
			done(true)
		}
	}()

	return deltaChan, wrappedErrChan
}
