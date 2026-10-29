package store_test

import (
	"context"
	"strings"
	"testing"

	"dougdomingos.com/aegis/internal/orchestrator/domain"
	"dougdomingos.com/aegis/internal/orchestrator/store"
	_ "modernc.org/sqlite"
)

// ============================================================================
// Create
// ============================================================================

func TestGroupStore_Create_WithValidName_CreatesGroup(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)
	name := "Test Group"

	createdGroup, err := store.Create(ctx, name)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if createdGroup.ID == 0 {
		t.Error("expected non-zero ID for created group")
	}

	if createdGroup.Name != name {
		t.Errorf("expected name %q, got %q", name, createdGroup.Name)
	}

	if createdGroup.CreatedAt.IsZero() {
		t.Error("expected created_at timestamp to be populated")
	}
}

func TestGroupStore_Create_WithDuplicateName_RejectsCreation(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)
	name := "Test Group"

	_, err := store.Create(ctx, name)
	if err != nil {
		t.Fatalf("failed to seed group: %v", err)
	}

	_, err = store.Create(ctx, name)
	if err == nil {
		t.Error("expected an error for unique constraint violation, got nil")
	}
}

// ============================================================================
// GetByID & GetByName
// ============================================================================

func TestGroupStore_GetByID_WithExistingID_ReturnsGroup(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)
	seededGroup, _ := store.Create(ctx, "Test Group")

	group, err := store.GetByID(ctx, seededGroup.ID)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if group == nil {
		t.Fatal("expected group to be returned, got nil")
	}
	if group.Name != seededGroup.Name {
		t.Errorf("expected name %q, got %q", seededGroup.Name, group.Name)
	}
}

func TestGroupStore_GetByID_WithInexistentID_ReturnsNil(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)

	group, err := store.GetByID(ctx, 999)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if group != nil {
		t.Errorf("expected nil group, got %+v", group)
	}
}

func TestGroupStore_GetByName_WithExistingName_ReturnsGroup(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)
	name := "Test Group"
	if _, err := store.Create(ctx, name); err != nil {
		t.Fatalf("failed to seed group: %v", err)
	}

	group, err := store.GetByName(ctx, name)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if group == nil {
		t.Fatal("expected group to be returned, got nil")
	}
	if group.Name != name {
		t.Errorf("expected name %q, got %q", name, group.Name)
	}
}

func TestGroupStore_GetByName_WithInexistentName_ReturnsNil(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)

	group, err := store.GetByName(ctx, "Non-existent Group")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if group != nil {
		t.Errorf("expected nil group, got %+v", group)
	}
}

// ============================================================================
// Exists
// ============================================================================

func TestGroupStore_Exists_WithExistingGroup_ReturnsTrue(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)
	name := "Test Group"

	if _, err := store.Create(ctx, name); err != nil {
		t.Fatalf("failed to seed group: %v", err)
	}

	exists, err := store.Exists(ctx, name)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !exists {
		t.Error("expected exists to be true")
	}
}

func TestGroupStore_Exists_WithInexistentGroup_ReturnsFalse(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)

	exists, err := store.Exists(ctx, "Unknown Group")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if exists {
		t.Error("expected exists to be false")
	}
}

// ============================================================================
// Update
// ============================================================================

func TestGroupStore_Update_WithValidData_UpdatesGroup(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)
	seededGroup, _ := store.Create(ctx, "Old Name")

	seededGroup.Name = "New Name"
	updatedGroup, err := store.Update(ctx, *seededGroup)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updatedGroup.Name != "New Name" {
		t.Errorf("expected updated name to be 'New Name', got %q", updatedGroup.Name)
	}

	fetched, _ := store.GetByID(ctx, seededGroup.ID)
	if fetched.Name != "New Name" {
		t.Errorf("expected DB record to be 'New Name', got %q", fetched.Name)
	}
}

func TestGroupStore_Update_WithInexistentID_RejectsUpdate(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)
	target := domain.Group{ID: 999, Name: "Ghost Group"}

	_, err := store.Update(ctx, target)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// ============================================================================
// List
// ============================================================================

func TestGroupStore_List_WithSeededGroups_ReturnsAll(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)
	names := []string{"Group A", "Group B", "Group C"}
	for _, name := range names {
		if _, err := store.Create(ctx, name); err != nil {
			t.Fatalf("failed to seed group: %v", err)
		}
	}

	groups, err := store.List(ctx)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	group_count, name_count := len(groups), len(names)
	if group_count != name_count {
		t.Errorf("expected %d groups, got %d", name_count, group_count)
	}
}

func TestGroupStore_List_WithEmptyDB_ReturnsEmptyList(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)

	groups, err := store.List(ctx)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

// ============================================================================
// Remove
// ============================================================================

func TestGroupStore_Remove_WithExistingGroup_DeletesGroup(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)
	name := "Test Group"

	if _, err := store.Create(ctx, name); err != nil {
		t.Fatalf("failed to seed group: %v", err)
	}

	err := store.Remove(ctx, name)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify removal
	exists, _ := store.Exists(ctx, name)
	if exists {
		t.Error("expected group to be removed from DB")
	}
}

