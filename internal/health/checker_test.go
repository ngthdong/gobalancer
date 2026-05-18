package health_test

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/health"
	"github.com/ngthdong/gobalancer/internal/metrics"
	"github.com/ngthdong/gobalancer/internal/pool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMetrics() *metrics.Metrics {
	reg := prometheus.NewRegistry()
	return metrics.New(reg)
}

func TestHealthCheckerMarksUnhealthy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	backend := pool.NewBackend(ln.Addr().String())

	strategy := &health.TCPChecker{}

	cfg := config.HealthConfig{
		Interval:         50 * time.Millisecond,
		Timeout:          100 * time.Millisecond,
		FailureThreshold: 1,
	}

	m := newTestMetrics()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checker := health.NewChecker(
		backend,
		m,
		strategy,
		cfg,
		slog.Default(),
	)

	go checker.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	assert.True(t, backend.IsHealthy())

	ln.Close()

	assert.Eventually(t, func() bool {
		return !backend.IsHealthy()
	}, 500*time.Millisecond, 10*time.Millisecond)
}

func TestHealthCheckerRecovers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()

	ln.Close()

	backend := pool.NewBackend(addr)

	strategy := &health.TCPChecker{}

	cfg := config.HealthConfig{
		Interval:         50 * time.Millisecond,
		Timeout:          100 * time.Millisecond,
		FailureThreshold: 1,
	}

	m := newTestMetrics()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checker := health.NewChecker(
		backend,
		m,
		strategy,
		cfg,
		slog.Default(),
	)

	go checker.Run(ctx)

	// wait until unhealthy
	assert.Eventually(t, func() bool {
		return !backend.IsHealthy()
	}, 500*time.Millisecond, 10*time.Millisecond)

	// restart backend
	ln, err = net.Listen("tcp", addr)
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	// wait until healthy again
	assert.Eventually(t, func() bool {
		return backend.IsHealthy()
	}, 500*time.Millisecond, 10*time.Millisecond)
}
