package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"dougdomingos.com/aegis/internal/orchestrator/api/handler"
	"dougdomingos.com/aegis/internal/orchestrator/api/middleware"
	"dougdomingos.com/aegis/internal/orchestrator/config"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
	"dougdomingos.com/aegis/internal/orchestrator/service"
	"dougdomingos.com/aegis/internal/orchestrator/store"
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

// Runtime bundles the HTTP handler with the propagation service that shares
// its stores, so the process can start both servers over one configuration.
type Runtime struct {
	// HTTP is the fully wired admin API router.
	HTTP http.Handler

	// Propagation is the service responsible for distributing rule packages to
	// the Sentinels.
	Propagation *service.PropagationService
}

// NewRuntime builds the HTTP router and the propagation service that shares
// the same stores and notifier.
func NewRuntime(db *sql.DB, cfg *config.Config, logger *slog.Logger) (*Runtime, error) {
	router := chi.NewRouter()
	router.Use(middleware.Logger)

	machineStore := store.NewMachineStore(db)
	policyStore := store.NewPolicyStore(db)
	groupStore := store.NewGroupStore(db)
	ruleStore := store.NewRuleStore(db)
	propagationStore := store.NewPropagationStore(db)

	propagationService := service.NewPropagationService(
		machineStore, groupStore, policyStore, ruleStore, propagationStore, logger, time.Now)

	machineService := service.NewMachineService(
		machineStore, propagationStore, cfg.LabKey, cfg.SessionSecret, cfg.SessionTTL, cfg.StalenessGrace, time.Now)

	if err := seedMachines(machineService, cfg); err != nil {
		return nil, err
	}

	registerHandlers(router,
		handler.NewHealthHandler(),
		handler.NewGroupHandler(service.NewGroupService(
			groupStore, machineStore, policyStore, ruleStore, propagationStore, propagationService, cfg.StalenessGrace, time.Now)),
		handler.NewPolicyHandler(service.NewPolicyService(policyStore, propagationService)),
		handler.NewRuleHandler(service.NewRuleService(ruleStore, policyStore, groupStore, propagationService)),
		handler.NewMachineHandler(machineService),
	)

	return &Runtime{HTTP: router, Propagation: propagationService}, nil
}

// NewRouter builds the shared HTTP router, synchronizing the configured
// machine sources into the database before registering their routes.
func NewRouter(db *sql.DB, cfg *config.Config) (http.Handler, error) {
	runtime, err := NewRuntime(db, cfg, nil)
	if err != nil {
		return nil, err
	}

	return runtime.HTTP, nil
}
