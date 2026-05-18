package health

import (
	"context"
	"log/slog"
	"time"

	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/pool"
)

type Checker struct {
	backend  *pool.Backend
	strategy CheckStrategy
	cfg      config.HealthConfig
	logger   *slog.Logger
}

func NewChecker(
	b *pool.Backend,
	strategy CheckStrategy,
	cfg config.HealthConfig,
	logger *slog.Logger,
) *Checker {
	return &Checker{
		backend:  b,
		strategy: strategy,
		cfg:      cfg,
		logger:   logger.With("backend", b.Addr),
	}
}

func (c *Checker) Run(ctx context.Context) {
	c.runCheck(ctx)

	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("health checker stopping")
			return
		case <-ticker.C:
			c.runCheck(ctx)
		}
	}
}

func (c *Checker) runCheck(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	err := c.strategy.Check(checkCtx, c.backend.Addr)

	wasHealthy := c.backend.IsHealthy()

	if err != nil {
		newFailures := c.backend.IncrementFailures()

		if newFailures < int32(c.cfg.FailureThreshold) {
			c.logger.Debug("health check failed, not yet at threshold",
				"failures", newFailures,
				"threshold", c.cfg.FailureThreshold,
				"error", err,
			)
			return
		}

		if wasHealthy {
			c.backend.SetHealthy(false)

			c.logger.Warn("backend unhealthy",
				"error", err,
				"consecutive_failures", newFailures,
			)
		}
		return
	}

	if !wasHealthy {
		c.backend.SetHealthy(true)
		
		c.logger.Info("backend recovered")
		return
	}
	
	c.logger.Debug("health check ok")
}