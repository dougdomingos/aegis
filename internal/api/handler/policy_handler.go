package handler

import (
	"context"
	"net/http"
	"strconv"

	"dougdomingos.com/aegis/internal/api/utils"
	"dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
	"github.com/go-chi/chi/v5"
)

// PolicyServiceInterface declares the methods provided by PolicyService
// implementations.
type PolicyServiceInterface interface {
	CreatePolicy(ctx context.Context, p schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error)
	GetPolicyByID(ctx context.Context, p schemas.GetPolicyByIDSchema) (*schemas.PolicyOutputSchema, error)
	ListPolicies(ctx context.Context) ([]schemas.PolicyOutputSchema, error)
	UpdatePolicy(ctx context.Context, p schemas.UpdatePolicySchema) (*schemas.PolicyOutputSchema, error)
	RemovePolicy(ctx context.Context, p schemas.RemovePolicySchema) error
}

// PolicyHandler implements the methods that map HTTP requests into operations
// within the application.
type PolicyHandler struct {
	service PolicyServiceInterface
}

// NewPolicyHandler creates a new PolicyHandler instance.
func NewPolicyHandler(s PolicyServiceInterface) *PolicyHandler {
	return &PolicyHandler{service: s}
}

// RegisterRoutes registers the policy management routes into the provided
// router.
func (handler *PolicyHandler) RegisterRoutes(r chi.Router) {
	r.Route("/policies", func(r chi.Router) {
		r.Post("/", handler.Create)
		r.Get("/", handler.List)
		r.Get("/{id}", handler.GetByID)
		r.Patch("/{id}", handler.Update)
		r.Delete("/{id}", handler.Delete)
	})
}

// Create registers a new policy within the system.
func (handler *PolicyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var payload schemas.CreatePolicySchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.CreatePolicy(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrPolicyNameRequired:      http.StatusBadRequest,
			errors.ErrPolicyNameAlreadyExists: http.StatusConflict,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusCreated, res)
}

// List returns every policy registered within the system.
func (handler *PolicyHandler) List(w http.ResponseWriter, r *http.Request) {
	policies, err := handler.service.ListPolicies(r.Context())
	if err != nil {
		utils.MapByError(w, err, nil)
		return
	}

	if policies == nil {
		policies = []schemas.PolicyOutputSchema{}
	}

	utils.EncodeToJSON(w, http.StatusOK, policies)
}

// GetByID retrieves a policy by its ID.
func (handler *PolicyHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePolicyID(w, r, "id")
	if !ok {
		return
	}

	res, err := handler.service.GetPolicyByID(r.Context(), schemas.GetPolicyByIDSchema{ID: id})
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrPolicyNotFound: http.StatusNotFound,
		})
		return
	}

	if res == nil {
		utils.EmitError(w, errors.ErrPolicyNotFound.Error(), http.StatusNotFound)
		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// Update applies the provided name change to an existent policy.
func (handler *PolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePolicyID(w, r, "id")
	if !ok {
		return
	}

	var payload schemas.UpdatePolicySchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	payload.ID = id

	res, err := handler.service.UpdatePolicy(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrPolicyNotFound:          http.StatusNotFound,
			errors.ErrPolicyNameRequired:      http.StatusBadRequest,
			errors.ErrPolicyNameAlreadyExists: http.StatusConflict,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// Delete removes a policy by its ID.
func (handler *PolicyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePolicyID(w, r, "id")
	if !ok {
		return
	}

	err := handler.service.RemovePolicy(r.Context(), schemas.RemovePolicySchema{ID: id})
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrPolicyNotFound: http.StatusNotFound,
		})

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parsePolicyID decodes the named URL parameter into an integer. If the
// parameter is not a valid number, it writes an HTTP 400 Bad Request and
// returns false.
func parsePolicyID(w http.ResponseWriter, r *http.Request, paramName string) (int64, bool) {
	rawID := chi.URLParam(r, paramName)

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		utils.EmitError(w, "invalid policy id", http.StatusBadRequest)
		return 0, false
	}

	return id, true
}
