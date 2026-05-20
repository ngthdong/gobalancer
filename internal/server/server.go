package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/conntrack"
	"github.com/ngthdong/gobalancer/internal/health"
	"github.com/ngthdong/gobalancer/internal/limiter"
	"github.com/ngthdong/gobalancer/internal/metrics"
	"github.com/ngthdong/gobalancer/internal/middleware"
	"github.com/ngthdong/gobalancer/internal/pool"
	"github.com/ngthdong/gobalancer/internal/proxy"
	"github.com/ngthdong/gobalancer/internal/session"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

type Server struct {
	cfg        *config.Config
	pool       *pool.BackendPool
	logger     *slog.Logger
	metrics    *metrics.Metrics
	reg        *prometheus.Registry
	httpServer *http.Server
	tcpWg      sync.WaitGroup
	cancelCtx  context.CancelFunc
}

func NewServer(cfg *config.Config, logger *slog.Logger) *Server {
	reg := prometheus.NewRegistry()

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return &Server{
		cfg:     cfg,
		pool:    pool.NewBackendPool(cfg),
		logger:  logger,
		metrics: metrics.NewMetrics(reg),
		reg:     reg,
	}
}

func (s *Server) Run(ctx context.Context) error {
	internalCtx, cancel := context.WithCancel(ctx)
	s.cancelCtx = cancel

	s.logger.Info("starting gobalancer",
		"mode", s.cfg.Mode,
		"listen", s.cfg.ListenAddr,
		"backends", s.cfg.Backends,
	)

	hm := health.NewManager(s.pool.Backends(), s.metrics, *s.cfg, s.logger)
	hm.Start(internalCtx)

	metricsServer := NewMetricsServer(s.cfg.MetricsAddr, s.reg, s.logger)
	metricsServer.Start(internalCtx)

	go s.watchReload(internalCtx)

	switch s.cfg.Mode {
	case "http":
		return s.runHTTP(internalCtx)
	case "tcp":
		return s.runTCP(internalCtx)
	default:
		return fmt.Errorf("unknown mode: %q", s.cfg.Mode)
	}
}

func (s *Server) runHTTP(ctx context.Context) error {
	b := s.newBalancer()
	hp := proxy.NewHTTPProxy(s.pool, b, s.cfg, s.logger)

	var handler http.Handler = hp

	if s.cfg.StickySession.Enabled {
		sticky := session.NewSticky(s.cfg.StickySession.CookieName)
		handler = middleware.StickySession(handler, sticky, s.pool)
	}

	if s.cfg.RateLimit.Enabled {
		rl := limiter.NewRateLimiter(
			s.cfg.RateLimit.RequestsPerSecond,
			s.cfg.RateLimit.Burst,
		)
		handler = limiter.RateLimit(rl, s.metrics)(handler)
	}

	handler = middleware.Metrics(handler, s.metrics)
	handler = middleware.Logging(handler, s.logger)

	s.httpServer = &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  s.cfg.Timeouts.Read,
		WriteTimeout: s.cfg.Timeouts.Write,
		IdleTimeout:  s.cfg.Timeouts.Idle,
	}

	s.logger.Info("HTTP proxy listening", "addr", s.cfg.ListenAddr)
	s.logger.Info(
		"sticky config",
		"enabled", s.cfg.StickySession.Enabled,
		"cookie", s.cfg.StickySession.CookieName,
	)

	if err := s.httpServer.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) runTCP(ctx context.Context) error {
	b := s.newBalancer()
	tracker := conntrack.NewTracker()
	tp := proxy.NewTCPProxy(s.pool, b, s.cfg, s.logger, tracker)

	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	s.logger.Info("TCP proxy listening", "addr", s.cfg.ListenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				s.logger.Info("TCP listener closed, stopping accept loop")
				break
			}
			s.logger.Warn("accept error", "error", err)
			continue
		}

		s.tcpWg.Add(1)
		go func() {
			defer s.tcpWg.Done()
			tp.HandleConn(conn)
		}()
	}

	return nil
}

func (s *Server) newBalancer() balancer.Balancer {
	switch s.cfg.Balancer {
	case "least_conn":
		return &balancer.LeastConnections{}
	default:
		return &balancer.RoundRobin{}
	}
}

// Shutdown initiates graceful shutdown and blocks until complete
// or the context deadline is exceeded.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("initiating graceful shutdown")

	var errs []error

	if s.httpServer != nil {
		s.logger.Info("draining HTTP connections...")
		if err := s.httpServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("http shutdown: %w", err))
		}
	}

	tcpDone := make(chan struct{})
	go func() {
		s.tcpWg.Wait()
		close(tcpDone)
	}()

	select {
	case <-tcpDone:
		s.logger.Info("all TCP connections drained")
	case <-ctx.Done():
		s.logger.Warn("shutdown timeout exceeded, some TCP connections may have been dropped")
		errs = append(errs, fmt.Errorf("tcp drain timeout: %w", ctx.Err()))
	}

	s.cancelCtx()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (s *Server) watchReload(ctx context.Context) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	defer signal.Stop(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			s.logger.Info("SIGHUP received, reloading config")
			if err := s.reload(); err != nil {
				s.logger.Error("config reload failed", "error", err)
			}
		}
	}
}

func (s *Server) reload() error {
	newCfg, err := config.Load(s.cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	currentBackendAddrs := s.pool.BackendAddrs()
	desiredBackendAddrs := newCfg.Backends

	added, removed := diffBackends(currentBackendAddrs, desiredBackendAddrs)

	// Add new backends first, they start receiving traffic immediately
	for _, addr := range added {
		s.pool.Add(pool.NewBackend(addr, newCfg))
		s.logger.Info("backend added", "addr", addr)
	}

	// Drain and remove old backends, no new connections, wait for active
	for _, addr := range removed {
		go func(addr string) {
			drainCtx, cancel := context.WithTimeout(
				context.Background(),
				30*time.Second,
			)
			defer cancel()

			if err := s.pool.Drain(drainCtx, addr, s.logger); err != nil {
				s.logger.Error("drain failed", "addr", addr, "error", err)
			}
		}(addr)
		s.logger.Info("backend draining", "addr", addr)
	}

	s.cfg = newCfg
	s.logger.Info("config reloaded",
		"added", len(added),
		"removed", len(removed),
	)
	return nil
}

func diffBackends(current, desired []string) (added, removed []string) {
	currentSet := make(map[string]struct{}, len(current))
	for _, addr := range current {
		currentSet[addr] = struct{}{}
	}

	desiredSet := make(map[string]struct{}, len(desired))
	for _, addr := range desired {
		desiredSet[addr] = struct{}{}
	}

	for addr := range desiredSet {
		if _, exists := currentSet[addr]; !exists {
			added = append(added, addr)
		}
	}

	for addr := range currentSet {
		if _, exists := desiredSet[addr]; !exists {
			removed = append(removed, addr)
		}
	}
	return
}
