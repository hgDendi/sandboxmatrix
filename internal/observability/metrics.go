// Package observability provides Prometheus metrics and structured logging for sandboxMatrix.
package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for sandboxMatrix.
var Metrics = struct {
	SandboxesActive      prometheus.Gauge
	SandboxOpsTotal      *prometheus.CounterVec
	SandboxOpDuration    *prometheus.HistogramVec
	ExecDuration         *prometheus.HistogramVec
	ExecTotal            *prometheus.CounterVec
	SessionsActive       prometheus.Gauge
	MatricesActive       prometheus.Gauge
	PoolSize             *prometheus.GaugeVec
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	WebSocketConnections prometheus.Gauge
}{
	SandboxesActive: promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "smx",
		Name:      "sandboxes_active",
		Help:      "Number of active (running/ready) sandboxes.",
	}),
	SandboxOpsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "smx",
		Name:      "sandbox_operations_total",
		Help:      "Total number of sandbox operations.",
	}, []string{"operation", "result"}),
	SandboxOpDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "smx",
		Name:      "sandbox_operation_duration_seconds",
		Help:      "Duration of sandbox operations in seconds.",
		Buckets:   prometheus.ExponentialBuckets(0.01, 2, 12),
	}, []string{"operation"}),
	ExecDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "smx",
		Name:      "exec_duration_seconds",
		Help:      "Duration of exec commands in seconds.",
		Buckets:   prometheus.ExponentialBuckets(0.01, 2, 14),
	}, []string{"sandbox"}),
	ExecTotal: promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "smx",
		Name:      "exec_total",
		Help:      "Total number of exec commands.",
	}, []string{"sandbox", "result"}),
	SessionsActive: promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "smx",
		Name:      "sessions_active",
		Help:      "Number of active sessions.",
	}),
	MatricesActive: promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "smx",
		Name:      "matrices_active",
		Help:      "Number of active matrices.",
	}),
	PoolSize: promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "smx",
		Name:      "pool_size",
		Help:      "Number of pre-warmed sandbox instances per blueprint.",
	}, []string{"blueprint"}),
	HTTPRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "smx",
		Name:      "http_requests_total",
		Help:      "Total number of HTTP API requests.",
	}, []string{"method", "path", "status"}),
	HTTPRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "smx",
		Name:      "http_request_duration_seconds",
		Help:      "Duration of HTTP API requests in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "path"}),
	WebSocketConnections: promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "smx",
		Name:      "websocket_connections",
		Help:      "Number of active WebSocket connections.",
	}),
}
