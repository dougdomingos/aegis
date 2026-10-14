package api

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"dougdomingos.com/aegis/internal/api/handler"
	"dougdomingos.com/aegis/internal/api/middleware"
	"dougdomingos.com/aegis/internal/config"
	"dougdomingos.com/aegis/internal/domain"
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

func seedMachines(s *service.MachineService, cfg *config.Config) error {
	machineList := make([]domain.Machine, 0, len(cfg.Machines))
	for _, machine := range cfg.Machines {
		machineList = append(machineList, domain.Machine{IP: machine.IP, Name: machine.Name})
	}

	if err := s.SyncMachines(context.Background(), machineList); err != nil {
		return err
	}

	return nil
}

// NewRouter builds the shared HTTP router, synchronizing the configured
// machine sources into the database before registering their routes.
func NewRouter(db *sql.DB, cfg *config.Config) (http.Handler, error) {
	router := chi.NewRouter()
	router.Use(middleware.Logger)

	machineStore := store.NewMachineStore(db)
	policyStore := store.NewPolicyStore(db)

	machineService := service.NewMachineService(machineStore, cfg.LabKey, cfg.SessionSecret, cfg.SessionTTL, time.Now)
	if err := seedMachines(machineService, cfg); err != nil {
		return nil, err
	}

	registerHandlers(router,
		handler.NewGroupHandler(service.NewGroupService(store.NewGroupStore(db))),
		handler.NewPolicyHandler(service.NewPolicyService(policyStore)),
		handler.NewRuleHandler(service.NewRuleService(store.NewRuleStore(db), policyStore)),
		handler.NewMachineHandler(machineService),
	)

	return router, nil
}
