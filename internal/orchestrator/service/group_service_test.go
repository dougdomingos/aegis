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

func TestGroupService_LinkPolicy_WithPolicyWithRules_LinksPolicy(t *testing.T) {
	ctx, groupStore, policyStore, ruleStore, svc := arrangeGroupPolicyServiceTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)
	seedRule(t, ctx, ruleStore, domain.NewDomainRule("test.com").WithPolicy(policy.ID).Build())

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

func TestGroupService_LinkPolicy_WithPolicyWithoutRules_ReturnsErrPolicyHasNoRules(t *testing.T) {
	ctx, groupStore, policyStore, _, svc := arrangeGroupPolicyServiceTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	_, _ = policyStore.Create(ctx, "default", domain.WhitelistPolicyType)

	_, err := svc.LinkPolicy(ctx, "lab1", "default")

	if !errors.Is(err, groupErrors.ErrPolicyHasNoRules) {
		t.Errorf("expected %v, got %v", groupErrors.ErrPolicyHasNoRules, err)
	}
	if group.PolicyID != nil {
		t.Errorf("expected group not to be linked, got policy id %d", *group.PolicyID)
	}
}

func TestGroupService_LinkPolicy_CurrentPolicyWithoutRules_ReturnsErrPolicyHasNoRules(t *testing.T) {
	ctx, groupStore, policyStore, ruleStore, svc := arrangeGroupPolicyServiceTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)
	rule := seedRule(t, ctx, ruleStore, domain.NewDomainRule("test.com").WithPolicy(policy.ID).Build())
	_, _ = svc.LinkPolicy(ctx, "lab1", "default")
	_ = ruleStore.Remove(ctx, rule.ID)

	_, err := svc.LinkPolicy(ctx, "lab1", "default")

	if !errors.Is(err, groupErrors.ErrPolicyHasNoRules) {
		t.Errorf("expected %v, got %v", groupErrors.ErrPolicyHasNoRules, err)
	}
	if group.PolicyID == nil || *group.PolicyID != policy.ID {
		t.Errorf("expected group to keep its previous policy %d, got %v", policy.ID, group.PolicyID)
	}
}

func TestGroupService_LinkPolicy_WithInexistentGroup_ReturnsErrGroupNotFound(t *testing.T) {
	ctx, _, policyStore, _, svc := arrangeGroupPolicyServiceTest(t)
	_, _ = policyStore.Create(ctx, "default", domain.WhitelistPolicyType)

	_, err := svc.LinkPolicy(ctx, "unknown", "default")

	if !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected %v, got %v", groupErrors.ErrGroupNotFound, err)
	}
}

func TestGroupService_LinkPolicy_WithInexistentPolicy_ReturnsErrPolicyNotFound(t *testing.T) {
	ctx, groupStore, _, _, svc := arrangeGroupPolicyServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")

	_, err := svc.LinkPolicy(ctx, "lab1", "unknown")

	if !errors.Is(err, groupErrors.ErrPolicyNotFound) {
		t.Errorf("expected %v, got %v", groupErrors.ErrPolicyNotFound, err)
	}
}

func TestGroupService_LinkPolicy_WithExistingPolicy_ReplacesPolicy(t *testing.T) {
	ctx, groupStore, policyStore, ruleStore, svc := arrangeGroupPolicyServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	first, _ := policyStore.Create(ctx, "first", domain.WhitelistPolicyType)
	second, _ := policyStore.Create(ctx, "second", domain.BlacklistPolicyType)
	seedRule(t, ctx, ruleStore, domain.NewDomainRule("first.com").WithPolicy(first.ID).Build())
	seedRule(t, ctx, ruleStore, domain.NewDomainRule("second.com").WithPolicy(second.ID).Build())
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
	ctx, groupStore, policyStore, ruleStore, svc := arrangeGroupPolicyServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)
	seedRule(t, ctx, ruleStore, domain.NewDomainRule("test.com").WithPolicy(policy.ID).Build())
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
	ctx, groupStore, _, _, svc := arrangeGroupPolicyServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")

	_, err := svc.UnlinkPolicy(ctx, "lab1")

	if !errors.Is(err, groupErrors.ErrGroupNotLinkedToPolicy) {
		t.Errorf("expected %v, got %v", groupErrors.ErrGroupNotLinkedToPolicy, err)
	}
}

