package handler

import (
	"net/http"

	"dougdomingos.com/aegis/internal/api/utils"
	"dougdomingos.com/aegis/internal/service/group"
	"github.com/go-chi/chi/v5"
)

// GroupHandler implements the methods that map HTTP requests into operations
// within the application.
type GroupHandler struct {
	service group.GroupServiceInterface
}

// NewGroupHandler creates a new GroupHandler instance.
func NewGroupHandler(s group.GroupServiceInterface) *GroupHandler {
	return &GroupHandler{service: s}
}

func (handler *GroupHandler) RegisterGroupRoutes(r chi.Router) {
	r.Route("/groups", func(r chi.Router) {
		r.Post("/", handler.Create)
		r.Get("/", handler.List)
		r.Get("/{name}", handler.GetByName)
		r.Patch("/rename", handler.Rename)
		r.Delete("/{name}", handler.Delete)
	})
}

func (handler *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	var payload group.CreateGroupSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.CreateGroup(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			group.ErrNameRequired:      http.StatusBadRequest,
			group.ErrNameAlreadyExists: http.StatusConflict,
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
		groups = []group.GroupOutputSchema{}
	}

	utils.EncodeToJSON(w, http.StatusOK, groups)
}

func (handler *GroupHandler) GetByName(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	res, err := handler.service.GetGroupByName(r.Context(), group.GetGroupByNameSchema{Name: name})
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			group.ErrNameRequired:  http.StatusBadRequest,
			group.ErrGroupNotFound: http.StatusNotFound,
		})
		return
	}

	if res == nil {
		utils.EmitError(w, group.ErrGroupNotFound.Error(), http.StatusNotFound)
		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

func (handler *GroupHandler) Rename(w http.ResponseWriter, r *http.Request) {
	var payload group.ChangeGroupNameSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.ChangeGroupName(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			group.ErrGroupNotFound:           http.StatusNotFound,
			group.ErrTargetGroupNameRequired: http.StatusBadRequest,
			group.ErrNewNameRequired:         http.StatusBadRequest,
			group.ErrNameAlreadyExists:       http.StatusConflict,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

func (handler *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	payload := group.RemoveGroup{Name: chi.URLParam(r, "name")}

	err := handler.service.RemoveGroup(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			group.ErrGroupNotFound: http.StatusNotFound,
			group.ErrNameRequired:  http.StatusBadRequest,
		})

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
