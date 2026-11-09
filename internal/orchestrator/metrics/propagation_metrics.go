// Package metrics exposes the Orchestrator's raw measurements in the
// Prometheus format, leaving every aggregate (percentiles, means) to the
// Prometheus server.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// statusAcked is the only status whose latency is observed by the histogram.
const statusAcked = "acked"

// durationBuckets are the histogram upper bounds in seconds, spread around
// sub-second acknowledgements up to the propagation send timeout.
var durationBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

// PropagationMetrics records propagation outcomes and acknowledgement latency
// per group, and serves them through its own Prometheus registry.
type PropagationMetrics struct {
	registry *prometheus.Registry
	duration *prometheus.HistogramVec
	total    *prometheus.CounterVec
}

// NewPropagationMetrics creates a PropagationMetrics with its instruments
// registered into a private registry.
func NewPropagationMetrics() *PropagationMetrics {
	registry := prometheus.NewRegistry()

	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aegis_propagation_duration_seconds",
		Help:    "Time between sending a package propagation and its acknowledgement, by group.",
		Buckets: durationBuckets,
	}, []string{"group"})

	total := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aegis_propagations_total",
		Help: "Package propagation outcomes, by group and status.",
	}, []string{"group", "status"})

	registry.MustRegister(duration, total)

	return &PropagationMetrics{registry: registry, duration: duration, total: total}
}

// RecordPropagation counts one propagation outcome for the group. The latency
// is observed only for acknowledged propagations and ignored otherwise.
func (m *PropagationMetrics) RecordPropagation(group, status string, latency time.Duration) {
	m.total.WithLabelValues(group, status).Inc()

	if status == statusAcked {
		m.duration.WithLabelValues(group).Observe(latency.Seconds())
	}
}

// Handler returns the HTTP handler that serves the registry in the Prometheus
// text format.
func (m *PropagationMetrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
