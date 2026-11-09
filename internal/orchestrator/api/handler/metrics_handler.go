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
	r.Method(http.MethodGet, "/metrics", handler.exposition)
}
