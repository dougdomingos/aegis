package handler

import (
	"net/http"

	"dougdomingos.com/aegis/internal/orchestrator/api/utils"
	"github.com/go-chi/chi/v5"
)

// HealthHandler serves the liveness probe. It holds no dependencies, so it
// answers whenever the process can serve HTTP, regardless of the state of the
// database or any other collaborator.
type HealthHandler struct{}

// NewHealthHandler creates a new HealthHandler instance.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// RegisterRoutes registers the health route into the provided router.
func (handler *HealthHandler) RegisterRoutes(r chi.Router) {
	r.Get("/health", handler.Check)
}

// Check replies 200 with a constant body to signal that the process is alive.
//
//	@Summary Liveness probe
//	@Tags Health
//	@ID getHealth
//	@Success 200 {object} map[string]string
//	@Router /health [get]
func (handler *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	utils.EncodeToJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
