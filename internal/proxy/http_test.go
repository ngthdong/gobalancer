package proxy_test

import (
	"io"
	"log/slog"
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

func TestHTTPProxy_ForwardsRequest(t *testing.T) {
	backend := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("hello from backend"))
		}),
	)
	defer backend.Close()

	cfg := &config.Config{
		Backends: []string{backend.Listener.Addr().String()},
		Timeouts: config.TimeoutConfig{
			Dial: time.Second,
			Read: time.Second,
			Idle: 30 * time.Second,
		},
		Retries: config.RetryConfig{
			MaxAttempts:  3,
			RetryOn5xx:   true,
			TotalTimeout: 5 * time.Second,
		},
	}
	p := pool.NewBackendPool(cfg)

	hp := proxy.NewHTTPProxy(
		p,
		&balancer.RoundRobin{},
		cfg,
		slog.Default(),
	)

	server := httptest.NewServer(hp)
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d want 200", resp.StatusCode)
	}

	if string(body) != "hello from backend" {
		t.Fatalf("got %q", body)
	}
}

func TestHTTPProxy_RetriesFailedBackend(t *testing.T) {
	healthy := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			w.Write([]byte("healthy backend"))
		}),
	)
	defer healthy.Close()

	cfg := &config.Config{
		Backends: []string{
			"127.0.0.1:19999",
			healthy.Listener.Addr().String(),
		},
		Timeouts: config.TimeoutConfig{
			Dial: 500 * time.Millisecond,
			Read: time.Second,
			Idle: 30 * time.Second,
		},
		Retries: config.RetryConfig{
			MaxAttempts:  3,
			RetryOn5xx:   true,
			TotalTimeout: 5 * time.Second,
		},
	}
	p := pool.NewBackendPool(cfg)

	hp := proxy.NewHTTPProxy(
		p,
		&balancer.RoundRobin{},
		cfg,
		slog.Default(),
	)

	server := httptest.NewServer(hp)
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(body) != "healthy backend" {
		t.Fatalf("got %q", body)
	}
}

func TestHTTPProxy_Returns502WhenAllBackendsFail(t *testing.T) {
	cfg := &config.Config{
		Backends: []string{
			"127.0.0.1:19001",
			"127.0.0.1:19002",
		},
		Timeouts: config.TimeoutConfig{
			Dial: 200 * time.Millisecond,
			Read: time.Second,
			Idle: 30 * time.Second,
		},
		Retries: config.RetryConfig{
			MaxAttempts:  2,
			RetryOn5xx:   true,
			TotalTimeout: time.Second,
		},
	}
	p := pool.NewBackendPool(cfg)

	hp := proxy.NewHTTPProxy(
		p,
		&balancer.RoundRobin{},
		cfg,
		slog.Default(),
	)

	server := httptest.NewServer(hp)
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("got %d want 502", resp.StatusCode)
	}
}

func TestHTTPProxy_AddsXForwardedFor(t *testing.T) {
	var forwardedFor string

	backend := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			forwardedFor = r.Header.Get("X-Forwarded-For")
			w.WriteHeader(http.StatusOK)
		}),
	)
	defer backend.Close()

	cfg := &config.Config{
		Backends: []string{backend.Listener.Addr().String()},
		Timeouts: config.TimeoutConfig{
			Dial: time.Second,
			Read: time.Second,
			Idle: 30 * time.Second,
		},
		Retries: config.RetryConfig{
			MaxAttempts:  2,
			RetryOn5xx:   true,
			TotalTimeout: 5 * time.Second,
		},
	}
	p := pool.NewBackendPool(cfg)

	hp := proxy.NewHTTPProxy(
		p,
		&balancer.RoundRobin{},
		cfg,
		slog.Default(),
	)

	server := httptest.NewServer(hp)
	defer server.Close()

	_, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if forwardedFor == "" {
		t.Fatal("expected X-Forwarded-For header")
	}
}

func TestHTTPProxy_DoesNotOverrideXForwardedFor(t *testing.T) {
	var forwardedFor string

	backend := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			forwardedFor = r.Header.Get("X-Forwarded-For")
			w.WriteHeader(http.StatusOK)
		}),
	)
	defer backend.Close()

	cfg := &config.Config{
		Backends: []string{backend.Listener.Addr().String()},
		Timeouts: config.TimeoutConfig{
			Dial: time.Second,
			Read: time.Second,
			Idle: 30 * time.Second,
		},
		Retries: config.RetryConfig{
			MaxAttempts:  2,
			RetryOn5xx:   true,
			TotalTimeout: 5 * time.Second,
		},
	}
	p := pool.NewBackendPool(cfg)
	hp := proxy.NewHTTPProxy(
		p,
		&balancer.RoundRobin{},
		cfg,
		slog.Default(),
	)

	server := httptest.NewServer(hp)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// httputil.ReverseProxy correctly appends the connecting client IP
	// to any existing X-Forwarded-For value — RFC 7239 compliant.
	// The original IP must be preserved at the start of the chain.
	if !strings.HasPrefix(forwardedFor, "1.2.3.4") {
		t.Fatalf("original IP lost from X-Forwarded-For chain: got %q", forwardedFor)
	}

	// The proxy must have appended its own hop — value should not be unchanged.
	if forwardedFor == "1.2.3.4" {
		t.Fatalf("proxy should have appended a hop, got only %q", forwardedFor)
	}
}
