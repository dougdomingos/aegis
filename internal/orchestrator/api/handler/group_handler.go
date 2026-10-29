package handler

import (
	"context"
	"net/http"
	"strconv"

	"dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/schemas"
	"dougdomingos.com/aegis/internal/orchestrator/api/utils"
	"github.com/go-chi/chi/v5"
)

// GroupServiceInterface declares the methods provided by GroupService
// implementations.
type GroupServiceInterface interface {
	CreateGroup(ctx context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error)
	GetGroupByName(ctx context.Context, p schemas.GetGroupByNameSchema) (*schemas.GroupOutputSchema, error)
	ListAllGroups(ctx context.Context) ([]schemas.GroupOutputSchema, error)
	ChangeGroupName(ctx context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error)
	RemoveGroup(ctx context.Context, p schemas.RemoveGroupSchema) error
	LinkMachine(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error)
	UnlinkMachine(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error)
	LinkPolicy(ctx context.Context, groupName, policyName string) (*schemas.GroupOutputSchema, error)
	UnlinkPolicy(ctx context.Context, groupName string) (*schemas.GroupOutputSchema, error)
}

// GroupHandler implements the methods that map HTTP requests into operations
// within the application.
type GroupHandler struct {
	service GroupServiceInterface
}

// NewGroupHandler creates a new GroupHandler instance.
func NewGroupHandler(s GroupServiceInterface) *GroupHandler {
	return &GroupHandler{service: s}
}

// RegisterRoutes registers the group routes into the provided router.
func (handler *GroupHandler) RegisterRoutes(r chi.Router) {
	r.Route("/groups", func(r chi.Router) {
		r.Post("/", handler.Create)
		r.Get("/", handler.List)
		r.Get("/{name}", handler.GetByName)
		r.Patch("/rename", handler.Rename)
		r.Post("/{name}/machines", handler.LinkMachine)
		r.Delete("/{name}/machines/{machineID}", handler.UnlinkMachine)
		r.Post("/{name}/policy", handler.LinkPolicy)
		r.Delete("/{name}/policy", handler.UnlinkPolicy)
		r.Delete("/{name}", handler.Delete)
	})
}

func (handler *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	var payload schemas.CreateGroupSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.CreateGroup(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNameRequired:      http.StatusBadRequest,
			errors.ErrGroupNameAlreadyExists: http.StatusConflict,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusCreated, res)
}

func (handler *GroupHandler) List(w http.ResponseWriter, r *http.Request) {
	groups, err := handler.service.ListAllGroups(r.Context())
	if err != nil {
		utils.MapByError(w, err, nil)
		return
	}

	if groups == nil {
		groups = []schemas.GroupOutputSchema{}
	}

	utils.EncodeToJSON(w, http.StatusOK, groups)
}

func (handler *GroupHandler) GetByName(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	res, err := handler.service.GetGroupByName(r.Context(), schemas.GetGroupByNameSchema{Name: name})
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNameRequired: http.StatusBadRequest,
			errors.ErrGroupNotFound:     http.StatusNotFound,
		})
		return
	}

	if res == nil {
		utils.EmitError(w, errors.ErrGroupNotFound.Error(), http.StatusNotFound)
		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

func (handler *GroupHandler) Rename(w http.ResponseWriter, r *http.Request) {
	var payload schemas.ChangeGroupNameSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.ChangeGroupName(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNotFound:          http.StatusNotFound,
			errors.ErrGroupNameRequired:      http.StatusBadRequest,
			errors.ErrGroupNewNameRequired:   http.StatusBadRequest,
			errors.ErrGroupNameAlreadyExists: http.StatusConflict,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

func (handler *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	payload := schemas.RemoveGroupSchema{Name: chi.URLParam(r, "name")}

	err := handler.service.RemoveGroup(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNotFound:     http.StatusNotFound,
			errors.ErrGroupNameRequired: http.StatusBadRequest,
		})

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// LinkMachine associates the machine named in the request body with the group
// named in the request path.
func (handler *GroupHandler) LinkMachine(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var payload schemas.LinkMachineToGroupSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.LinkMachine(r.Context(), name, payload.MachineID)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNameRequired: http.StatusBadRequest,
			errors.ErrGroupNotFound:     http.StatusNotFound,
			errors.ErrMachineNotFound:   http.StatusNotFound,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// UnlinkMachine removes the association between the machine named in the
// request path and the group named in the request path.
func (handler *GroupHandler) UnlinkMachine(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	machineID, ok := parseMachineID(w, r, "machineID")
	if !ok {
		return
	}

	res, err := handler.service.UnlinkMachine(r.Context(), name, machineID)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNameRequired: http.StatusBadRequest,
			errors.ErrGroupNotFound:     http.StatusNotFound,
			errors.ErrMachineNotFound:   http.StatusNotFound,
			errors.ErrMachineNotInGroup: http.StatusNotFound,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// LinkPolicy associates the policy named in the request body with the group
// named in the request path, replacing any previously linked policy.
func (handler *GroupHandler) LinkPolicy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var payload schemas.LinkPolicyToGroupSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.LinkPolicy(r.Context(), name, payload.PolicyName)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNameRequired:  http.StatusBadRequest,
			errors.ErrPolicyNameRequired: http.StatusBadRequest,
			errors.ErrGroupNotFound:      http.StatusNotFound,
			errors.ErrPolicyNotFound:     http.StatusNotFound,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// UnlinkPolicy removes the association between the group named in the request
// path and its currently linked policy.
func (handler *GroupHandler) UnlinkPolicy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	res, err := handler.service.UnlinkPolicy(r.Context(), name)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNameRequired:      http.StatusBadRequest,
			errors.ErrGroupNotFound:          http.StatusNotFound,
			errors.ErrGroupNotLinkedToPolicy: http.StatusNotFound,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// parseMachineID decodes the named URL parameter into an integer. If the
// parameter is not a valid number, it writes an HTTP 400 Bad Request and
// returns false.
func parseMachineID(w http.ResponseWriter, r *http.Request, paramName string) (int64, bool) {
	rawID := chi.URLParam(r, paramName)

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		utils.EmitError(w, "invalid machine id", http.StatusBadRequest)
		return 0, false
	}

	return id, true
}