func TestGroupService_UnlinkPolicy_WithInexistentGroup_ReturnsErrGroupNotFound(t *testing.T) {
	ctx, _, _, _, svc := arrangeGroupPolicyServiceTest(t)

	_, err := svc.UnlinkPolicy(ctx, "unknown")

	if !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected %v, got %v", groupErrors.ErrGroupNotFound, err)
	}
}

// ============================================================================
// Propagation notifications
// ============================================================================

func TestGroupService_LinkMachine_WithNotifier_NotifiesMachine(t *testing.T) {
	ctx, groupStore, machineStore, svc, _, log := arrangeNotifiedGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})

	_, err := svc.LinkMachine(ctx, "lab1", machine.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !logEquals(log, "notify_machine") {
		t.Errorf("expected exactly [notify_machine], got %v", *log)
	}
}

func TestGroupService_UnlinkMachine_WithNotifier_NotifiesMachine(t *testing.T) {
	ctx, groupStore, machineStore, svc, _, log := arrangeNotifiedGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab1", machine.ID)

	*log = (*log)[:0]

	_, err := svc.UnlinkMachine(ctx, "lab1", machine.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !logEquals(log, "notify_machine") {
		t.Errorf("expected exactly [notify_machine], got %v", *log)
	}
}

func TestGroupService_LinkPolicy_WithNotifier_NotifiesGroup(t *testing.T) {
	ctx, groupStore, policyStore, ruleStore, svc, log := arrangeNotifiedGroupPolicyServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)
	seedRule(t, ctx, ruleStore, domain.NewDomainRule("test.com").WithPolicy(policy.ID).Build())

	_, err := svc.LinkPolicy(ctx, "lab1", "default")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !logEquals(log, "notify_group") {
		t.Errorf("expected exactly [notify_group], got %v", *log)
	}
}

func TestGroupService_UnlinkPolicy_WithNotifier_NotifiesGroup(t *testing.T) {
	ctx, groupStore, policyStore, ruleStore, svc, log := arrangeNotifiedGroupPolicyServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)
	seedRule(t, ctx, ruleStore, domain.NewDomainRule("test.com").WithPolicy(policy.ID).Build())
	_, _ = svc.LinkPolicy(ctx, "lab1", "default")

	*log = (*log)[:0]

	_, err := svc.UnlinkPolicy(ctx, "lab1")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !logEquals(log, "notify_group") {
		t.Errorf("expected exactly [notify_group], got %v", *log)
	}
}

func TestGroupService_RemoveGroup_ResolvesBeforeDeleteAndRevokesAfter(t *testing.T) {
	ctx, groupStore, _, svc, notifier, log := arrangeNotifiedGroupMachineServiceTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	notifier.GroupMachineIDs = []int64{7, 8}

	err := svc.RemoveGroup(ctx, schemas.RemoveGroupSchema{Name: "lab1"})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !logEquals(log, "resolve", "remove", "revoke") {
		t.Errorf("expected [resolve remove revoke], got %v", *log)
	}

	if len(notifier.RevokedMachineIDs) != 2 {
		t.Errorf("expected the resolved machine IDs to be revoked, got %v", notifier.RevokedMachineIDs)
	}
	if notifier.RevokedPolicy != nil {
		t.Errorf("expected no policy on a group removal revoke, got %+v", notifier.RevokedPolicy)
	}

	if remaining, _ := groupStore.GetByName(ctx, group.Name); remaining != nil {
		t.Errorf("expected group to be deleted, got %+v", remaining)
	}
}

func TestGroupService_RemoveGroup_WhenResolveFails_DoesNotDelete(t *testing.T) {
	ctx, groupStore, _, svc, notifier, log := arrangeNotifiedGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	notifier.ResolveGroupMachinesErr = errors.New("resolver unavailable")

	err := svc.RemoveGroup(ctx, schemas.RemoveGroupSchema{Name: "lab1"})

	if err == nil {
		t.Fatal("expected an error when resolution fails, got nil")
	}

	if !logEquals(log, "resolve") {
		t.Errorf("expected [resolve] only, got %v", *log)
	}

	if remaining, _ := groupStore.GetByName(ctx, "lab1"); remaining == nil {
		t.Error("expected the group to survive a failed resolution")
	}
}

