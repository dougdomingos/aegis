package service_test

import (
	"context"
	"errors"
	"testing"

	groupErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
	"dougdomingos.com/aegis/internal/service"
)

// ============================================================================
// CreateGroup
// ============================================================================

func TestGroupService_CreateGroup_WithValidPayload_AcceptsCreation(t *testing.T) {
	ctx, _, service := arrangeTest(t)
	payload := schemas.CreateGroupSchema{Name: "Test Group"}

	createdGroup, err := service.CreateGroup(ctx, payload)

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if createdGroup.ID == 0 {
		t.Error("expected non-zero UUID for created group")
	}

	if createdGroup.Name != payload.Name {
		t.Errorf("expected group name %q, got %q", payload.Name, createdGroup.Name)
	}

	if createdGroup.CreatedAt.IsZero() {
		t.Error("expected group to have creation timestamp")
	}
}

func TestGroupService_CreateGroup_WithDuplicatedName_RejectsCreation(t *testing.T) {
	ctx, store, service := arrangeTest(t)
	payload := schemas.CreateGroupSchema{Name: "Test Group"}

	_, err := store.Create(ctx, "Test Group")
	if err != nil {
		t.Fatalf("failed to seed initial group: %v", err)
	}

	createdGroup, err := service.CreateGroup(ctx, payload)

	if !errors.Is(err, groupErrors.ErrGroupNameAlreadyExists) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNameAlreadyExists, err)
	}

	if createdGroup != nil {
		t.Errorf("expected returned group to be nil on error, got %+v", createdGroup)
	}
}

func TestGroupService_CreateGroup_WithEmptyName_RejectsCreation(t *testing.T) {
	ctx, _, service := arrangeTest(t)
	payload := schemas.CreateGroupSchema{Name: ""}

	createdGroup, err := service.CreateGroup(ctx, payload)

	if !errors.Is(err, groupErrors.ErrGroupNameRequired) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNameRequired, err)
	}

	if createdGroup != nil {
		t.Errorf("expected returned group to be nil on error, got %+v", createdGroup)
	}
}

func TestGroupService_CreateGroup_WhenStoreFails_RejectsCreation(t *testing.T) {
	ctx, store, service := arrangeTest(t)
	expectedErr := errors.New("failed to insert group into database")
	store.StoreErr = expectedErr

	payload := schemas.CreateGroupSchema{Name: "Test Group"}

	if _, err := service.CreateGroup(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// GetGroupByName
// ============================================================================

func TestGroupService_GetGroupByName_WithSeededGroup_ReturnsGroup(t *testing.T) {
	ctx, store, service := arrangeTest(t)
	payload := schemas.GetGroupByNameSchema{Name: "Test Group"}

	if _, err := store.Create(ctx, payload.Name); err != nil {
		t.Fatalf("failed to seed group %q: %v", payload.Name, err)
	}

	group, err := service.GetGroupByName(ctx, payload)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if group == nil {
		t.Errorf("expected group to be returned, got nil")
	} else if group.Name != payload.Name {
		t.Errorf("expected group name to be %q, got %q", payload.Name, group.Name)
	}
}

func TestGroupService_GetGroupByName_WithInexistentGroup_ReturnsNil(t *testing.T) {
	ctx, _, service := arrangeTest(t)
	payload := schemas.GetGroupByNameSchema{Name: "Test Group"}

	group, err := service.GetGroupByName(ctx, payload)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if group != nil {
		t.Errorf("expected group %q to be nil, got %+v", payload.Name, group)
	}
}

func TestGroupService_GetGroupByName_WithEmptyName_RejectsFetch(t *testing.T) {
	ctx, _, service := arrangeTest(t)
	payload := schemas.GetGroupByNameSchema{Name: ""}

	queryResult, err := service.GetGroupByName(ctx, payload)

	if !errors.Is(err, groupErrors.ErrGroupNameRequired) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNameRequired, err)
	}

	if queryResult != nil {
		t.Errorf("expected returned group to be nil on error, got %+v", queryResult)
	}
}

func TestGroupService_CreateGroup_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, store, service := arrangeTest(t)
	expectedErr := errors.New("failed to query database for groups")
	store.StoreErr = expectedErr

	payload := schemas.CreateGroupSchema{Name: "Test Group"}

	if _, err := service.CreateGroup(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// ListAllGroups
// ============================================================================

func TestGroupService_ListAllGroups_WithSeededGroups_ReturnsAllGroups(t *testing.T) {
	ctx, store, service := arrangeTest(t)

	names := []string{"Group 1", "Group 2", "Group 3"}
	for _, name := range names {
		if _, err := store.Create(ctx, name); err != nil {
			t.Fatalf("failed to seed group %q: %v", name, err)
		}
	}

	groups, err := service.ListAllGroups(ctx)
	group_count, name_count := len(groups), len(names)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if group_count != name_count {
		t.Errorf("expected %d groups, got %d", name_count, group_count)
	}
}

func TestGroupService_ListAllGroups_WithNoGroupsPresent_ReturnsEmptyList(t *testing.T) {
	ctx, _, service := arrangeTest(t)

	groups, err := service.ListAllGroups(ctx)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(groups) != 0 {
		t.Errorf("expected no groups, got %d", len(groups))
	}
}

func TestGroupService_ListAllGroups_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, store, service := arrangeTest(t)
	expectedErr := errors.New("failed to query database for groups")
	store.StoreErr = expectedErr

	if _, err := service.ListAllGroups(ctx); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// ChangeGroupName
// ============================================================================

func TestGroupService_ChangeGroupName_WithValidPayload_AcceptsNameChange(t *testing.T) {
	ctx, store, service := arrangeTest(t)
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "Target Group", NewName: "Super Group"}

	if _, err := store.Create(ctx, payload.TargetGroupName); err != nil {
		t.Fatalf("failed to seed group %q: %v", payload.TargetGroupName, err)
	}

	group, err := service.ChangeGroupName(ctx, payload)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if group.Name != payload.NewName {
		t.Errorf("expected group name to be %q, got %q", payload.NewName, group.Name)
	}
}

