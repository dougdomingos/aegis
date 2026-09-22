package service_test

import (
	"context"
	"errors"
	"testing"

	"dougdomingos.com/aegis/internal/domain"
	policyErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
	"dougdomingos.com/aegis/internal/service"
)

// ============================================================================
// CreatePolicy
// ============================================================================

func TestPolicyService_CreatePolicy_WithValidPayload_AcceptsCreation(t *testing.T) {
	ctx, _, service := arrangePolicyServiceTest(t)
	payload := schemas.CreatePolicySchema{Name: "Test Policy"}

	createdPolicy, err := service.CreatePolicy(ctx, payload)

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if createdPolicy.ID == 0 {
		t.Error("expected non-zero ID for created policy")
	}

	if createdPolicy.Name != payload.Name {
		t.Errorf("expected policy name %q, got %q", payload.Name, createdPolicy.Name)
	}

	if createdPolicy.Version != 1 {
		t.Errorf("expected version 1 for created policy, got %d", createdPolicy.Version)
	}

	if createdPolicy.CreatedAt.IsZero() {
		t.Error("expected policy to have creation timestamp")
	}

	if createdPolicy.UpdatedAt.IsZero() {
		t.Error("expected policy to have update timestamp")
	}

	mapped := mapOutputToPolicy(*createdPolicy)
	if mapped.Name != payload.Name {
		t.Errorf("expected mapped policy name %q, got %q", payload.Name, mapped.Name)
	}

	if mapped.Version != createdPolicy.Version {
		t.Errorf("expected mapped version %d, got %d", createdPolicy.Version, mapped.Version)
	}
}

func TestPolicyService_CreatePolicy_WithDuplicatedName_RejectsCreation(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	payload := schemas.CreatePolicySchema{Name: "Test Policy"}

	if _, err := store.Create(ctx, payload.Name); err != nil {
		t.Fatalf("failed to seed initial policy: %v", err)
	}

	createdPolicy, err := service.CreatePolicy(ctx, payload)

	if !errors.Is(err, policyErrors.ErrPolicyNameAlreadyExists) {
		t.Errorf("expected error %q, got %q", policyErrors.ErrPolicyNameAlreadyExists, err)
	}

	if createdPolicy != nil {
		t.Errorf("expected returned policy to be nil on error, got %+v", createdPolicy)
	}
}

func TestPolicyService_CreatePolicy_WithEmptyName_RejectsCreation(t *testing.T) {
	ctx, _, service := arrangePolicyServiceTest(t)
	payload := schemas.CreatePolicySchema{Name: ""}

	createdPolicy, err := service.CreatePolicy(ctx, payload)

	if !errors.Is(err, policyErrors.ErrPolicyNameRequired) {
		t.Errorf("expected error %q, got %q", policyErrors.ErrPolicyNameRequired, err)
	}

	if createdPolicy != nil {
		t.Errorf("expected returned policy to be nil on error, got %+v", createdPolicy)
	}
}

