// pool/pool.go
package pool

import "sync"

// BackendPool holds the list of upstream backends.
type BackendPool struct {
	mu       sync.RWMutex
	backends []*Backend
}

func NewBackendPool(addrs []string) *BackendPool {
	backends := make([]*Backend, len(addrs))
	for i, addr := range addrs {
		backends[i] = NewBackend(addr)
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

func (p *BackendPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.backends)
}
