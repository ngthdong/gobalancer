package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/middleware"
	"github.com/ngthdong/gobalancer/internal/pool"
	"github.com/ngthdong/gobalancer/internal/proxy"
)

func main() {
	cfgPath := flag.String("config", "local.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	log.Printf("starting gobalancer mode=%s listen=%s backends=%v",
		cfg.Mode, cfg.ListenAddr, cfg.Backends)

	p := pool.NewBackendPool(cfg.Backends)
	rr := &balancer.RoundRobin{}

	switch cfg.Mode {
	case "http":
		runHTTP(cfg, p, rr)
	case "tcp":
		runTCP(cfg, p, rr)
	default:
		log.Fatalf("unknown mode: %s", cfg.Mode)
	}
}

func runHTTP(cfg *config.Config, p *pool.BackendPool, b balancer.Balancer) {
	hp := proxy.NewHTTPProxy(p, b, cfg)
	handler := middleware.Logging(hp)

	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler,
		ReadTimeout:  cfg.Timeouts.Read,
		WriteTimeout: cfg.Timeouts.Write,
		IdleTimeout:  cfg.Timeouts.Idle,
	}

	log.Printf("HTTP proxy listening on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func runTCP(cfg *config.Config, p *pool.BackendPool, b balancer.Balancer) {
	tp := proxy.NewTCPProxy(p, b, cfg)
	tp.ListenAndServe(cfg.ListenAddr)
}
