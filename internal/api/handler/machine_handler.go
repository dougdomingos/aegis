package handler

import (
	"context"
	"net/http"

	"dougdomingos.com/aegis/internal/api/utils"
	"dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
	"github.com/go-chi/chi/v5"
)

// MachineServiceInterface declares the methods provided by MachineService
// implementations.
type MachineServiceInterface interface {
	ListMachines(ctx context.Context) ([]schemas.MachineOutputSchema, error)
	InitiateAuthentication(ctx context.Context, p schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error)
	Authenticate(ctx context.Context, p schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error)
}

// MachineHandler implements the methods that map HTTP requests into operations
// within the application.
type MachineHandler struct {
	service MachineServiceInterface
}

// NewMachineHandler creates a new MachineHandler instance.
func NewMachineHandler(s MachineServiceInterface) *MachineHandler {
	return &MachineHandler{service: s}
}

// RegisterRoutes registers the machine routes into the provided router.
func (handler *MachineHandler) RegisterRoutes(r chi.Router) {
	r.Route("/machines", func(r chi.Router) {
		r.Get("/", handler.List)
		r.Post("/authenticate/init", handler.Initiate)
		r.Post("/authenticate", handler.Authenticate)
	})
}

// List handles requests to list the configured machines and their status.
func (handler *MachineHandler) List(w http.ResponseWriter, r *http.Request) {
	machines, err := handler.service.ListMachines(r.Context())
	if err != nil {
		utils.MapByError(w, err, nil)
		return
	}

	if machines == nil {
		machines = []schemas.MachineOutputSchema{}
	}

	utils.EncodeToJSON(w, http.StatusOK, machines)
}

// Initiate handles requests to start an authentication challenge for a machine.
func (handler *MachineHandler) Initiate(w http.ResponseWriter, r *http.Request) {
	var payload schemas.InitiateAuthenticationSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.InitiateAuthentication(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrMachineIPRequired: http.StatusBadRequest,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// Authenticate handles requests to authenticate a machine against the system.
func (handler *MachineHandler) Authenticate(w http.ResponseWriter, r *http.Request) {
	var payload schemas.AuthenticateMachineSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.Authenticate(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrMachineIPRequired:  http.StatusBadRequest,
			errors.ErrInvalidCredentials: http.StatusUnauthorized,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}
