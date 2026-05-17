package balancer

import "github.com/ngthdong/gobalancer/internal/pool"

type Balancer interface {
	Next(backends []*pool.Backend) *pool.Backend
	NextExcluding(backends []*pool.Backend, exclude map[string]struct{}) *pool.Backend
}
