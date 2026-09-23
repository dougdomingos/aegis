package store_test

import (
	"strings"
	"testing"

	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/store"
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