func TestGroupService_RemoveGroup_WhenDeleteFails_DoesNotRevoke(t *testing.T) {
	ctx, groupStore, _, svc, notifier, log := arrangeNotifiedGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	notifier.GroupMachineIDs = []int64{7, 8}
	groupStore.RemoveErr = errors.New("database unavailable")

	err := svc.RemoveGroup(ctx, schemas.RemoveGroupSchema{Name: "lab1"})

	if err == nil {
		t.Fatal("expected an error when the delete fails, got nil")
	}

	if !logEquals(log, "resolve", "remove") {
		t.Errorf("expected [resolve remove] with no revoke, got %v", *log)
	}
}

// ============================================================================
// ListGroupMachines
// ============================================================================

func TestGroupService_ListGroupMachines_WithMachinesInGroup_ReturnsOnlyThoseMachines(t *testing.T) {
	ctx, groupStore, machineStore, propagationStore, svc := arrangeGroupListMachinesServiceTest(t)
	if _, err := groupStore.Create(ctx, "lab1"); err != nil {
		t.Fatalf("failed to seed group: %v", err)
	}
	if _, err := groupStore.Create(ctx, "lab2"); err != nil {
		t.Fatalf("failed to seed group: %v", err)
	}
	first, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	second, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.12", Name: "lcc1-012"})
	other, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.2.11", Name: "lcc2-011"})
	for _, id := range []int64{first.ID, second.ID} {
		if _, err := svc.LinkMachine(ctx, "lab1", id); err != nil {
			t.Fatalf("failed to link machine %d: %v", id, err)
		}
	}
	if _, err := svc.LinkMachine(ctx, "lab2", other.ID); err != nil {
		t.Fatalf("failed to link machine %d: %v", other.ID, err)
	}
	sentAt := time.Date(2026, 1, 1, 11, 59, 0, 0, time.UTC)
	ackedAt := sentAt.Add(120 * time.Millisecond)
	propagationStore.Rows[first.ID] = []domain.Propagation{
		{MachineID: first.ID, Sequence: 1, Status: domain.PropagationAcked, SentAt: &sentAt, AckedAt: &ackedAt},
	}

	res, err := svc.ListGroupMachines(ctx, "lab1")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 machines, got %d", len(res))
	}
	for _, machine := range res {
		if machine.ID == other.ID {
			t.Errorf("expected machine %d from another group to be excluded", other.ID)
		}
		if machine.ID != first.ID {
			continue
		}
		if machine.Propagation.Status != string(domain.PropagationAcked) {
			t.Errorf("expected propagation status %q, got %q", domain.PropagationAcked, machine.Propagation.Status)
		}
		if machine.Propagation.DurationMS == nil || *machine.Propagation.DurationMS != 120 {
			t.Errorf("expected propagation duration 120ms, got %v", machine.Propagation.DurationMS)
		}
	}
}

func TestGroupService_ListGroupMachines_WithEmptyGroup_ReturnsEmptySlice(t *testing.T) {
	ctx, groupStore, _, _, svc := arrangeGroupListMachinesServiceTest(t)
	if _, err := groupStore.Create(ctx, "lab1"); err != nil {
		t.Fatalf("failed to seed group: %v", err)
	}

	res, err := svc.ListGroupMachines(ctx, "lab1")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res == nil || len(res) != 0 {
		t.Errorf("expected non-nil empty slice, got %v", res)
	}
}

func TestGroupService_ListGroupMachines_WithMachineWithoutPropagation_ReturnsPendingPropagation(t *testing.T) {
	ctx, groupStore, machineStore, _, svc := arrangeGroupListMachinesServiceTest(t)
	if _, err := groupStore.Create(ctx, "lab1"); err != nil {
		t.Fatalf("failed to seed group: %v", err)
	}
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	if _, err := svc.LinkMachine(ctx, "lab1", machine.ID); err != nil {
		t.Fatalf("failed to link machine: %v", err)
	}

	res, err := svc.ListGroupMachines(ctx, "lab1")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 machine, got %d", len(res))
	}
	if res[0].Propagation.Status != string(domain.PropagationPending) {
		t.Errorf("expected propagation status %q, got %q", domain.PropagationPending, res[0].Propagation.Status)
	}
}

