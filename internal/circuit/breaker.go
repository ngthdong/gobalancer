package circuit

import (
	"sync"
	"time"

	"github.com/ngthdong/gobalancer/internal/config"
)

type State int32

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

type Breaker struct {
	mu       sync.Mutex
	state    State
	failures int
	lastFail time.Time
	cfg      *config.Config
}

func NewBreaker(cfg *config.Config) *Breaker {
	return &Breaker{
		state: StateClosed,
		cfg:   cfg,
	}
}

func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true

	case StateOpen:
		if time.Since(b.lastFail) >= b.cfg.CircuitBreaker.Timeout {
			b.state = StateHalfOpen
			return true
		}
		return false

	case StateHalfOpen:
		// In HalfOpen, only one probe is allowed at a time.
		// Subsequent requests are rejected until the probe resolves.
		// This prevents a flood of probes overwhelming a recovering backend.
		return false
	}

	return false
}

func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateHalfOpen:
		b.state = StateClosed
		b.failures = 0

	case StateClosed:
		b.failures = 0
	}
}

func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures++
	b.lastFail = time.Now()

	switch b.state {
	case StateClosed:
		if b.failures >= b.cfg.CircuitBreaker.FailureThreshold {
			b.state = StateOpen
		}

	case StateHalfOpen:
		b.state = StateOpen
	}
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}
