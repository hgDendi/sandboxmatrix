package observability

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRegistered(t *testing.T) {
	// Verify all metrics are registered and can be collected.
	ch := make(chan prometheus.Metric, 100)

	// Gauges
	Metrics.SandboxesActive.Collect(ch)
	Metrics.SessionsActive.Collect(ch)
	Metrics.MatricesActive.Collect(ch)
	Metrics.WebSocketConnections.Collect(ch)

	// CounterVecs
	Metrics.SandboxOpsTotal.WithLabelValues("create", "success").Inc()
	Metrics.SandboxOpsTotal.Collect(ch)

	Metrics.ExecTotal.WithLabelValues("test-sb", "success").Inc()
	Metrics.ExecTotal.Collect(ch)

	Metrics.HTTPRequestsTotal.WithLabelValues("GET", "/health", "200").Inc()
	Metrics.HTTPRequestsTotal.Collect(ch)

	// HistogramVecs
	Metrics.SandboxOpDuration.WithLabelValues("create").Observe(0.5)
	Metrics.SandboxOpDuration.Collect(ch)

	Metrics.ExecDuration.WithLabelValues("test-sb").Observe(1.0)
	Metrics.ExecDuration.Collect(ch)

	Metrics.HTTPRequestDuration.WithLabelValues("GET", "/health").Observe(0.01)
	Metrics.HTTPRequestDuration.Collect(ch)

	// GaugeVecs
	Metrics.PoolSize.WithLabelValues("python-dev").Set(5)
	Metrics.PoolSize.Collect(ch)

	close(ch)

	count := 0
	for range ch {
		count++
	}
	if count == 0 {
		t.Fatal("expected metrics to be collected")
	}
}

func TestMetricsIncDec(t *testing.T) {
	// Verify gauge inc/dec works without panics.
	Metrics.SandboxesActive.Inc()
	Metrics.SandboxesActive.Inc()
	Metrics.SandboxesActive.Dec()

	ch := make(chan prometheus.Metric, 1)
	Metrics.SandboxesActive.Collect(ch)
	<-ch // just verify we get a metric back
}