func TestGroupStore_Remove_WithInexistentGroup_RejectsRemoval(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewGroupStore)

	err := store.Remove(ctx, "Unknown Group")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// ============================================================================
// LinkToPolicy, UnlinkFromPolicy & Policy projection
// ============================================================================

func TestGroupStore_LinkToPolicy_WithValidNameAndID_SetsPolicyAndName(t *testing.T) {
	ctx, groupStore, policyStore := arrangeGroupPolicyStoreTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)

	linked, err := groupStore.LinkToPolicy(ctx, "lab1", policy.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if linked.PolicyID == nil || *linked.PolicyID != policy.ID {
		t.Errorf("expected policy id %d, got %v", policy.ID, linked.PolicyID)
	}
	if linked.PolicyName != "default" {
		t.Errorf("expected policy name %q, got %q", "default", linked.PolicyName)
	}
}

func TestGroupStore_LinkToPolicy_WithInexistentGroup_ReturnsNotFound(t *testing.T) {
	ctx, groupStore, policyStore := arrangeGroupPolicyStoreTest(t)
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)

	_, err := groupStore.LinkToPolicy(ctx, "unknown", policy.ID)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestGroupStore_LinkToPolicy_WithNewPolicy_ReplacesPreviousPolicy(t *testing.T) {
	ctx, groupStore, policyStore := arrangeGroupPolicyStoreTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	first, _ := policyStore.Create(ctx, "first", domain.WhitelistPolicyType)
	second, _ := policyStore.Create(ctx, "second", domain.BlacklistPolicyType)
	_, _ = groupStore.LinkToPolicy(ctx, "lab1", first.ID)

	linked, err := groupStore.LinkToPolicy(ctx, "lab1", second.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if linked.PolicyID == nil || *linked.PolicyID != second.ID {
		t.Errorf("expected policy id %d, got %v", second.ID, linked.PolicyID)
	}
	if linked.PolicyName != "second" {
		t.Errorf("expected policy name %q, got %q", "second", linked.PolicyName)
	}
}

func TestGroupStore_UnlinkFromPolicy_WithLinkedGroup_ClearsPolicyID(t *testing.T) {
	ctx, groupStore, policyStore := arrangeGroupPolicyStoreTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)
	_, _ = groupStore.LinkToPolicy(ctx, "lab1", policy.ID)

	unlinked, err := groupStore.UnlinkFromPolicy(ctx, "lab1")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if unlinked.PolicyID != nil {
		t.Errorf("expected nil policy id, got %v", *unlinked.PolicyID)
	}
}

func TestGroupStore_GetByName_WithLinkedGroup_ReturnsPolicyName(t *testing.T) {
	ctx, groupStore, policyStore := arrangeGroupPolicyStoreTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)
	_, _ = groupStore.LinkToPolicy(ctx, "lab1", policy.ID)

	found, err := groupStore.GetByName(ctx, "lab1")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if found.PolicyName != "default" {
		t.Errorf("expected policy name %q, got %q", "default", found.PolicyName)
	}
}

func TestGroupStore_GetByName_WithUnlinkedGroup_ReturnsNilPolicy(t *testing.T) {
	ctx, groupStore, _ := arrangeGroupPolicyStoreTest(t)
	_, _ = groupStore.Create(ctx, "lab1")

	found, err := groupStore.GetByName(ctx, "lab1")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if found.PolicyID != nil {
		t.Errorf("expected nil policy id, got %v", *found.PolicyID)
	}
	if found.PolicyName != "" {
		t.Errorf("expected empty policy name, got %q", found.PolicyName)
	}
}

func TestGroupStore_List_WithLinkedGroup_ReturnsPolicyName(t *testing.T) {
	ctx, groupStore, policyStore := arrangeGroupPolicyStoreTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)
	_, _ = groupStore.LinkToPolicy(ctx, "lab1", policy.ID)

	groups, err := groupStore.List(ctx)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(groups) != 1 || groups[0].PolicyName != "default" {
		t.Errorf("expected one group linked to policy 'default', got %+v", groups)
	}
}

func TestGroupStore_Remove_WithLinkedPolicy_KeepsPolicyPersisted(t *testing.T) {
	ctx, groupStore, policyStore := arrangeGroupPolicyStoreTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	policy, _ := policyStore.Create(ctx, "default", domain.WhitelistPolicyType)
	_, _ = groupStore.LinkToPolicy(ctx, "lab1", policy.ID)

	if err := groupStore.Remove(ctx, "lab1"); err != nil {
		t.Fatalf("expected group to be removed, got: %v", err)
	}

	found, err := policyStore.GetByName(ctx, "default")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if found == nil {
		t.Fatal("expected policy to remain persisted after group removal, got nil")
	}
}

// arrangeGroupPolicyStoreTest builds group and policy stores over a migrated
// database.
func arrangeGroupPolicyStoreTest(t *testing.T) (context.Context, *store.GroupStore, *store.PolicyStore) {
	t.Helper()

	_, db := arrangeStoreDB(t)
	return context.Background(), store.NewGroupStore(db), store.NewPolicyStore(db)
}
