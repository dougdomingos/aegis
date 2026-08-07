package handler

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"

	"dougdomingos.com/aegis/internal/api/utils"

	service "dougdomingos.com/aegis/internal/service/group"
	store "dougdomingos.com/aegis/internal/store/group"
)

// GroupHandler implements the methods that map HTTP requests into operations
// within the application.
type GroupHandler struct {
	service *service.GroupService
}

// NewGroupHandler creates a new GroupHandler instance.
func NewGroupHandler(db *sql.DB) *GroupHandler {
	groupStore := store.NewGroupStore(db)
	groupService := service.NewGroupService(groupStore)

	return &GroupHandler{service: groupService}
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
	var payload service.CreateGroupSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.CreateGroup(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			service.ErrNameRequired:      http.StatusBadRequest,
			service.ErrNameAlreadyExists: http.StatusBadRequest,
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
		groups = []service.GroupOutputSchema{}
	}

	utils.EncodeToJSON(w, http.StatusOK, groups)
}

func (handler *GroupHandler) GetByName(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	res, err := handler.service.GetGroupByName(r.Context(), service.GetGroupByNameSchema{Name: name})
	if err != nil {
		utils.MapByError(w, err, nil)
		return
	}

	if res == nil {
		utils.EmitError(w, service.ErrGroupNotFound.Error(), http.StatusNotFound)
		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

func (handler *GroupHandler) Rename(w http.ResponseWriter, r *http.Request) {
	var payload service.ChangeGroupNameSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.ChangeGroupName(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			service.ErrGroupNotFound:           http.StatusNotFound,
			service.ErrTargetGroupNameRequired: http.StatusBadRequest,
			service.ErrNewNameRequired:         http.StatusBadRequest,
			service.ErrNameAlreadyExists:       http.StatusBadRequest,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

func (handler *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	payload := service.RemoveGroup{Name: chi.URLParam(r, "name")}

	err := handler.service.RemoveGroup(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			service.ErrGroupNotFound: http.StatusNotFound,
			service.ErrNameRequired:  http.StatusBadRequest,
		})

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