func TestGroupService_ListGroupMachines_WithInexistentGroup_ReturnsErrGroupNotFound(t *testing.T) {
	ctx, _, _, _, svc := arrangeGroupListMachinesServiceTest(t)

	res, err := svc.ListGroupMachines(ctx, "ghost")

	if !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected error %q, got %v", groupErrors.ErrGroupNotFound, err)
	}
	if res != nil {
		t.Errorf("expected nil result on error, got %v", res)
	}
}

func TestGroupService_ListGroupMachines_WithBlankName_ReturnsErrGroupNameRequired(t *testing.T) {
	ctx, _, _, _, svc := arrangeGroupListMachinesServiceTest(t)

	res, err := svc.ListGroupMachines(ctx, "  ")

	if !errors.Is(err, groupErrors.ErrGroupNameRequired) {
		t.Errorf("expected error %q, got %v", groupErrors.ErrGroupNameRequired, err)
	}
	if res != nil {
		t.Errorf("expected nil result on error, got %v", res)
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
	ruleStore := NewMockRuleStore()
	now := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	svc := service.NewGroupService(groupStore, machineStore, policyStore, ruleStore, NewMockPropagationStore(), nil, time.Minute, now)

	return context.Background(), groupStore, machineStore, *svc
}

// arrangeGroupPolicyServiceTest initializes a new group service instance with
// mocked group, policy and rule store providers.
func arrangeGroupPolicyServiceTest(t *testing.T) (context.Context, *MockGroupStore, *MockPolicyStore, *MockRuleStore, service.GroupService) {
	t.Helper()

	groupStore := NewMockGroupStore()
	machineStore := NewMockMachineStore()
	policyStore := NewMockPolicyStore()
	ruleStore := NewMockRuleStore()
	now := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	svc := service.NewGroupService(groupStore, machineStore, policyStore, ruleStore, NewMockPropagationStore(), nil, time.Minute, now)

	return context.Background(), groupStore, policyStore, ruleStore, *svc
}

// arrangeNotifiedGroupMachineServiceTest initializes a group service with
// mocked group and machine stores, a propagation notifier, and a shared call
// log wired into both the notifier and the group store's Remove.
func arrangeNotifiedGroupMachineServiceTest(t *testing.T) (context.Context, *MockGroupStore, *MockMachineStore, service.GroupService, *MockPropagationNotifier, *[]string) {
	t.Helper()

	var log []string
	notifier := &MockPropagationNotifier{Log: &log}

	groupStore := NewMockGroupStore()
	groupStore.RemoveLog = &log

	machineStore := NewMockMachineStore()
	policyStore := NewMockPolicyStore()
	ruleStore := NewMockRuleStore()
	now := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	svc := service.NewGroupService(groupStore, machineStore, policyStore, ruleStore, NewMockPropagationStore(), notifier, time.Minute, now)

	return context.Background(), groupStore, machineStore, *svc, notifier, &log
}

// arrangeNotifiedGroupPolicyServiceTest initializes a group service with
// mocked group, policy and rule stores, a propagation notifier, and a shared
// call log.
func arrangeNotifiedGroupPolicyServiceTest(t *testing.T) (context.Context, *MockGroupStore, *MockPolicyStore, *MockRuleStore, service.GroupService, *[]string) {
	t.Helper()

	var log []string
	notifier := &MockPropagationNotifier{Log: &log}

	groupStore := NewMockGroupStore()
	machineStore := NewMockMachineStore()
	policyStore := NewMockPolicyStore()
	ruleStore := NewMockRuleStore()
	now := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	svc := service.NewGroupService(groupStore, machineStore, policyStore, ruleStore, NewMockPropagationStore(), notifier, time.Minute, now)

	return context.Background(), groupStore, policyStore, ruleStore, *svc, &log
}

// arrangeGroupListMachinesServiceTest initializes a group service with mocked
// group, machine and propagation stores, exposing the propagation store so
// tests can seed latest propagation rows.
func arrangeGroupListMachinesServiceTest(t *testing.T) (context.Context, *MockGroupStore, *MockMachineStore, *MockPropagationStore, service.GroupService) {
	t.Helper()

	groupStore := NewMockGroupStore()
	machineStore := NewMockMachineStore()
	propagationStore := NewMockPropagationStore()
	now := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	svc := service.NewGroupService(groupStore, machineStore, NewMockPolicyStore(), NewMockRuleStore(), propagationStore, nil, time.Minute, now)

	return context.Background(), groupStore, machineStore, propagationStore, *svc
}
