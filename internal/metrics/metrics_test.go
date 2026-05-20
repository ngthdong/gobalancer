package metrics_test

import (
	"testing"

	"github.com/ngthdong/gobalancer/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRequestsCounterIncrements(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

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
	m := metrics.NewMetrics(reg)

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
	m := metrics.NewMetrics(reg)

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

func TestCircuitStateGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	// Test setting circuit state: 0=closed, 1=open, 2=half-open
	m.CircuitState.WithLabelValues("localhost:9001").Set(0) // closed
	m.CircuitState.WithLabelValues("localhost:9002").Set(1) // open
	m.CircuitState.WithLabelValues("localhost:9003").Set(2) // half-open

	v1 := testutil.ToFloat64(m.CircuitState.WithLabelValues("localhost:9001"))
	v2 := testutil.ToFloat64(m.CircuitState.WithLabelValues("localhost:9002"))
	v3 := testutil.ToFloat64(m.CircuitState.WithLabelValues("localhost:9003"))

	if v1 != 0 {
		t.Errorf("localhost:9001 want closed (0), got %v", v1)
	}
	if v2 != 1 {
		t.Errorf("localhost:9002 want open (1), got %v", v2)
	}
	if v3 != 2 {
		t.Errorf("localhost:9003 want half-open (2), got %v", v3)
	}
}
