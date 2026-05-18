package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all instrumentation for the proxy.
// One instance, created at startup, injected wherever needed.
type Metrics struct {
	// RequestsTotal counts completed requests, labelled by backend,
	// method, and HTTP status code.
	// rate(gobalancer_requests_total[1m]) -> current RPS
	RequestsTotal *prometheus.CounterVec

	// RequestDuration records end-to-end latency from first byte received
	// from client to last byte sent to client, in seconds.
	// histogram_quantile(0.99, rate(gobalancer_request_duration_seconds_bucket[5m]))
	RequestDuration *prometheus.HistogramVec

	// ActiveConnections tracks in-flight connections per backend.
	// This is a Gauge — it goes up on connect and down on disconnect.
	ActiveConnections *prometheus.GaugeVec

	// BackendHealthy tracks the current health state per backend.
	// 1 = healthy, 0 = unhealthy. Useful for alerting:
	// alert when gobalancer_backend_healthy == 0
	BackendHealthy *prometheus.GaugeVec

	// RetryTotal counts retry attempts, labelled by backend and reason.
	// Spikes here indicate backend instability.
	RetryTotal *prometheus.CounterVec

	// ErrorsTotal counts proxy-level errors (not backend 4xx/5xx),
	// labelled by kind (upstream_timeout, no_backends, upstream_error).
	ErrorsTotal *prometheus.CounterVec
}

// New creates and registers all metrics with the provided registry.
// Using a custom registry (not prometheus.DefaultRegisterer) means
// tests can create isolated registries without "already registered" panics.
func New(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		RequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gobalancer_requests_total",
				Help: "Total number of requests proxied, by backend, method, and status.",
			},
			[]string{"backend", "method", "status"},
		),

		RequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "gobalancer_request_duration_seconds",
				Help: "End-to-end request duration in seconds.",
				// Buckets tuned for a proxy: most requests under 100ms,
				// but we want visibility into the slow tail up to 10s.
				Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"backend", "method"},
		),

		ActiveConnections: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gobalancer_active_connections",
				Help: "Number of currently active proxy connections per backend.",
			},
			[]string{"backend"},
		),

		BackendHealthy: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gobalancer_backend_healthy",
				Help: "Current health state of each backend (1=healthy, 0=unhealthy).",
			},
			[]string{"backend"},
		),

		RetryTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gobalancer_retries_total",
				Help: "Total number of retry attempts, by backend and reason.",
			},
			[]string{"backend", "reason"},
		),

		ErrorsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gobalancer_errors_total",
				Help: "Total number of proxy errors, by kind.",
			},
			[]string{"kind"},
		),
	}
}
