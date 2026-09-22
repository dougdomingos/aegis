package handler_test

import (
	"context"

	"dougdomingos.com/aegis/internal/schemas"
)

// ============================================================================
// Service mock
// ============================================================================

// mockPolicyService provides an injectable implementation of
// handler.PolicyServiceInterface for testing policy handler scenarios.
type mockPolicyService struct {
	createFn  func(ctx context.Context, p schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error)
	getByIDFn func(ctx context.Context, p schemas.GetPolicyByIDSchema) (*schemas.PolicyOutputSchema, error)
	listFn    func(ctx context.Context) ([]schemas.PolicyOutputSchema, error)
	updateFn  func(ctx context.Context, p schemas.UpdatePolicySchema) (*schemas.PolicyOutputSchema, error)
	removeFn  func(ctx context.Context, p schemas.RemovePolicySchema) error
}

func (m *mockPolicyService) CreatePolicy(ctx context.Context, p schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error) {
	return m.createFn(ctx, p)
}

func (m *mockPolicyService) GetPolicyByID(ctx context.Context, p schemas.GetPolicyByIDSchema) (*schemas.PolicyOutputSchema, error) {
	return m.getByIDFn(ctx, p)
}

func (m *mockPolicyService) ListPolicies(ctx context.Context) ([]schemas.PolicyOutputSchema, error) {
	return m.listFn(ctx)
}

func (m *mockPolicyService) UpdatePolicy(ctx context.Context, p schemas.UpdatePolicySchema) (*schemas.PolicyOutputSchema, error) {
	return m.updateFn(ctx, p)
}

func (m *mockPolicyService) RemovePolicy(ctx context.Context, p schemas.RemovePolicySchema) error {
	return m.removeFn(ctx, p)
}
