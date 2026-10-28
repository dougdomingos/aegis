package handler

import (
	"context"
	"net/http"
	"strings"

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
	Heartbeat(ctx context.Context, token string) error
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
		r.Post("/heartbeat", handler.Heartbeat)
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

// bearerPrefix is the scheme expected in the Authorization header of machine
// requests that present a session token.
const bearerPrefix = "bearer "

// Heartbeat handles connectivity reports from authenticated machines.
func (handler *MachineHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)

	if err := handler.service.Heartbeat(r.Context(), token); err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrSessionTokenInvalid: http.StatusUnauthorized,
		})

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// extractBearerToken returns the bearer credential of the Authorization
// header, tolerating scheme case and surrounding whitespace, or an empty
// string when the header does not carry one.
func extractBearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))

	if len(value) < len(bearerPrefix) || !strings.EqualFold(value[:len(bearerPrefix)], bearerPrefix) {
		return ""
	}

	return strings.TrimSpace(value[len(bearerPrefix):])
}
