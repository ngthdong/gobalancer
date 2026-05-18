package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/constant"
	"github.com/ngthdong/gobalancer/internal/pool"
)

type RetryingTransport struct {
	inner    http.RoundTripper
	pool     *pool.BackendPool
	balancer balancer.Balancer
	cfg      *config.Config
	logger   *slog.Logger
}

func NewRetryingTransport(
	inner http.RoundTripper,
	p *pool.BackendPool,
	b balancer.Balancer,
	cfg *config.Config,
	logger *slog.Logger,
) *RetryingTransport {
	return &RetryingTransport{
		inner:    inner,
		pool:     p,
		balancer: b,
		cfg:      cfg,
		logger:   logger,
	}
}

func (rt *RetryingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	maxAttempts := rt.cfg.Retries.MaxAttempts
	if !isIdempotent(req.Method) {
		maxAttempts = 1
	}

	excluded := make(map[string]struct{})
	backends := rt.pool.Backends()

	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var backend *pool.Backend
		if attempt == 1 {
			backend = rt.balancer.Next(backends)
		} else {
			backend = rt.balancer.NextExcluding(backends, excluded)
		}

		if backend == nil {
			return nil, fmt.Errorf("no available backends (excluded: %d)", len(excluded))
		}

		reqCopy := req.Clone(req.Context())
		reqCopy.URL.Host = backend.Addr
		reqCopy.URL.Scheme = "http"
		reqCopy.Host = backend.Addr

		if carrier, ok := req.Context().Value(constant.ContextKeyBackend).(*constant.BackendCarrier); ok {
			carrier.Addr = backend.Addr
		}

		backend.TrackConn(+1)
		resp, err := rt.inner.RoundTrip(reqCopy)
		backend.TrackConn(-1)

		if err == nil {
			if rt.cfg.Retries.RetryOn5xx && isIdempotent(req.Method) &&
				resp.StatusCode >= 500 {

				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				rt.recordFailure(backend)

				excluded[backend.Addr] = struct{}{}
				lastErr = fmt.Errorf("backend returned %d", resp.StatusCode)

				rt.logger.Warn("retrying on 5xx",
					"backend", backend.Addr,
					"status", resp.StatusCode,
					"attempt", attempt,
				)

				if attempt < maxAttempts {
					rt.backoff(req.Context(), attempt)
				}
				continue
			}

			backend.SetHealthy(true)

			if attempt > 1 {
				rt.logger.Info("retry succeeded",
					"backend", backend.Addr,
					"attempt", attempt,
				)
			}
			return resp, nil
		}

		rt.recordFailure(backend)

		excluded[backend.Addr] = struct{}{}
		lastErr = err

		rt.logger.Warn("transport error, retrying",
			"backend", backend.Addr,
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"error", err,
		)

		if attempt < maxAttempts {
			rt.backoff(req.Context(), attempt)
		}
	}

	return nil, fmt.Errorf("all %d attempts failed, last error: %w",
		maxAttempts, lastErr)
}

func (rt *RetryingTransport) recordFailure(backend *pool.Backend) {
	newFailures := backend.IncrementFailures()
	if newFailures >= int32(rt.cfg.Health.FailureThreshold) {
		wasHealthy := backend.IsHealthy()
		backend.SetHealthy(false)
		if wasHealthy {
			rt.logger.Warn("backend marked unhealthy by passive check",
				"backend", backend.Addr,
				"consecutive_failures", newFailures,
			)
		}
	}
}
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead,
		http.MethodOptions, http.MethodTrace,
		http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

func (rt *RetryingTransport) backoff(ctx context.Context, attempt int) {
	base := time.Duration(10*(1<<uint(attempt-1))) * time.Millisecond
	if base > 500*time.Millisecond {
		base = 500 * time.Millisecond
	}
	jitter := time.Duration(rand.Int63n(int64(base / 2)))
	delay := base + jitter

	select {
	case <-time.After(delay):
	case <-ctx.Done():
	}
}
