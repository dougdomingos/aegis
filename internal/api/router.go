package api

import (
	"database/sql"
	"net/http"

	"dougdomingos.com/aegis/internal/api/handler"
	"dougdomingos.com/aegis/internal/api/middleware"
	"dougdomingos.com/aegis/internal/service"
	"dougdomingos.com/aegis/internal/store"
	"github.com/go-chi/chi/v5"
)

// routerHandler declares the operation of registering a handler's routes
// into the provided router.
type routerHandler interface {
	RegisterRoutes(r chi.Router)
}

// registerHandlers registers the routes of every provided registrar into the
// shared router.
func registerHandlers(r chi.Router, handlers ...routerHandler) {
	for _, handler := range handlers {
		handler.RegisterRoutes(r)
	}
}

func NewRouter(db *sql.DB) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.Logger)

	registerHandlers(router,
		handler.NewGroupHandler(service.NewGroupService(store.NewGroupStore(db))),
		handler.NewRuleHandler(service.NewRuleService(store.NewRuleStore(db))),
		handler.NewPolicyHandler(service.NewPolicyService(store.NewPolicyStore(db))),
	)

	return router
}
