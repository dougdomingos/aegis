package api

import (
	"database/sql"
	"net/http"

	"dougdomingos.com/aegis/internal/api/handler"
	"dougdomingos.com/aegis/internal/api/middleware"
	"github.com/go-chi/chi/v5"

	service "dougdomingos.com/aegis/internal/service/group"
	store "dougdomingos.com/aegis/internal/store/group"
)

func NewRouter(db *sql.DB) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.Logger)

	groupStore := store.NewGroupStore(db)
	groupService := service.NewGroupService(groupStore)
	groups := handler.NewGroupHandler(groupService)
	groups.RegisterGroupRoutes(router)

	return router
}
