package proxy_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/pool"
	"github.com/ngthdong/gobalancer/internal/proxy"
)

func TestHTTPProxyRoundTrip(t *testing.T) {
	// Start a real backend HTTP server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "backend received: %s", r.Host)
	}))
	defer backend.Close()

	// Build the proxy pointing at it
	p := pool.NewBackendPool([]string{strings.TrimPrefix(backend.URL, "http://")})
	rr := &balancer.RoundRobin{}
	cfg := &config.Config{Timeouts: config.TimeoutConfig{
		Dial: 5 * time.Second, Read: 10 * time.Second, Idle: 30 * time.Second,
	}}
	hp := proxy.NewHTTPProxy(p, rr, cfg)

	proxyServer := httptest.NewServer(hp)
	defer proxyServer.Close()

	resp, err := http.Get(proxyServer.URL + "/anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "backend received") {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestHostHeaderRewrite(t *testing.T) {
	var receivedHost string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	p := pool.NewBackendPool([]string{backendAddr})
	rr := &balancer.RoundRobin{}

	cfg := &config.Config{
		Timeouts: config.TimeoutConfig{
			Dial: 5 * time.Second,
			Read: 10 * time.Second,
			Idle: 30 * time.Second,
		},
	}

	hp := proxy.NewHTTPProxy(p, rr, cfg)

	proxyServer := httptest.NewServer(hp)
	defer proxyServer.Close()

	resp, err := http.Get(proxyServer.URL + "/hello")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if receivedHost != backendAddr {
		t.Errorf("backend received Host: %q, want %q",
			receivedHost,
			backendAddr,
		)
	}
}
