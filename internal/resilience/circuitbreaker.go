// Package resilience provides fault tolerance components for the orchestrator.
//
// V5 adds:
// - Circuit breaker for failure isolation
// - Advanced retry with jitter
package resilience

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half-open"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type Config struct {
	Threshold    int
	ResetTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{
		Threshold:    5,
		ResetTimeout: 30 * time.Second,
	}
}

type CircuitBreaker struct {
	mu           sync.RWMutex
	failures     int
	threshold    int
	state        CircuitState
	lastFailure  time.Time
	resetTimeout time.Duration
	successes    int
}

func NewCircuitBreaker(cfg Config) *CircuitBreaker {
	return &CircuitBreaker{
		failures:     0,
		threshold:    cfg.Threshold,
		state:        CircuitClosed,
		resetTimeout: cfg.ResetTimeout,
		successes:    0,
	}
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.allowRequest() {
		return ErrCircuitOpen
	}

	err := fn()

	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}

	return err
}

func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailure) >= cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.successes = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return true
	}
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == CircuitHalfOpen {
		cb.state = CircuitOpen
	} else if cb.failures >= cb.threshold {
		cb.state = CircuitOpen
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successes++

	if cb.state == CircuitHalfOpen {
		if cb.successes >= 2 {
			cb.state = CircuitClosed
			cb.failures = 0
		}
	} else {
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	Multiplier  float64
	Jitter      bool
	MaxDelay    time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Second,
		Multiplier:  2.0,
		Jitter:      true,
		MaxDelay:    30 * time.Second,
	}
}

func (p RetryPolicy) NextDelay(attempt int) time.Duration {
	delay := p.BaseDelay * time.Duration(pow(p.Multiplier, float64(attempt-1)))
	if delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	if p.Jitter {
		jitter := float64(delay) * 0.3
		random := jitter * (2*rand.Float64() - 1)
		delay = time.Duration(float64(delay) + random)
		if delay < 0 {
			delay = p.BaseDelay
		}
	}
	return delay
}

func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}
