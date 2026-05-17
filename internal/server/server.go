package server

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/middleware"
	"github.com/ngthdong/gobalancer/internal/pool"
	"github.com/ngthdong/gobalancer/internal/proxy"
)

type Server struct {
	cfg    *config.Config
	pool   *pool.BackendPool
	logger *slog.Logger
}

func NewServer(cfg *config.Config, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		pool:   pool.NewBackendPool(cfg.Backends),
		logger: logger,
	}
}

func (s *Server) Run() error {
	s.logger.Info("starting gobalancer",
		"mode", s.cfg.Mode,
		"listen", s.cfg.ListenAddr,
		"backends", s.cfg.Backends,
	)

	switch s.cfg.Mode {
	case "http":
		return s.runHTTP()
	case "tcp":
		return s.runTCP()
	default:
		return fmt.Errorf("unknown mode: %q", s.cfg.Mode)
	}
}

func (s *Server) runHTTP() error {
	b := s.newBalancer()
	hp := proxy.NewHTTPProxy(s.pool, b, s.cfg, s.logger)
	handler := middleware.Logging(hp, s.logger)

	srv := &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  s.cfg.Timeouts.Read,
		WriteTimeout: s.cfg.Timeouts.Write,
		IdleTimeout:  s.cfg.Timeouts.Idle,
	}

	s.logger.Info("HTTP proxy listening", "addr", s.cfg.ListenAddr)
	return srv.ListenAndServe()
}

func (s *Server) runTCP() error {
	b := s.newBalancer()
	tp := proxy.NewTCPProxy(s.pool, b, s.cfg, s.logger)

	s.logger.Info("TCP proxy listening", "addr", s.cfg.ListenAddr)
	return tp.ListenAndServe(s.cfg.ListenAddr)
}

func (s *Server) newBalancer() balancer.Balancer {
	switch s.cfg.Balancer {
	case "least_conn":
		return &balancer.LeastConnections{}
	default:
		return &balancer.RoundRobin{}
	}
}
