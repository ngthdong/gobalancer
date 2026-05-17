package proxy_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/pool"
	"github.com/ngthdong/gobalancer/internal/proxy"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(
	r *http.Request,
) (*http.Response, error) {
	return f(r)
}

func TestRetryingTransport_RetriesTransportError(t *testing.T) {
	attempts := 0

	inner := roundTripFunc(func(
		r *http.Request,
	) (*http.Response, error) {
		attempts++

		if attempts == 1 {
			return nil, errors.New("connection refused")
		}

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	p := pool.NewBackendPool([]string{
		"backend-1",
		"backend-2",
	})

	rt := proxy.NewRetryingTransport(
		inner,
		p,
		&balancer.RoundRobin{},
		&config.Config{
			Retries: config.RetryConfig{
				MaxAttempts:  2,
				RetryOn5xx:   true,
				TotalTimeout: 5 * time.Second,
			},
		},
		slog.Default(),
	)

	req, _ := http.NewRequest(
		http.MethodGet,
		"http://example.com",
		nil,
	)

	resp, err := rt.RoundTrip(req)

	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("got %d want 200", resp.StatusCode)
	}

	if attempts != 2 {
		t.Fatalf("got %d attempts want 2", attempts)
	}
}

func TestRetryingTransport_DoesNotRetryPost(t *testing.T) {
	attempts := 0

	inner := roundTripFunc(func(
		r *http.Request,
	) (*http.Response, error) {
		attempts++
		return nil, errors.New("boom")
	})

	p := pool.NewBackendPool([]string{
		"backend-1",
		"backend-2",
	})

	rt := proxy.NewRetryingTransport(
		inner,
		p,
		&balancer.RoundRobin{},
		&config.Config{
			Retries: config.RetryConfig{
				MaxAttempts:  5,
				TotalTimeout: 5 * time.Second,
			},
		},
		slog.Default(),
	)

	req, _ := http.NewRequest(
		http.MethodPost,
		"http://example.com",
		nil,
	)

	_, err := rt.RoundTrip(req)

	if err == nil {
		t.Fatal("expected error")
	}

	if attempts != 1 {
		t.Fatalf("got %d attempts want 1", attempts)
	}
}

func TestRetryingTransport_RetriesOn5xx(t *testing.T) {
	attempts := 0

	inner := roundTripFunc(func(
		r *http.Request,
	) (*http.Response, error) {
		attempts++

		if attempts == 1 {
			return &http.Response{
				StatusCode: 503,
				Body:       io.NopCloser(strings.NewReader("bad")),
			}, nil
		}

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	p := pool.NewBackendPool([]string{
		"backend-1",
		"backend-2",
	})

	rt := proxy.NewRetryingTransport(
		inner,
		p,
		&balancer.RoundRobin{},
		&config.Config{
			Retries: config.RetryConfig{
				MaxAttempts:  2,
				RetryOn5xx:   true,
				TotalTimeout: 5 * time.Second,
			},
		},
		slog.Default(),
	)

	req, _ := http.NewRequest(
		http.MethodGet,
		"http://example.com",
		nil,
	)

	resp, err := rt.RoundTrip(req)

	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("got %d want 200", resp.StatusCode)
	}

	if attempts != 2 {
		t.Fatalf("got %d attempts want 2", attempts)
	}
}

func TestRetryingTransport_ExhaustsAllBackends(t *testing.T) {
	attempts := 0

	inner := roundTripFunc(func(
		r *http.Request,
	) (*http.Response, error) {
		attempts++
		return nil, errors.New("boom")
	})

	p := pool.NewBackendPool([]string{
		"backend-1",
		"backend-2",
		"backend-3",
	})

	rt := proxy.NewRetryingTransport(
		inner,
		p,
		&balancer.RoundRobin{},
		&config.Config{
			Retries: config.RetryConfig{
				MaxAttempts:  3,
				TotalTimeout: 5 * time.Second,
			},
		},
		slog.Default(),
	)

	req, _ := http.NewRequest(
		http.MethodGet,
		"http://example.com",
		nil,
	)

	resp, err := rt.RoundTrip(req)

	if err == nil {
		t.Fatal("expected error")
	}

	if resp != nil {
		t.Fatal("expected nil response")
	}

	if attempts != 3 {
		t.Fatalf("got %d attempts want 3", attempts)
	}
}

func TestRetryingTransport_UsesDifferentBackendAfterFailure(t *testing.T) {
	var hosts []string

	inner := roundTripFunc(func(
		r *http.Request,
	) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)

		if len(hosts) == 1 {
			return nil, errors.New("boom")
		}

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	p := pool.NewBackendPool([]string{
		"backend-1",
		"backend-2",
	})

	rt := proxy.NewRetryingTransport(
		inner,
		p,
		&balancer.RoundRobin{},
		&config.Config{
			Retries: config.RetryConfig{
				MaxAttempts:  2,
				TotalTimeout: 5 * time.Second,
			},
		},
		slog.Default(),
	)

	req, _ := http.NewRequest(
		http.MethodGet,
		"http://example.com",
		nil,
	)

	_, err := rt.RoundTrip(req)

	if err != nil {
		t.Fatal(err)
	}

	if len(hosts) != 2 {
		t.Fatalf("got %d hosts want 2", len(hosts))
	}

	if hosts[0] == hosts[1] {
		t.Fatal("expected retry to use different backend")
	}
}
