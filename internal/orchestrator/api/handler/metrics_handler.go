package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// MetricsHandler serves the Prometheus scrape route by delegating to the
// provided HTTP handler.
type MetricsHandler struct {
	exposition http.Handler
}

// NewMetricsHandler creates a new MetricsHandler instance around the handler
// that renders the metrics exposition.
func NewMetricsHandler(exposition http.Handler) *MetricsHandler {
	return &MetricsHandler{exposition: exposition}
}

// RegisterRoutes registers the metrics route into the provided router.
func (handler *MetricsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/metrics", handler.Scrape)
}

// Scrape renders the Prometheus metrics exposition.
//
//	@Summary Prometheus metrics
//	@Tags Metrics
//	@ID getMetrics
//	@Produce plain
//	@Success 200 {string} string "aegis_propagation_duration_seconds{group} histogram; aegis_propagations_total{group,status} counter"
//	@Router /metrics [get]
func (handler *MetricsHandler) Scrape(w http.ResponseWriter, r *http.Request) {
	handler.exposition.ServeHTTP(w, r)
}
