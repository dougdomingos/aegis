package handler_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"dougdomingos.com/aegis/internal/orchestrator/api/handler"
)

func TestMetricsHandler_RegisterRoutes_WithGetRequest_ReturnsStatus200(t *testing.T) {
	router := setupMetricsRouter(t)

	rec := performRequest(t, router, http.MethodGet, "/metrics", nil)

	assertStatusCode(t, rec, http.StatusOK)
}

func TestMetricsHandler_RegisterRoutes_WithPostRequest_ReturnsStatus405(t *testing.T) {
	router := setupMetricsRouter(t)

	rec := performRequest(t, router, http.MethodPost, "/metrics", nil)

	assertStatusCode(t, rec, http.StatusMethodNotAllowed)
}

// ============================================================================
// Helpers
// ============================================================================

// setupMetricsRouter builds a router wired with a metrics handler that wraps a
// stub responding 200.
func setupMetricsRouter(t *testing.T) *chi.Mux {
	t.Helper()

	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r := chi.NewRouter()
	handler.NewMetricsHandler(stub).RegisterRoutes(r)

	return r
}
