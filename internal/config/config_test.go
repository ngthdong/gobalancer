package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/ngthdong/gobalancer/internal/config"
)

func TestLoadValidConfig(t *testing.T) {
	const yaml = `
listen_addr: ":8080"
mode: http
backends:
- "localhost:9001"
- "localhost:9002"
timeouts:
dial: 5s
read: 30s
`
	f, err := os.CreateTemp("", "config*.yaml")
	if err != nil {
		t.Fatalf("cannot create file")
	}
	f.WriteString(yaml)
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Backends) != 2 {
		t.Errorf("want 2 backends, got %d", len(cfg.Backends))
	}
	if cfg.Timeouts.Dial != 5*time.Second {
		t.Errorf("want 5s dial timeout, got %v", cfg.Timeouts.Dial)
	}
}
