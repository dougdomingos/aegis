package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/orchestrator/domain"

	groupErrors "dougdomingos.com/aegis/internal/common/errors"
	machineErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/schemas"
	"dougdomingos.com/aegis/internal/orchestrator/service"
)

// ============================================================================
// CreateGroup
// ============================================================================

func TestGroupService_CreateGroup_WithValidPayload_AcceptsCreation(t *testing.T) {
	ctx, _, service := arrangeGroupServiceTest(t)
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
	ctx, store, service := arrangeGroupServiceTest(t)
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
	ctx, _, service := arrangeGroupServiceTest(t)
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
	ctx, store, service := arrangeGroupServiceTest(t)
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
	ctx, store, service := arrangeGroupServiceTest(t)
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
	ctx, _, service := arrangeGroupServiceTest(t)
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
	ctx, _, service := arrangeGroupServiceTest(t)
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
	ctx, store, service := arrangeGroupServiceTest(t)
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
	ctx, store, service := arrangeGroupServiceTest(t)

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
	ctx, _, service := arrangeGroupServiceTest(t)

	groups, err := service.ListAllGroups(ctx)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(groups) != 0 {
		t.Errorf("expected no groups, got %d", len(groups))
	}
}

func TestGroupService_ListAllGroups_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, store, service := arrangeGroupServiceTest(t)
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
	ctx, store, service := arrangeGroupServiceTest(t)
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
	ctx, _, service := arrangeGroupServiceTest(t)
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "", NewName: "Super Group"}

	if _, err := service.ChangeGroupName(ctx, payload); !errors.Is(err, groupErrors.ErrGroupNameRequired) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNameRequired, err)
	}
}

func TestGroupService_ChangeGroupName_WithEmptyNewName_RejectsChange(t *testing.T) {
	ctx, _, service := arrangeGroupServiceTest(t)
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "Test Group", NewName: ""}

	if _, err := service.ChangeGroupName(ctx, payload); !errors.Is(err, groupErrors.ErrGroupNewNameRequired) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNewNameRequired, err)
	}
}

func TestGroupService_ChangeGroupName_WithInexistentGroup_RejectsChange(t *testing.T) {
	ctx, _, service := arrangeGroupServiceTest(t)
	payload := schemas.ChangeGroupNameSchema{TargetGroupName: "Test Group", NewName: "Super Group"}

	if _, err := service.ChangeGroupName(ctx, payload); !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNotFound, err)
	}
}

func TestGroupService_ChangeGroupName_WithDuplicatedNewName_RejectsChange(t *testing.T) {
	ctx, store, service := arrangeGroupServiceTest(t)
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
	ctx, store, service := arrangeGroupServiceTest(t)
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
	ctx, store, service := arrangeGroupServiceTest(t)
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
	ctx, _, service := arrangeGroupServiceTest(t)
	payload := schemas.RemoveGroupSchema{Name: "Test Group"}

	err := service.RemoveGroup(ctx, payload)

	if !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNotFound, err)
	}
}

func TestGroupService_RemoveGroup_WithEmptyName_RejectsRemoval(t *testing.T) {
	ctx, _, service := arrangeGroupServiceTest(t)
	payload := schemas.RemoveGroupSchema{Name: ""}

	err := service.RemoveGroup(ctx, payload)

	if !errors.Is(err, groupErrors.ErrGroupNameRequired) {
		t.Errorf("expected error %q, got %q", groupErrors.ErrGroupNameRequired, err)
	}
}

func TestGroupService_RemoveGroup_WhenStoreFails_RejectsRemoval(t *testing.T) {
	ctx, store, service := arrangeGroupServiceTest(t)
	expectedErr := errors.New("failed to query database for requested grouṕ")
	store.StoreErr = expectedErr

	payload := schemas.RemoveGroupSchema{Name: "Test Group"}

	if err := service.RemoveGroup(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// LinkMachine & UnlinkMachine
// ============================================================================

func TestGroupService_LinkMachine_WithValidGroupAndMachine_LinksMachine(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})

	res, err := svc.LinkMachine(ctx, "lab1", machine.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res.GroupID == nil || *res.GroupID != group.ID {
		t.Errorf("expected group id %d, got %v", group.ID, res.GroupID)
	}
}

func TestGroupService_LinkMachine_WithInexistentGroup_ReturnsErrGroupNotFound(t *testing.T) {
	ctx, _, machineStore, svc := arrangeGroupMachineServiceTest(t)
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})

	_, err := svc.LinkMachine(ctx, "unknown", machine.ID)

	if !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected %v, got %v", groupErrors.ErrGroupNotFound, err)
	}
}

