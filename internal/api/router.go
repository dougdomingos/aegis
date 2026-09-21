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

func NewRouter(db *sql.DB) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.Logger)

	groupStore := store.NewGroupStore(db)
	groupService := service.NewGroupService(groupStore)
	groups := handler.NewGroupHandler(groupService)
	groups.RegisterGroupRoutes(router)

	ruleStore := store.NewRuleStore(db)
	ruleService := service.NewRuleService(ruleStore)
	rules := handler.NewRuleHandler(ruleService)
	rules.RegisterRuleRoutes(router)

	return router
}
