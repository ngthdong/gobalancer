package balancer

import (
	"sync"
	"testing"

	"github.com/ngthdong/gobalancer/internal/pool"
)

// balancer/roundrobin_test.go
func TestRoundRobinDistribution(t *testing.T) {
	backends := []*pool.Backend{
		pool.NewBackend("a:1"),
		pool.NewBackend("b:2"),
		pool.NewBackend("c:3"),
	}
	rr := &RoundRobin{}

	counts := map[string]int{}
	for i := 0; i < 9; i++ {
		b := rr.Next(backends)
		counts[b.Addr]++
	}

	for _, b := range backends {
		if counts[b.Addr] != 3 {
			t.Errorf("backend %s got %d requests, want 3", b.Addr, counts[b.Addr])
		}
	}
}

func TestRoundRobinConcurrent(t *testing.T) {
	backends := []*pool.Backend{
		pool.NewBackend("a:1"),
		pool.NewBackend("b:2"),
	}
	rr := &RoundRobin{}

	// 1000 concurrent goroutines run with go test -race
	var wg sync.WaitGroup
	results := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b := rr.Next(backends)
			results[idx] = b.Addr
		}(i)
	}
	wg.Wait()

	// Each backend should get ~500 requests (within 10%)
	counts := map[string]int{}
	for _, addr := range results {
		counts[addr]++
	}
	for addr, count := range counts {
		if count < 450 || count > 550 {
			t.Errorf("uneven distribution: %s got %d/1000", addr, count)
		}
	}
}

func TestRoundRobinEmptyPool(t *testing.T) {
	rr := &RoundRobin{}
	if b := rr.Next(nil); b != nil {
		t.Errorf("expected nil for empty pool, got %v", b)
	}
}
