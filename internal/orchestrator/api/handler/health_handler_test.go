package handler_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"dougdomingos.com/aegis/internal/orchestrator/api/handler"
)

func TestHealthHandler_Check_WithGetRequest_ReturnsStatus200(t *testing.T) {
	router := setupHealthRouter(t)

	rec := performRequest(t, router, http.MethodGet, "/health", nil)

	assertStatusCode(t, rec, http.StatusOK)
}

func TestHealthHandler_Check_WithPostRequest_ReturnsStatus405(t *testing.T) {
	router := setupHealthRouter(t)

	rec := performRequest(t, router, http.MethodPost, "/health", nil)

	assertStatusCode(t, rec, http.StatusMethodNotAllowed)
}

// ============================================================================
// Helpers
// ============================================================================

// setupHealthRouter builds a router wired with a health handler.
func setupHealthRouter(t *testing.T) *chi.Mux {
	t.Helper()

	r := chi.NewRouter()
	handler.NewHealthHandler().RegisterRoutes(r)

	return r
}
