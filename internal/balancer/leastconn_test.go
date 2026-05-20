package balancer_test

import (
	"testing"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/pool"
)

func TestLeastConnPicksLowest(t *testing.T) {
	backends := []*pool.Backend{
		pool.NewBackend("a:1", nil),
		pool.NewBackend("b:2", nil),
		pool.NewBackend("c:3", nil),
	}
	backends[0].TrackConn(10)
	backends[1].TrackConn(3) // lowest
	backends[2].TrackConn(7)

	lc := &balancer.LeastConnections{}
	picked := lc.Next(backends)

	if picked.Addr != "b:2" {
		t.Errorf("expected b:2 (3 conns), got %s", picked.Addr)
	}
}

func TestLeastConnSkipsUnhealthy(t *testing.T) {
	healthy := pool.NewBackend("healthy:1", nil)
	unhealthy := pool.NewBackend("unhealthy:2", nil)
	unhealthy.SetHealthy(false)

	unhealthy.TrackConn(0)
	healthy.TrackConn(100)

	lc := &balancer.LeastConnections{}
	picked := lc.Next([]*pool.Backend{unhealthy, healthy})

	if picked.Addr != "healthy:1" {
		t.Errorf("expected healthy backend, got %s", picked.Addr)
	}
}
