// proxy/http.go
package proxy

import (
	"context"
	"log"
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
	pool     *pool.BackendPool
	balancer balancer.Balancer
	rp       *httputil.ReverseProxy
}

func NewHTTPProxy(p *pool.BackendPool, b balancer.Balancer, cfg *config.Config) *HTTPProxy {
	hp := &HTTPProxy{
		pool:     p,
		balancer: b,
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   cfg.Timeouts.Dial,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,

		IdleConnTimeout: cfg.Timeouts.Idle,

		TLSHandshakeTimeout: 10 * time.Second,

		ResponseHeaderTimeout: cfg.Timeouts.Read,

		ForceAttemptHTTP2: false,
	}

	hp.rp = &httputil.ReverseProxy{
		Director:     hp.director,
		Transport:    transport,
		ErrorHandler: hp.errorHandler,
	}

	return hp
}

// director is called by ReverseProxy for every request.
// It mutates the request to point at the selected backend.
func (hp *HTTPProxy) director(req *http.Request) {
	backend := hp.balancer.Next(hp.pool.Backends())
	if backend == nil {
		// No healthy backends. We can't return an error from Director
		// Instead, we set a sentinel URL that will fail to dial, which triggers ErrorHandler.
		req.URL.Host = "no-backend.invalid"
		req.URL.Scheme = "http"
		return
	}

	req.URL.Scheme = "http" 
	req.URL.Host = backend.Addr
	req.Host = backend.Addr

	// Add standard forwarding headers.
	// The X-Forwarded-For header is set to the client IP address.
	if req.TLS != nil {
		req.Header.Set("X-Forwarded-Proto", "https")
	} else {
		req.Header.Set("X-Forwarded-Proto", "http")
	}

	ctx := context.WithValue(req.Context(), constant.ContextKeyBackend, backend.Addr)
	*req = *req.WithContext(ctx)
}

func (hp *HTTPProxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("proxy error: backend=%s err=%v",
		r.Context().Value(constant.ContextKeyBackend), err)

	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

func (hp *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hp.rp.ServeHTTP(w, r)
}
