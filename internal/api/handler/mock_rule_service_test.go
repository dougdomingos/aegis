package handler_test

import (
	"context"

	"dougdomingos.com/aegis/internal/schemas"
)

// ============================================================================
// Service mock
// ============================================================================

// mockRuleService provides an injectable implementation of
// handler.RuleServiceInterface for testing rule handler scenarios.
type mockRuleService struct {
	createFn  func(ctx context.Context, p schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error)
	getByIDFn func(ctx context.Context, p schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error)
	listFn    func(ctx context.Context, p schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error)
	updateFn  func(ctx context.Context, p schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error)
	removeFn  func(ctx context.Context, p schemas.RemoveRuleSchema) error
}

func (m *mockRuleService) CreateRule(ctx context.Context, p schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
	return m.createFn(ctx, p)
}

func (m *mockRuleService) GetRuleByID(ctx context.Context, p schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
	return m.getByIDFn(ctx, p)
}

func (m *mockRuleService) ListRules(ctx context.Context, p schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
	return m.listFn(ctx, p)
}

func (m *mockRuleService) UpdateRule(ctx context.Context, p schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
	return m.updateFn(ctx, p)
}

func (m *mockRuleService) RemoveRule(ctx context.Context, p schemas.RemoveRuleSchema) error {
	return m.removeFn(ctx, p)
}