func TestGroupService_LinkMachine_WithInexistentMachine_ReturnsErrMachineNotFound(t *testing.T) {
	ctx, groupStore, _, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")

	_, err := svc.LinkMachine(ctx, "lab1", 999)

	if !errors.Is(err, machineErrors.ErrMachineNotFound) {
		t.Errorf("expected %v, got %v", machineErrors.ErrMachineNotFound, err)
	}
}

func TestGroupService_LinkMachine_WithMachineInAnotherGroup_MovesMachine(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	second, _ := groupStore.Create(ctx, "lab2")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab1", machine.ID)

	res, err := svc.LinkMachine(ctx, "lab2", machine.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res.GroupID == nil || *res.GroupID != second.ID {
		t.Errorf("expected group id %d, got %v", second.ID, res.GroupID)
	}
}

func TestGroupService_UnlinkMachine_WithLinkedMachine_ClearsGroup(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab1", machine.ID)

	res, err := svc.UnlinkMachine(ctx, "lab1", machine.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res.GroupID != nil {
		t.Errorf("expected nil group id, got %v", *res.GroupID)
	}
}

func TestGroupService_UnlinkMachine_WhenLinkedToAnotherGroup_ReturnsErrMachineNotInGroup(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	_, _ = groupStore.Create(ctx, "lab2")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab2", machine.ID)

	_, err := svc.UnlinkMachine(ctx, "lab1", machine.ID)

	if !errors.Is(err, machineErrors.ErrMachineNotInGroup) {
		t.Errorf("expected %v, got %v", machineErrors.ErrMachineNotInGroup, err)
	}
}

func TestGroupService_GetGroupByName_WithLinkedMachine_IncludesMachineIDs(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab1", machine.ID)

	res, err := svc.GetGroupByName(ctx, schemas.GetGroupByNameSchema{Name: "lab1"})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(res.MachineIDs) != 1 || res.MachineIDs[0] != machine.ID {
		t.Errorf("expected machine ids [%d], got %v", machine.ID, res.MachineIDs)
	}
}

func TestGroupService_ListAllGroups_WithLinkedMachines_IncludesMachineIDs(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab1", machine.ID)

	res, err := svc.ListAllGroups(ctx)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(res) != 1 || len(res[0].MachineIDs) != 1 || res[0].MachineIDs[0] != machine.ID {
		t.Errorf("expected one group linked to machine %d, got %+v", machine.ID, res)
	}
}

// ============================================================================
// LinkPolicy & UnlinkPolicy
// ============================================================================

func TestGroupService_LinkPolicy_WithValidGroupAndPolicy_LinksPolicy(t *testing.T) {
	ctx, groupStore, policyStore, svc := arrangeGroupPolicyServiceTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)

	res, err := svc.LinkPolicy(ctx, "lab1", "default")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res.PolicyID == nil || *res.PolicyID != policy.ID {
		t.Errorf("expected policy id %d, got %v", policy.ID, res.PolicyID)
	}
	if res.PolicyName != "default" {
		t.Errorf("expected policy name %q, got %q", "default", res.PolicyName)
	}
	if group.PolicyID == nil || *group.PolicyID != policy.ID {
		t.Errorf("expected store group linked, got %v", group.PolicyID)
	}
}

func TestGroupService_LinkPolicy_WithInexistentGroup_ReturnsErrGroupNotFound(t *testing.T) {
	ctx, _, policyStore, svc := arrangeGroupPolicyServiceTest(t)
	_, _ = policyStore.Create(ctx, "default", domain.WhitelistPolicyType)

	_, err := svc.LinkPolicy(ctx, "unknown", "default")

	if !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected %v, got %v", groupErrors.ErrGroupNotFound, err)
	}
}

