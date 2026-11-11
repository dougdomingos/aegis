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
	ListGroupMachines(ctx context.Context, groupName string) ([]schemas.MachineOutputSchema, error)
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
		r.Get("/{name}/machines", handler.ListMachines)
		r.Post("/{name}/machines", handler.LinkMachine)
		r.Delete("/{name}/machines/{machineID}", handler.UnlinkMachine)
		r.Post("/{name}/policy", handler.LinkPolicy)
		r.Delete("/{name}/policy", handler.UnlinkPolicy)
		r.Delete("/{name}", handler.Delete)
	})
}

// Create registers a new group within the system.
//
//	@Summary Create group
//	@Tags Groups
//	@ID createGroup
//	@Param body body schemas.CreateGroupSchema true "Group"
//	@Success 201 {object} schemas.GroupOutputSchema
//	@Failure 400,409 {object} utils.ErrorResponse
//	@Router /groups [post]
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

// List returns every group registered within the system.
//
//	@Summary List groups
//	@Tags Groups
//	@ID listGroups
//	@Success 200 {array} schemas.GroupOutputSchema
//	@Router /groups [get]
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

// GetByName retrieves a group by the name provided in the request path.
//
//	@Summary Get group by name
//	@Tags Groups
//	@ID getGroupByName
//	@Param name path string true "Group name"
//	@Success 200 {object} schemas.GroupOutputSchema
//	@Failure 400,404 {object} utils.ErrorResponse
//	@Router /groups/{name} [get]
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

// ListMachines returns the machines linked to the group named in the request
// path, each with its latest propagation.
//
//	@Summary List machines of a group
//	@Tags Groups
//	@ID listGroupMachines
//	@Param name path string true "Group name"
//	@Success 200 {array} schemas.MachineOutputSchema
//	@Failure 400,404 {object} utils.ErrorResponse
//	@Router /groups/{name}/machines [get]
func (handler *GroupHandler) ListMachines(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	machines, err := handler.service.ListGroupMachines(r.Context(), name)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNameRequired: http.StatusBadRequest,
			errors.ErrGroupNotFound:     http.StatusNotFound,
		})
		return
	}

	if machines == nil {
		machines = []schemas.MachineOutputSchema{}
	}

	utils.EncodeToJSON(w, http.StatusOK, machines)
}

// Rename changes the name of the group referenced in the request body.
//
//	@Summary Rename group
//	@Tags Groups
//	@ID renameGroup
//	@Param body body schemas.ChangeGroupNameSchema true "Names"
//	@Success 200 {object} schemas.GroupOutputSchema
//	@Failure 400,404,409 {object} utils.ErrorResponse
//	@Router /groups/rename [patch]
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

// Delete removes the group named in the request path.
//
//	@Summary Delete group
//	@Tags Groups
//	@ID deleteGroup
//	@Param name path string true "Group name"
//	@Success 204
//	@Failure 400,404 {object} utils.ErrorResponse
//	@Router /groups/{name} [delete]
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
//
//	@Summary Link machine to group
//	@Tags Groups
//	@ID linkMachineToGroup
//	@Param name path string true "Group name"
//	@Param body body schemas.LinkMachineToGroupSchema true "Machine"
//	@Success 200 {object} schemas.MachineOutputSchema
//	@Failure 400,404 {object} utils.ErrorResponse
//	@Router /groups/{name}/machines [post]
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
//
//	@Summary Unlink machine from group
//	@Tags Groups
//	@ID unlinkMachineFromGroup
//	@Param name path string true "Group name"
//	@Param machineID path int true "Machine ID"
//	@Success 200 {object} schemas.MachineOutputSchema
//	@Failure 400,404 {object} utils.ErrorResponse
//	@Router /groups/{name}/machines/{machineID} [delete]
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
//
//	@Summary Link policy to group
//	@Tags Groups
//	@ID linkPolicyToGroup
//	@Param name path string true "Group name"
//	@Param body body schemas.LinkPolicyToGroupSchema true "Policy"
//	@Success 200 {object} schemas.GroupOutputSchema
//	@Failure 400,404,409 {object} utils.ErrorResponse
//	@Router /groups/{name}/policy [post]
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
			errors.ErrPolicyHasNoRules:   http.StatusConflict,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// UnlinkPolicy removes the association between the group named in the request
// path and its currently linked policy.
//
//	@Summary Unlink policy from group
//	@Tags Groups
//	@ID unlinkPolicyFromGroup
//	@Param name path string true "Group name"
//	@Success 200 {object} schemas.GroupOutputSchema
//	@Failure 400,404 {object} utils.ErrorResponse
//	@Router /groups/{name}/policy [delete]
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
