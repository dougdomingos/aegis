package handler

import (
	"context"
	"net/http"

	"dougdomingos.com/aegis/internal/api/utils"
	"dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
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
