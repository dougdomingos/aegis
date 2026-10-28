package handler_test

import (
	"context"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/api/handler"
	"dougdomingos.com/aegis/internal/schemas"
	"github.com/go-chi/chi/v5"
)

// ============================================================================
// Service mock
// ============================================================================

// mockMachineService provides an injectable implementation of
// handler.MachineServiceInterface for testing machine handler scenarios.
type mockMachineService struct {
	listFn         func(ctx context.Context) ([]schemas.MachineOutputSchema, error)
	initiateFn     func(ctx context.Context, p schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error)
	authenticateFn func(ctx context.Context, p schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error)
	heartbeatFn    func(ctx context.Context, token string) error
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
