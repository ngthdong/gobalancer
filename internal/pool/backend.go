package pool

import (
	"sync/atomic"
	"time"
)

// Backend represents a single upstream server.
type BackendState int32

const (
	StateActive BackendState = iota
	StateDraining
	StateRemoved
)

type Backend struct {
	Addr                string
	healthy             atomic.Bool
	activeConns         atomic.Int64
	state               atomic.Int32
	consecutiveFailures atomic.Int32
	lastChecked         atomic.Int64
}

func NewBackend(addr string) *Backend {
	b := &Backend{Addr: addr}
	b.healthy.Store(true)
	b.state.Store(int32(StateActive))
	return b
}

func (b *Backend) IsHealthy() bool {
	return b.healthy.Load() &&
		BackendState(b.state.Load()) == StateActive
}

func (b *Backend) SetHealthy(healthy bool) {
	if healthy {
		b.consecutiveFailures.Store(0)
	}
	b.healthy.Store(healthy)
	b.lastChecked.Store(time.Now().UnixNano())
}

func (b *Backend) TrackConn(delta int64) {
	b.activeConns.Add(delta)
}

func (b *Backend) ActiveConns() int64 {
	return b.activeConns.Load()
}

func (b *Backend) ConsecutiveFailures() int32 {
	return b.consecutiveFailures.Load()
}

func (b *Backend) LastChecked() time.Time {
	ns := b.lastChecked.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

func (b *Backend) IncrementFailures() int32 {
	return b.consecutiveFailures.Add(1)
}
