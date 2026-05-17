package balancer

import (
	"sync/atomic"

	"github.com/ngthdong/gobalancer/internal/pool"
)

type RoundRobin struct {
	counter atomic.Uint64
}

func (rr *RoundRobin) Next(backends []*pool.Backend) *pool.Backend {
	if len(backends) == 0 {
		return nil
	}

	healthy := make([]*pool.Backend, 0, len(backends))
	for _, b := range backends {
		if b.IsHealthy() {
			healthy = append(healthy, b)
		}
	}
	if len(healthy) == 0 {
		return nil
	}

	// Atomic increment and modulo.
	// Add(1) returns the NEW value, so we subtract 1 to get a 0-based index.
	// This avoids the index-0 bias you'd get from Load() -> compute -> Store().
	idx := rr.counter.Add(1) - 1
	return healthy[idx%uint64(len(healthy))]
}

func (rr *RoundRobin) NextExcluding(
    backends []*pool.Backend,
    exclude map[string]struct{},
) *pool.Backend {
    if len(backends) == 0 {
        return nil
    }

    healthy := make([]*pool.Backend, 0, len(backends))
    for _, b := range backends {
        if b.IsHealthy() {
            if _, excluded := exclude[b.Addr]; !excluded {
                healthy = append(healthy, b)
            }
        }
    }
    if len(healthy) == 0 {
        return nil
    }

    idx := rr.counter.Add(1) - 1
    return healthy[idx%uint64(len(healthy))]
}
