package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ngthdong/gobalancer/internal/metrics"
)

func Metrics(next http.Handler, m *metrics.Metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := newResponseRecorder(w)

		next.ServeHTTP(rec, r)

		backend := backendFromResponse(rec)
		status := strconv.Itoa(rec.statusCode)
		duration := time.Since(start).Seconds()

		m.RequestsTotal.WithLabelValues(backend, r.Method, status).Inc()
		m.RequestDuration.WithLabelValues(backend, r.Method).Observe(duration)
	})
}

// backendFromResponse extracts the backend address from response headers
// This is set by HTTPProxy after ReverseProxy returns
func backendFromResponse(rec *responseRecorder) string {
	return rec.Header().Get("X-Backend")
}
