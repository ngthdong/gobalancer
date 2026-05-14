package pool

import "sync/atomic"

// Backend represents a single upstream server.
type Backend struct {
    Addr string
    healthy     atomic.Bool  
    activeConns atomic.Int64 
}

func NewBackend(addr string) *Backend {
    b := &Backend{Addr: addr}
    b.healthy.Store(true) 
    return b
}

func (b *Backend) IsHealthy() bool {
    return b.healthy.Load()
}

func (b *Backend) TrackConn(delta int64) {
    b.activeConns.Add(delta)
}

func (b *Backend) ActiveConns() int64 {
    return b.activeConns.Load()
}