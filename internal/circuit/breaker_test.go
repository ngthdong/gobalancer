package circuit_test

import (
	"sync"
	"testing"
	"time"

	"github.com/ngthdong/gobalancer/internal/circuit"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/stretchr/testify/assert"
)

func newBreaker(threshold int, timeout time.Duration) *circuit.Breaker {
	cfg := &config.Config{}
	cfg.CircuitBreaker.FailureThreshold = threshold
	cfg.CircuitBreaker.Timeout = timeout

	return circuit.NewBreaker(cfg)
}

func TestBreaker_InitialState(t *testing.T) {
	b := newBreaker(3, time.Second)

	assert.Equal(t, circuit.StateClosed, b.State())
	assert.True(t, b.Allow())
}

func TestBreaker_OpensAfterFailureThreshold(t *testing.T) {
	b := newBreaker(3, time.Second)

	b.RecordFailure()
	b.RecordFailure()

	assert.Equal(t, circuit.StateClosed, b.State())
	assert.True(t, b.Allow())

	b.RecordFailure()

	assert.Equal(t, circuit.StateOpen, b.State())
	assert.False(t, b.Allow())
}

func TestBreaker_TransitionsToHalfOpenAfterTimeout(t *testing.T) {
	b := newBreaker(1, 50*time.Millisecond)

	b.RecordFailure()

	assert.Equal(t, circuit.StateOpen, b.State())
	assert.False(t, b.Allow())

	time.Sleep(60 * time.Millisecond)

	assert.True(t, b.Allow())
	assert.Equal(t, circuit.StateHalfOpen, b.State())
}

func TestBreaker_AllowsOnlySingleProbeInHalfOpen(t *testing.T) {
	b := newBreaker(1, 50*time.Millisecond)

	b.RecordFailure()

	time.Sleep(60 * time.Millisecond)

	// first probe allowed
	assert.True(t, b.Allow())

	// subsequent probes rejected
	assert.False(t, b.Allow())
	assert.False(t, b.Allow())

	assert.Equal(t, circuit.StateHalfOpen, b.State())
}

func TestBreaker_ClosesAfterSuccessfulProbe(t *testing.T) {
	b := newBreaker(1, 50*time.Millisecond)

	b.RecordFailure()

	time.Sleep(60 * time.Millisecond)

	assert.True(t, b.Allow())

	b.RecordSuccess()

	assert.Equal(t, circuit.StateClosed, b.State())
	assert.True(t, b.Allow())
}

func TestBreaker_ReopensAfterFailedProbe(t *testing.T) {
	b := newBreaker(1, 50*time.Millisecond)

	b.RecordFailure()

	time.Sleep(60 * time.Millisecond)

	assert.True(t, b.Allow())

	b.RecordFailure()

	assert.Equal(t, circuit.StateOpen, b.State())
	assert.False(t, b.Allow())
}

func TestBreaker_RecordSuccessResetsFailures(t *testing.T) {
	b := newBreaker(3, time.Second)

	b.RecordFailure()
	b.RecordFailure()

	b.RecordSuccess()

	assert.Equal(t, circuit.StateClosed, b.State())

	// should not open after one more failure
	b.RecordFailure()

	assert.Equal(t, circuit.StateClosed, b.State())
}

func TestBreaker_ConcurrentSafety(t *testing.T) {
	b := newBreaker(10, time.Second)

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			b.Allow()
			b.RecordFailure()
			b.RecordSuccess()
			_ = b.State()
		}()
	}

	wg.Wait()

	state := b.State()

	assert.Contains(t,
		[]circuit.State{
			circuit.StateClosed,
			circuit.StateOpen,
			circuit.StateHalfOpen,
		},
		state,
	)
}
