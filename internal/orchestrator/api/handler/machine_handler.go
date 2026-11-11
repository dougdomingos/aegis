package handler

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"strings"

	"dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/schemas"
	"dougdomingos.com/aegis/internal/orchestrator/api/utils"
	"github.com/go-chi/chi/v5"
)

// bearerPrefix is the scheme expected in the Authorization header of machine
// requests that present a session token.
const bearerPrefix = "bearer "

// maxImportBytes is the maximum size accepted for a machine registry import.
const maxImportBytes = 1 << 20

// MachineServiceInterface declares the methods provided by MachineService
// implementations.
type MachineServiceInterface interface {
	ListMachines(ctx context.Context) ([]schemas.MachineOutputSchema, error)
	InitiateAuthentication(ctx context.Context, p schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error)
	Authenticate(ctx context.Context, p schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error)
	Heartbeat(ctx context.Context, token string) error
	GetPropagationStatus(ctx context.Context, machineID int64) (*schemas.PropagationOutputSchema, error)
	ImportMachines(ctx context.Context, document io.Reader) (*schemas.MachineImportOutputSchema, error)
	RemoveMachine(ctx context.Context, machineID int64) error
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
		r.Post("/import", handler.Import)
		r.Get("/{id}/propagation", handler.GetPropagationStatus)
		r.Delete("/{id}", handler.Delete)
		r.Post("/authenticate/init", handler.Initiate)
		r.Post("/authenticate", handler.Authenticate)
		r.Post("/heartbeat", handler.Heartbeat)
	})
}

// List handles requests to list the configured machines and their status.
//
//	@Summary List machines
//	@Tags Machines
//	@ID listMachines
//	@Success 200 {array} schemas.MachineOutputSchema
//	@Router /machines [get]
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

// GetPropagationStatus handles requests to read the latest propagation state of a
// single machine. Machines that never received a message report PENDING with
// HTTP 200; unknown machines return 404.
//
//	@Summary Get machine propagation status
//	@Tags Machines
//	@ID getMachinePropagationStatus
//	@Param id path int true "Machine ID"
//	@Success 200 {object} schemas.PropagationOutputSchema
//	@Failure 400,404 {object} utils.ErrorResponse
//	@Router /machines/{id}/propagation [get]
func (handler *MachineHandler) GetPropagationStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMachineID(w, r, "id")
	if !ok {
		return
	}

	res, err := handler.service.GetPropagationStatus(r.Context(), id)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrMachineNotFound: http.StatusNotFound,
		})
		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
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

// Import handles requests to register machines and their labs from an
// "ip,name,lab" CSV document sent as the request body.
//
//	@Summary Import machines
//	@Tags Machines
//	@ID importMachines
//	@Accept text/csv
//	@Param body body string true "CSV"
//	@Success 200 {object} schemas.MachineImportOutputSchema
//	@Failure 400,413 {object} utils.ErrorResponse
//	@Router /machines/import [post]
func (handler *MachineHandler) Import(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, maxImportBytes)

	res, err := handler.service.ImportMachines(r.Context(), body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if stderrors.As(err, &tooLarge) {
			utils.EmitError(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		utils.MapByError(w, err, map[error]int{
			errors.ErrInvalidMachinesCSV: http.StatusBadRequest,
		})
		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// Delete removes the machine identified in the request path.
//
//	@Summary Delete machine
//	@Tags Machines
//	@ID deleteMachine
//	@Param id path int true "Machine ID"
//	@Success 204
//	@Failure 400,404 {object} utils.ErrorResponse
//	@Router /machines/{id} [delete]
func (handler *MachineHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseMachineID(w, r, "id")
	if !ok {
		return
	}

	if err := handler.service.RemoveMachine(r.Context(), id); err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrMachineNotFound: http.StatusNotFound,
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
