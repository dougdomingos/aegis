package handler_test

import (
	"context"

	"dougdomingos.com/aegis/internal/schemas"
)

// ============================================================================
// Service mock
// ============================================================================

// mockGroupService provides an injectable implementation of
// handler.GroupServiceInterface for testing group handler scenarios.
type mockGroupService struct {
	createFn    func(ctx context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error)
	listFn      func(ctx context.Context) ([]schemas.GroupOutputSchema, error)
	getByNameFn func(ctx context.Context, p schemas.GetGroupByNameSchema) (*schemas.GroupOutputSchema, error)
	renameFn    func(ctx context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error)
	removeFn    func(ctx context.Context, p schemas.RemoveGroupSchema) error
}

func (m *mockGroupService) CreateGroup(ctx context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
	return m.createFn(ctx, p)
}

func (m *mockGroupService) ListAllGroups(ctx context.Context) ([]schemas.GroupOutputSchema, error) {
	return m.listFn(ctx)
}

func (m *mockGroupService) GetGroupByName(ctx context.Context, p schemas.GetGroupByNameSchema) (*schemas.GroupOutputSchema, error) {
	return m.getByNameFn(ctx, p)
}

func (m *mockGroupService) ChangeGroupName(ctx context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
	return m.renameFn(ctx, p)
}

func (m *mockGroupService) RemoveGroup(ctx context.Context, p schemas.RemoveGroupSchema) error {
	return m.removeFn(ctx, p)
}
