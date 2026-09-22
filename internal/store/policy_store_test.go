package store_test

import (
	"strings"
	"testing"

	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/store"
	_ "modernc.org/sqlite"
)

const queryInitPolicyTable = `
	CREATE TABLE IF NOT EXISTS policies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

// ============================================================================
// Create
// ============================================================================

func TestPolicyStore_Create_WithValidName_CreatesPolicy(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)
	name := "Test Policy"

	createdPolicy, err := store.Create(ctx, name)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if createdPolicy.ID == 0 {
		t.Error("expected non-zero ID for created policy")
	}

	if createdPolicy.Name != name {
		t.Errorf("expected name %q, got %q", name, createdPolicy.Name)
	}

	if createdPolicy.Version != 1 {
		t.Errorf("expected version 1 for created policy, got %d", createdPolicy.Version)
	}

	if createdPolicy.CreatedAt.IsZero() {
		t.Error("expected created_at timestamp to be populated")
	}

	if createdPolicy.UpdatedAt.IsZero() {
		t.Error("expected updated_at timestamp to be populated")
	}
}

func TestPolicyStore_Create_WithDuplicateName_RejectsCreation(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)
	name := "Test Policy"

	if _, err := store.Create(ctx, name); err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}

	_, err := store.Create(ctx, name)
	if err == nil {
		t.Error("expected an error for unique constraint violation, got nil")
	}
}

// ============================================================================
// GetByID & GetByName
// ============================================================================

func TestPolicyStore_GetByID_WithExistingID_ReturnsPolicy(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)
	seededPolicy, _ := store.Create(ctx, "Test Policy")

	policy, err := store.GetByID(ctx, seededPolicy.ID)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if policy == nil {
		t.Fatal("expected policy to be returned, got nil")
	}
	if policy.Name != seededPolicy.Name {
		t.Errorf("expected name %q, got %q", seededPolicy.Name, policy.Name)
	}
	if policy.Version != seededPolicy.Version {
		t.Errorf("expected version %d, got %d", seededPolicy.Version, policy.Version)
	}
}

func TestPolicyStore_GetByID_WithInexistentID_ReturnsNil(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)

	policy, err := store.GetByID(ctx, 999)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if policy != nil {
		t.Errorf("expected nil policy, got %+v", policy)
	}
}

func TestPolicyStore_GetByName_WithExistingName_ReturnsPolicy(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)
	name := "Test Policy"
	if _, err := store.Create(ctx, name); err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}

	policy, err := store.GetByName(ctx, name)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if policy == nil {
		t.Fatal("expected policy to be returned, got nil")
	}
	if policy.Name != name {
		t.Errorf("expected name %q, got %q", name, policy.Name)
	}
}

func TestPolicyStore_GetByName_WithInexistentName_ReturnsNil(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)

	policy, err := store.GetByName(ctx, "Non-existent Policy")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if policy != nil {
		t.Errorf("expected nil policy, got %+v", policy)
	}
}

// ============================================================================
// Exists
// ============================================================================

func TestPolicyStore_Exists_WithExistingPolicy_ReturnsTrue(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)
	name := "Test Policy"

	if _, err := store.Create(ctx, name); err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}

	exists, err := store.Exists(ctx, name)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !exists {
		t.Error("expected exists to be true")
	}
}

func TestPolicyStore_Exists_WithInexistentPolicy_ReturnsFalse(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)

	exists, err := store.Exists(ctx, "Unknown Policy")

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

func TestPolicyStore_Update_WithValidData_UpdatesPolicy(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)
	seededPolicy, _ := store.Create(ctx, "Old Name")

	if seededPolicy.UpdatedAt.IsZero() {
		t.Fatal("expected seeded policy to have updated_at timestamp")
	}

	seededPolicy.Name = "New Name"
	updatedPolicy, err := store.Update(ctx, *seededPolicy)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updatedPolicy.Name != "New Name" {
		t.Errorf("expected updated name to be 'New Name', got %q", updatedPolicy.Name)
	}
	if updatedPolicy.Version != seededPolicy.Version+1 {
		t.Errorf("expected version %d, got %d", seededPolicy.Version+1, updatedPolicy.Version)
	}
	if updatedPolicy.UpdatedAt.Before(seededPolicy.UpdatedAt) {
		t.Errorf("expected updated_at to be refreshed, got %v", updatedPolicy.UpdatedAt)
	}

	fetched, _ := store.GetByID(ctx, seededPolicy.ID)
	if fetched.Name != "New Name" {
		t.Errorf("expected DB record to be 'New Name', got %q", fetched.Name)
	}
}

func TestPolicyStore_Update_WithInexistentID_RejectsUpdate(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)
	target := domain.Policy{ID: 999, Name: "Ghost Policy"}

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

func TestPolicyStore_List_WithSeededPolicies_ReturnsAll(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)
	names := []string{"Policy A", "Policy B", "Policy C"}
	for _, name := range names {
		if _, err := store.Create(ctx, name); err != nil {
			t.Fatalf("failed to seed policy: %v", err)
		}
	}

	policies, err := store.List(ctx)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	policy_count, name_count := len(policies), len(names)
	if policy_count != name_count {
		t.Errorf("expected %d policies, got %d", name_count, policy_count)
	}
}

func TestPolicyStore_List_WithEmptyDB_ReturnsEmptyList(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)

	policies, err := store.List(ctx)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(policies) != 0 {
		t.Errorf("expected 0 policies, got %d", len(policies))
	}
}

// ============================================================================
// Remove
// ============================================================================

func TestPolicyStore_Remove_WithExistingPolicy_DeletesPolicy(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)
	seededPolicy, _ := store.Create(ctx, "Test Policy")

	err := store.Remove(ctx, seededPolicy.ID)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	policy, _ := store.GetByID(ctx, seededPolicy.ID)
	if policy != nil {
		t.Error("expected policy to be removed from DB")
	}
}

func TestPolicyStore_Remove_WithInexistentPolicy_RejectsRemoval(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewPolicyStore, queryInitPolicyTable)

	err := store.Remove(ctx, 999)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}
