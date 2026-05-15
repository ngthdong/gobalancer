// balancer/leastconn.go
package balancer

import (
	"math"

	"github.com/ngthdong/gobalancer/internal/pool"
)

type LeastConnections struct{}

func (lc *LeastConnections) Next(backends []*pool.Backend) *pool.Backend {
	var best *pool.Backend
	bestConns := int64(math.MaxInt64)

	for _, b := range backends {
		if !b.IsHealthy() {
			continue
		}
		conns := b.ActiveConns()
		if conns < bestConns {
			bestConns = conns
			best = b
		}
	}

	return best
}
