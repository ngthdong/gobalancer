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

// Uses time.NewTicker (not time.Sleep) so cancellation is immediate
// Runs the first check immediately, not after the first interval
// Logs state transitions, not every check result
func (c *Checker) Run(ctx context.Context) {
	// Check immediately on startup rather than waiting for the first tick.
	// Without this, backends start healthy (optimistic default) and you
	// won't discover dead backends until the first interval elapses.
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
	isHealthy := err == nil

	if !isHealthy {
		failures := c.backend.ConsecutiveFailures() + 1
		if failures < int32(c.cfg.FailureThreshold) {
			c.backend.SetHealthy(false)
			c.logger.Debug("health check failed, not yet at threshold",
				"failures", failures,
				"threshold", c.cfg.FailureThreshold,
				"error", err)
			return
		}
	}

	c.backend.SetHealthy(isHealthy)

	// Log only on state transitions, not on every check.
	// Logging every successful check at INFO would flood your log pipeline
	// at 1 check/10s × 100 backends = 10 log lines/second of noise.
	if wasHealthy != c.backend.IsHealthy() {
		if isHealthy {
			c.logger.Info("backend recovered",
				"consecutive_failures_before", c.backend.ConsecutiveFailures())
		} else {
			c.logger.Warn("backend unhealthy",
				"error", err,
				"consecutive_failures", c.backend.ConsecutiveFailures())
		}
	} else {
		c.logger.Debug("health check ok")
	}
}
