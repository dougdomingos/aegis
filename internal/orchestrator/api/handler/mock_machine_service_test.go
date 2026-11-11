package handler_test

import (
	"context"
	"io"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/common/schemas"
	"dougdomingos.com/aegis/internal/orchestrator/api/handler"
	"github.com/go-chi/chi/v5"
)

// ============================================================================
// Service mock
// ============================================================================

// mockMachineService provides an injectable implementation of
// handler.MachineServiceInterface for testing machine handler scenarios.
type mockMachineService struct {
	listFn                 func(ctx context.Context) ([]schemas.MachineOutputSchema, error)
	initiateFn             func(ctx context.Context, p schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error)
	authenticateFn         func(ctx context.Context, p schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error)
	heartbeatFn            func(ctx context.Context, token string) error
	getPropagationStatusFn func(ctx context.Context, machineID int64) (*schemas.PropagationOutputSchema, error)
	importMachinesFn       func(ctx context.Context, document io.Reader) (*schemas.MachineImportOutputSchema, error)
	removeMachineFn        func(ctx context.Context, machineID int64) error
}

func (m *mockMachineService) ListMachines(ctx context.Context) ([]schemas.MachineOutputSchema, error) {
	return m.listFn(ctx)
}

func (m *mockMachineService) InitiateAuthentication(ctx context.Context, p schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error) {
	return m.initiateFn(ctx, p)
}

func (m *mockMachineService) Authenticate(ctx context.Context, p schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error) {
	return m.authenticateFn(ctx, p)
}

func (m *mockMachineService) GetPropagationStatus(ctx context.Context, machineID int64) (*schemas.PropagationOutputSchema, error) {
	return m.getPropagationStatusFn(ctx, machineID)
}

func (m *mockMachineService) ImportMachines(ctx context.Context, document io.Reader) (*schemas.MachineImportOutputSchema, error) {
	return m.importMachinesFn(ctx, document)
}

func (m *mockMachineService) RemoveMachine(ctx context.Context, machineID int64) error {
	return m.removeMachineFn(ctx, machineID)
}

// ============================================================================
// Router helper
// ============================================================================

// setupMachineRouter builds a router wired with a machine handler backed by the
// provided service mock.
func setupMachineRouter(t *testing.T, svc handler.MachineServiceInterface) *chi.Mux {
	t.Helper()

	r := chi.NewRouter()
	h := handler.NewMachineHandler(svc)
	h.RegisterRoutes(r)

	return r
}

// machineOutput is a small helper to build an output schema in tests.
func machineOutput(id int64, name, ip string) schemas.MachineOutputSchema {
	return schemas.MachineOutputSchema{
		ID:        id,
		Name:      name,
		IP:        ip,
		Status:    "DISCONNECTED",
		CreatedAt: time.Now(),
	}
}

func (m *mockMachineService) Heartbeat(ctx context.Context, token string) error {
	return m.heartbeatFn(ctx, token)
}
