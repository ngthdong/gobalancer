package proxy

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/ngthdong/gobalancer/internal/balancer"
	"github.com/ngthdong/gobalancer/internal/config"
	"github.com/ngthdong/gobalancer/internal/constant"
	"github.com/ngthdong/gobalancer/internal/pool"
)

type HTTPProxy struct {
	rp *httputil.ReverseProxy
}

type backendResponseWriter struct {
	http.ResponseWriter
	backend *string
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
	carrier, ok := r.Context().Value(constant.ContextKeyBackend).(*constant.BackendCarrier)
	if !ok {
		carrier = &constant.BackendCarrier{}
		ctx := context.WithValue(r.Context(), constant.ContextKeyBackend, carrier)
		r = r.WithContext(ctx)
	}

	brw := &backendResponseWriter{
		ResponseWriter: w,
		backend:        &carrier.Addr,
	}

	hp.rp.ServeHTTP(brw, r)

	if carrier.Addr != "" && w.Header().Get("X-Backend") == "" {
		w.Header().Set("X-Backend", carrier.Addr)
	}
}

func (brw *backendResponseWriter) Write(b []byte) (int, error) {
	if *brw.backend != "" && brw.ResponseWriter.Header().Get("X-Backend") == "" {
		brw.ResponseWriter.Header().Set("X-Backend", *brw.backend)
	}
	return brw.ResponseWriter.Write(b)
}