func TestGroupService_LinkPolicy_WithInexistentPolicy_ReturnsErrPolicyNotFound(t *testing.T) {
	ctx, groupStore, _, svc := arrangeGroupPolicyServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")

	_, err := svc.LinkPolicy(ctx, "lab1", "unknown")

	if !errors.Is(err, groupErrors.ErrPolicyNotFound) {
		t.Errorf("expected %v, got %v", groupErrors.ErrPolicyNotFound, err)
	}
}

func TestGroupService_LinkPolicy_WithExistingPolicy_ReplacesPolicy(t *testing.T) {
	ctx, groupStore, policyStore, svc := arrangeGroupPolicyServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	first, _ := policyStore.Create(ctx, "first", domain.WhitelistPolicyType)
	second, _ := policyStore.Create(ctx, "second", domain.BlacklistPolicyType)
	_, _ = svc.LinkPolicy(ctx, "lab1", "first")

	res, err := svc.LinkPolicy(ctx, "lab1", "second")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res.PolicyID == nil || *res.PolicyID != second.ID {
		t.Errorf("expected policy id %d, got %v", second.ID, res.PolicyID)
	}
	if res.PolicyName != "second" {
		t.Errorf("expected policy name %q, got %q", "second", res.PolicyName)
	}
	_ = first
}

func TestGroupService_UnlinkPolicy_WithLinkedGroup_ClearsPolicy(t *testing.T) {
	ctx, groupStore, policyStore, svc := arrangeGroupPolicyServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	_, _ = policyStore.Create(ctx, "default", domain.WhitelistPolicyType)
	_, _ = svc.LinkPolicy(ctx, "lab1", "default")

	res, err := svc.UnlinkPolicy(ctx, "lab1")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res.PolicyID != nil {
		t.Errorf("expected nil policy id, got %v", *res.PolicyID)
	}
	if res.PolicyName != "" {
		t.Errorf("expected empty policy name, got %q", res.PolicyName)
	}
}

func TestGroupService_UnlinkPolicy_WithUnlinkedGroup_ReturnsErrGroupNotLinkedToPolicy(t *testing.T) {
	ctx, groupStore, _, svc := arrangeGroupPolicyServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")

	_, err := svc.UnlinkPolicy(ctx, "lab1")

	if !errors.Is(err, groupErrors.ErrGroupNotLinkedToPolicy) {
		t.Errorf("expected %v, got %v", groupErrors.ErrGroupNotLinkedToPolicy, err)
	}
}

func TestGroupService_UnlinkPolicy_WithInexistentGroup_ReturnsErrGroupNotFound(t *testing.T) {
	ctx, _, _, svc := arrangeGroupPolicyServiceTest(t)

	_, err := svc.UnlinkPolicy(ctx, "unknown")

	if !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected %v, got %v", groupErrors.ErrGroupNotFound, err)
	}
}

// ============================================================================
// Helpers
// ============================================================================

// arrangeGroupServiceTest initializes a new service instance with a mocked store provider.
func arrangeGroupServiceTest(t *testing.T) (context.Context, *MockGroupStore, service.GroupService) {
	t.Helper()

	ctx, groupStore, _, svc := arrangeGroupMachineServiceTest(t)
	return ctx, groupStore, svc
}

// arrangeGroupMachineServiceTest initializes a new group service instance with
// mocked group and machine store providers.
func arrangeGroupMachineServiceTest(t *testing.T) (context.Context, *MockGroupStore, *MockMachineStore, service.GroupService) {
	t.Helper()

	groupStore := NewMockGroupStore()
	machineStore := NewMockMachineStore()
	policyStore := NewMockPolicyStore()
	now := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	svc := service.NewGroupService(groupStore, machineStore, policyStore, time.Minute, now)

	return context.Background(), groupStore, machineStore, *svc
}

// arrangeGroupPolicyServiceTest initializes a new group service instance with
// mocked group and policy store providers.
func arrangeGroupPolicyServiceTest(t *testing.T) (context.Context, *MockGroupStore, *MockPolicyStore, service.GroupService) {
	t.Helper()

	groupStore := NewMockGroupStore()
	machineStore := NewMockMachineStore()
	policyStore := NewMockPolicyStore()
	now := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	svc := service.NewGroupService(groupStore, machineStore, policyStore, time.Minute, now)

	return context.Background(), groupStore, policyStore, *svc
}
