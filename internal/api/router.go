package api

import (
	"database/sql"
	"net/http"

	"dougdomingos.com/aegis/internal/api/handler"
	"dougdomingos.com/aegis/internal/api/middleware"
	"github.com/go-chi/chi/v5"
)

func NewRouter(db *sql.DB) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.Logger)

	groups := handler.NewGroupHandler(db)
	groups.RegisterGroupRoutes(router)

	return router
}