func TestPolicyService_CreatePolicy_WhenStoreFails_RejectsCreation(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	expectedErr := errors.New("failed to insert policy into database")
	store.StoreErr = expectedErr

	payload := schemas.CreatePolicySchema{Name: "Test Policy"}

	if _, err := service.CreatePolicy(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// GetPolicyByID
// ============================================================================

func TestPolicyService_GetPolicyByID_WithSeededPolicy_ReturnsPolicy(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	seededPolicy := seedPolicy(t, ctx, store, "Test Policy")
	payload := schemas.GetPolicyByIDSchema{ID: seededPolicy.ID}

	policy, err := service.GetPolicyByID(ctx, payload)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if policy == nil {
		t.Errorf("expected policy to be returned, got nil")
	} else if policy.ID != seededPolicy.ID {
		t.Errorf("expected policy ID %d, got %d", seededPolicy.ID, policy.ID)
	} else if policy.Name != seededPolicy.Name {
		t.Errorf("expected policy name to be %q, got %q", seededPolicy.Name, policy.Name)
	}
}

func TestPolicyService_GetPolicyByID_WithInexistentPolicy_ReturnsNil(t *testing.T) {
	ctx, _, service := arrangePolicyServiceTest(t)
	payload := schemas.GetPolicyByIDSchema{ID: 999}

	policy, err := service.GetPolicyByID(ctx, payload)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if policy != nil {
		t.Errorf("expected policy to be nil, got %+v", policy)
	}
}

func TestPolicyService_GetPolicyByID_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	expectedErr := errors.New("failed to query database for policy")
	store.StoreErr = expectedErr

	payload := schemas.GetPolicyByIDSchema{ID: 1}

	if _, err := service.GetPolicyByID(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// ListPolicies
// ============================================================================

func TestPolicyService_ListPolicies_WithSeededPolicies_ReturnsAllPolicies(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)

	names := []string{"Policy 1", "Policy 2", "Policy 3"}
	for _, name := range names {
		seedPolicy(t, ctx, store, name)
	}

	policies, err := service.ListPolicies(ctx)
	policy_count, name_count := len(policies), len(names)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if policy_count != name_count {
		t.Errorf("expected %d policies, got %d", name_count, policy_count)
	}
}

func TestPolicyService_ListPolicies_WithNoPoliciesPresent_ReturnsEmptyList(t *testing.T) {
	ctx, _, service := arrangePolicyServiceTest(t)

	policies, err := service.ListPolicies(ctx)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(policies) != 0 {
		t.Errorf("expected no policies, got %d", len(policies))
	}
}

func TestPolicyService_ListPolicies_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	expectedErr := errors.New("failed to query database for policies")
	store.StoreErr = expectedErr

	if _, err := service.ListPolicies(ctx); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// UpdatePolicy
// ============================================================================

func TestPolicyService_UpdatePolicy_WithValidName_AcceptsNameChange(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	seededPolicy := seedPolicy(t, ctx, store, "Old Policy Name")
	update := schemas.UpdatePolicySchema{ID: seededPolicy.ID, Name: "New Policy Name"}

	updatedPolicy, err := service.UpdatePolicy(ctx, update)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if updatedPolicy.Name != update.Name {
		t.Errorf("expected policy name to be %q, got %q", update.Name, updatedPolicy.Name)
	}

	if updatedPolicy.Version != seededPolicy.Version+1 {
		t.Errorf("expected version %d, got %d", seededPolicy.Version+1, updatedPolicy.Version)
	}

	if !updatedPolicy.UpdatedAt.After(seededPolicy.UpdatedAt) {
		t.Errorf("expected updated_at to change, got %v", updatedPolicy.UpdatedAt)
	}
}

func TestPolicyService_UpdatePolicy_WithSameName_KeepsVersionUnchanged(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	seededPolicy := seedPolicy(t, ctx, store, "Test Policy")
	update := schemas.UpdatePolicySchema{ID: seededPolicy.ID, Name: seededPolicy.Name}

	updatedPolicy, err := service.UpdatePolicy(ctx, update)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if updatedPolicy.Version != seededPolicy.Version {
		t.Errorf("expected version to stay %d, got %d", seededPolicy.Version, updatedPolicy.Version)
	}

	if !updatedPolicy.UpdatedAt.Equal(seededPolicy.UpdatedAt) {
		t.Errorf("expected updated_at to stay %v, got %v", seededPolicy.UpdatedAt, updatedPolicy.UpdatedAt)
	}
}

func TestPolicyService_UpdatePolicy_WithEmptyName_RejectsChange(t *testing.T) {
	ctx, _, service := arrangePolicyServiceTest(t)
	update := schemas.UpdatePolicySchema{ID: 1, Name: ""}

	if _, err := service.UpdatePolicy(ctx, update); !errors.Is(err, policyErrors.ErrPolicyNameRequired) {
		t.Errorf("expected error %q, got %q", policyErrors.ErrPolicyNameRequired, err)
	}
}

func TestPolicyService_UpdatePolicy_WithInexistentPolicy_RejectsChange(t *testing.T) {
	ctx, _, service := arrangePolicyServiceTest(t)
	update := schemas.UpdatePolicySchema{ID: 999, Name: "New Policy Name"}

	if _, err := service.UpdatePolicy(ctx, update); !errors.Is(err, policyErrors.ErrPolicyNotFound) {
		t.Errorf("expected error %q, got %q", policyErrors.ErrPolicyNotFound, err)
	}
}

func TestPolicyService_UpdatePolicy_WithDuplicatedNewName_RejectsChange(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	seedPolicy(t, ctx, store, "Target Policy")
	seedPolicy(t, ctx, store, "Super Policy")

	target, _ := store.GetByName(ctx, "Target Policy")
	update := schemas.UpdatePolicySchema{ID: target.ID, Name: "Super Policy"}

	if _, err := service.UpdatePolicy(ctx, update); !errors.Is(err, policyErrors.ErrPolicyNameAlreadyExists) {
		t.Errorf("expected error %q, got %q", policyErrors.ErrPolicyNameAlreadyExists, err)
	}
}

func TestPolicyService_UpdatePolicy_WhenStoreFails_RejectsChange(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	expectedErr := errors.New("failed to query database for policy")
	store.StoreErr = expectedErr

	update := schemas.UpdatePolicySchema{ID: 1, Name: "New Policy Name"}

	if _, err := service.UpdatePolicy(ctx, update); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// RemovePolicy
// ============================================================================

func TestPolicyService_RemovePolicy_WithExistentPolicy_RemovesPolicy(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	seededPolicy := seedPolicy(t, ctx, store, "Test Policy")

	err := service.RemovePolicy(ctx, schemas.RemovePolicySchema{ID: seededPolicy.ID})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if policy, _ := store.GetByID(ctx, seededPolicy.ID); policy != nil {
		t.Errorf("expected policy to be deleted, got %+v", policy)
	}
}

func TestPolicyService_RemovePolicy_WithNonExistentPolicy_RejectsRemoval(t *testing.T) {
	ctx, _, service := arrangePolicyServiceTest(t)

	err := service.RemovePolicy(ctx, schemas.RemovePolicySchema{ID: 999})

	if !errors.Is(err, policyErrors.ErrPolicyNotFound) {
		t.Errorf("expected error %q, got %q", policyErrors.ErrPolicyNotFound, err)
	}
}

func TestPolicyService_RemovePolicy_WhenStoreFails_RejectsRemoval(t *testing.T) {
	ctx, store, service := arrangePolicyServiceTest(t)
	expectedErr := errors.New("failed to query database for policy")
	store.StoreErr = expectedErr

	if err := service.RemovePolicy(ctx, schemas.RemovePolicySchema{ID: 1}); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// Helpers
// ============================================================================

// arrangePolicyServiceTest initializes a new service instance with a mocked
// store provider.
func arrangePolicyServiceTest(t *testing.T) (context.Context, *MockPolicyStore, service.PolicyService) {
	t.Helper()

	mockStore := NewMockPolicyStore()
	ctx := context.Background()
	service := service.NewPolicyService(mockStore)

	return ctx, mockStore, *service
}

// seedPolicy creates one policy in the mock store and fails the test on error.
func seedPolicy(t *testing.T, ctx context.Context, store *MockPolicyStore, name string) *domain.Policy {
	t.Helper()

	seededPolicy, err := store.Create(ctx, name)
	if err != nil {
		t.Fatalf("failed to seed policy %q: %v", name, err)
	}

	return seededPolicy
}

// mapOutputToPolicy maps the provided PolicyOutputSchema to a domain.Policy
// instance.
func mapOutputToPolicy(output schemas.PolicyOutputSchema) domain.Policy {
	return domain.Policy{
		ID:        output.ID,
		Name:      output.Name,
		Version:   output.Version,
		CreatedAt: output.CreatedAt,
		UpdatedAt: output.UpdatedAt,
	}
}
