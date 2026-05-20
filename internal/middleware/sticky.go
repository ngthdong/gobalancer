package middleware

import (
	"context"
	"net/http"

	"github.com/ngthdong/gobalancer/internal/constant"
	"github.com/ngthdong/gobalancer/internal/pool"
	"github.com/ngthdong/gobalancer/internal/session"
)

// stickyResponseWriter wraps http.ResponseWriter to intercept headers
// and add Set-Cookie before they're sent to the client.
type stickyResponseWriter struct {
	http.ResponseWriter
	headerWritten     bool
	carrier           *constant.BackendCarrier
	sticky            *session.Sticky
	pool              *pool.BackendPool
	stickyBackendAddr string
	cookieSet         bool
}

func (srw *stickyResponseWriter) WriteHeader(statusCode int) {
	// Set cookie if:
	// 1. No sticky cookie existed in the incoming request
	// 2. Or the backend that was actually selected differs
	// 	 from the sticky backend (degradation)
	if !srw.cookieSet && srw.carrier.Addr != "" {
		shouldSetCookie := false

		if (srw.stickyBackendAddr == "") ||
			(srw.stickyBackendAddr != srw.carrier.Addr) {
			shouldSetCookie = true
		}

		if shouldSetCookie {
			for _, b := range srw.pool.Backends() {
				if b.Addr == srw.carrier.Addr {
					srw.sticky.SetCookie(srw.ResponseWriter, b)
					srw.cookieSet = true
					break
				}
			}
		}
	}
	srw.headerWritten = true
	srw.ResponseWriter.WriteHeader(statusCode)
}

func (srw *stickyResponseWriter) Write(b []byte) (int, error) {
	if !srw.headerWritten {
		srw.WriteHeader(http.StatusOK)
	}
	return srw.ResponseWriter.Write(b)
}

// StickySession wraps a handler and enforces session affinity.
// It reads the sticky cookie before the request and sets it after.
func StickySession(
	next http.Handler,
	sticky *session.Sticky,
	p *pool.BackendPool,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		carrier := &constant.BackendCarrier{}

		ctx := context.WithValue(
			r.Context(),
			constant.ContextKeyBackend,
			carrier)

		r = r.WithContext(ctx)

		var stickyBackendAddr string
		if cookie, err := r.Cookie(sticky.CookieName()); err == nil {
			stickyBackendAddr = cookie.Value
			backends := p.Backends()
			for _, b := range backends {
				if b.Addr == cookie.Value && b.IsAvailable() {
					ctx := context.WithValue(
						r.Context(),
						constant.ContextKeyStickyBackend,
						cookie.Value)

					r = r.WithContext(ctx)
					break
				}
			}
		}

		srw := &stickyResponseWriter{
			ResponseWriter:    w,
			carrier:           carrier,
			sticky:            sticky,
			pool:              p,
			stickyBackendAddr: stickyBackendAddr,
		}

		next.ServeHTTP(srw, r)
	})
}
