package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/server"
)

func main() {
	cfgPath := flag.String("config", "local.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("failed to load config", "path", *cfgPath, "error", err)
		os.Exit(1)
	}

	logger := server.BuildLogger(cfg)

	if err := server.NewServer(cfg, logger).Run(); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}