func TestGroupService_ChangeGroupName_WithEmptyTargetGroupName_RejectsChange(t *testing.T) {
	ctx, _, service := arrangeTest(t)
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "", NewName: "Super Group"}

	if _, err := service.ChangeGroupName(ctx, payload); !errors.Is(err, groupErrors.ErrGroupNameRequired) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNameRequired, err)
	}
}

func TestGroupService_ChangeGroupName_WithEmptyNewName_RejectsChange(t *testing.T) {
	ctx, _, service := arrangeTest(t)
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "Test Group", NewName: ""}

	if _, err := service.ChangeGroupName(ctx, payload); !errors.Is(err, groupErrors.ErrGroupNewNameRequired) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNewNameRequired, err)
	}
}

func TestGroupService_ChangeGroupName_WithInexistentGroup_RejectsChange(t *testing.T) {
	ctx, _, service := arrangeTest(t)
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "Test Group", NewName: "Super Group"}

	if _, err := service.ChangeGroupName(ctx, payload); !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNotFound, err)
	}
}

func TestGroupService_ChangeGroupName_WithDuplicatedNewName_RejectsChange(t *testing.T) {
	ctx, store, service := arrangeTest(t)
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "Test Group", NewName: "Super Group"}
	mocked_groups := map[string]string{
		"target":  payload.TargetGroupName,
		"newName": payload.NewName,
	}

	for _, group_name := range mocked_groups {
		if _, err := store.Create(ctx, group_name); err != nil {
			t.Fatalf("failed to seed group %q: %v", group_name, err)
		}
	}

	if _, err := service.ChangeGroupName(ctx, payload); !errors.Is(err, groupErrors.ErrGroupNameAlreadyExists) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNameAlreadyExists, err)
	}
}

func TestGroupService_ChangeGroupName_WhenStoreFails_RejectsChange(t *testing.T) {
	ctx, store, service := arrangeTest(t)
	expectedErr := errors.New("failed to query database for requested grouṕ")
	store.StoreErr = expectedErr

	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "Test Group", NewName: "Super Group"}

	if _, err := service.ChangeGroupName(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// RemoveGroup
// ============================================================================

func TestGroupService_RemoveGroup_WithExistentGroup_RemovesGroup(t *testing.T) {
	ctx, store, service := arrangeTest(t)
	payload := schemas.RemoveGroupSchema{Name: "Test Group"}

	if _, err := store.Create(ctx, payload.Name); err != nil {
		t.Fatalf("failed to seed group %q: %v", payload.Name, err)
	}

	err := service.RemoveGroup(ctx, payload)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if group, _ := store.GetByName(ctx, payload.Name); group != nil {
		t.Errorf("expected group %q to be nil, got %+v", payload.Name, group)
	}
}

func TestGroupService_RemoveGroup_WithInexistentGroup_RejectsRemoval(t *testing.T) {
	ctx, _, service := arrangeTest(t)
	payload := schemas.RemoveGroupSchema{Name: "Test Group"}

	err := service.RemoveGroup(ctx, payload)

	if !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNotFound, err)
	}
}

func TestGroupService_RemoveGroup_WithEmptyName_RejectsRemoval(t *testing.T) {
	ctx, _, service := arrangeTest(t)
	payload := schemas.RemoveGroupSchema{Name: ""}

	err := service.RemoveGroup(ctx, payload)

	if !errors.Is(err, groupErrors.ErrGroupNameRequired) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNameRequired, err)
	}
}

func TestGroupService_RemoveGroup_WhenStoreFails_RejectsRemoval(t *testing.T) {
	ctx, store, service := arrangeTest(t)
	expectedErr := errors.New("failed to query database for requested grouṕ")
	store.StoreErr = expectedErr

	payload := schemas.RemoveGroupSchema{Name: "Test Group"}

	if err := service.RemoveGroup(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// Helpers
// ============================================================================

// arrangeTest initializes a new service instance with a mocked store provider.
func arrangeTest(t *testing.T) (context.Context, *MockGroupStore, service.GroupService) {
	t.Helper()

	mockStore := NewMockGroupStore()
	ctx := context.Background()
	service := service.NewGroupService(mockStore)

	return ctx, mockStore, *service
}
