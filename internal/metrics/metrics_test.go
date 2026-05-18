package metrics_test

import (
	"testing"

	"github.com/ngthdong/gobalancer/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRequestsCounterIncrements(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)

	m.RequestsTotal.WithLabelValues("localhost:9001", "GET", "200").Inc()
	m.RequestsTotal.WithLabelValues("localhost:9001", "GET", "200").Inc()
	m.RequestsTotal.WithLabelValues("localhost:9002", "GET", "500").Inc()

	v := testutil.ToFloat64(
		m.RequestsTotal.WithLabelValues("localhost:9001", "GET", "200"),
	)
	if v != 2 {
		t.Errorf("want 2, got %v", v)
	}
}

func TestActiveConnectionsGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)

	m.ActiveConnections.WithLabelValues("localhost:9001").Add(3)
	m.ActiveConnections.WithLabelValues("localhost:9001").Sub(1)

	v := testutil.ToFloat64(m.ActiveConnections.WithLabelValues("localhost:9001"))
	if v != 2 {
		t.Errorf("want 2, got %v", v)
	}
}

func TestMetricsEndpointReachable(t *testing.T) {
	// Use testutil.GatherAndCompare for full metric snapshot testing
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)

	m.RequestsTotal.WithLabelValues("b1", "GET", "200").Inc()

	// Gather and verify output contains expected metric name
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "gobalancer_requests_total" {
			found = true
		}
	}
	if !found {
		t.Error("gobalancer_requests_total not found in gathered metrics")
	}
}
