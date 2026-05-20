package pool

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ngthdong/gobalancer/internal/config"
)

// BackendPool holds the list of upstream backends.
type BackendPool struct {
	mu       sync.RWMutex
	backends []*Backend
}

func NewBackendPool(cfg *config.Config) *BackendPool {
	addrs := cfg.Backends
	backends := make([]*Backend, len(cfg.Backends))
	for i, addr := range addrs {
		backends[i] = NewBackend(addr, cfg)
	}
	return &BackendPool{backends: backends}
}

func (p *BackendPool) Backends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]*Backend, len(p.backends))
	copy(result, p.backends)
	return result
}

func (p *BackendPool) Drain(ctx context.Context, addr string, logger *slog.Logger) error {
	p.mu.RLock()
	var target *Backend
	for _, b := range p.backends {
		if b.Addr == addr {
			target = b
			break
		}
	}
	p.mu.RUnlock()

	if target == nil {
		return fmt.Errorf("backend %s not found", addr)
	}

	if !target.state.CompareAndSwap(
		int32(StateActive),
		int32(StateDraining),
	) {
		return fmt.Errorf("backend %s is not in active state", addr)
	}

	logger.Info("draining backend", "addr", addr,
		"active_conns", target.ActiveConns())

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			remaining := target.ActiveConns()
			logger.Warn("drain deadline exceeded",
				"addr", addr,
				"remaining_conns", remaining)

			target.state.Store(int32(StateRemoved))
			return fmt.Errorf("drain timeout: %d connections remaining", remaining)

		case <-ticker.C:
			remaining := target.ActiveConns()
			logger.Debug("drain progress",
				"addr", addr,
				"remaining_conns", remaining)

			if remaining <= 0 {
				logger.Info("backend drained successfully", "addr", addr)
				target.state.Store(int32(StateRemoved))
				p.remove(addr)
				return nil
			}
		}
	}
}

func (p *BackendPool) remove(addr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	filtered := p.backends[:0]
	for _, b := range p.backends {
		if b.Addr != addr {
			filtered = append(filtered, b)
		}
	}
	p.backends = filtered
}

func (p *BackendPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.backends)
}

func (p *BackendPool) BackendAddrs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	addrs := make([]string, len(p.backends))
	for i, b := range p.backends {
		addrs[i] = b.Addr
	}
	return addrs
}

func (p *BackendPool) Add(newBackend *Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()
	currentBackendAddrs := p.BackendAddrs()
	for _, addr := range currentBackendAddrs {
		if addr == newBackend.Addr {
			return
		}
	}
	p.backends = append(p.backends, newBackend)
}
