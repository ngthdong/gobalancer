package health

import (
	"context"
	"sync"
	"log/slog"

	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/pool"
)

type Manager struct {
	checkers []*Checker
	logger   *slog.Logger
}

func NewManager(
	backends []*pool.Backend,
	config config.Config,
	healthConfig config.HealthConfig,
	logger *slog.Logger,
) *Manager {
	var strategy CheckStrategy
	if config.Mode == "http" {
		strategy = NewHTTPChecker(healthConfig.Timeout, healthConfig.Path)
	} else {
		strategy = &TCPChecker{}
	}

	checkers := make([]*Checker, len(backends))
	for i, b := range backends {
		checkers[i] = NewChecker(b, strategy, healthConfig, logger)
	}

	return &Manager{checkers: checkers, logger: logger}
}

func (m *Manager) Start(ctx context.Context) {
	var wg sync.WaitGroup
	for _, c := range m.checkers {
		wg.Add(1)
		c := c 
		go func() {
			defer wg.Done()
			c.Run(ctx)
		}()
	}

	go func() {
		wg.Wait()
		m.logger.Info("all health checkers stopped")
	}()
}
