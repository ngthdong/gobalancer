package proxy

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/pool"
)

type HTTPProxy struct {
	rp *httputil.ReverseProxy
}

func NewHTTPProxy(
	p *pool.BackendPool,
	b balancer.Balancer,
	cfg *config.Config,
	logger *slog.Logger,
) *HTTPProxy {
	innerTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   cfg.Timeouts.Dial,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       cfg.Timeouts.Idle,
		ResponseHeaderTimeout: cfg.Timeouts.Read,
		ForceAttemptHTTP2:     false,
	}

	retryingTransport := NewRetryingTransport(innerTransport, p, b, cfg, logger)

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Minimal director: RetryingTransport.RoundTrip owns backend
			// selection and rewrites req.URL.Host per attempt.
			// Only set the scheme so ReverseProxy doesn't reject the request,
			// and append X-Forwarded-For if the client hasn't set it already.
			req.URL.Scheme = "http"
			req.URL.Host = "placeholder"
			if req.Header.Get("X-Forwarded-For") == "" {
				req.Header.Set("X-Forwarded-For", req.RemoteAddr)
			}
		},
		Transport:    retryingTransport,
		ErrorHandler: errorHandler(logger),
	}

	return &HTTPProxy{rp: rp}
}

func (hp *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hp.rp.ServeHTTP(w, r)
}
