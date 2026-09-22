package service_test

import (
	"context"
	"fmt"
	"time"

	"dougdomingos.com/aegis/internal/domain"
)

// MockPolicyStore provides an in-memory implementation of domain.PolicyStore
// for testing service operations.
//
// Each method also provides an injectable error field for testing scenarios
// where the store provider fails.
type MockPolicyStore struct {
	policies map[int64]*domain.Policy
	StoreErr error
	nextID   int64
}

// NewMockPolicyStore initializes a new MockPolicyStore instance.
func NewMockPolicyStore() *MockPolicyStore {
	return &MockPolicyStore{
		policies: make(map[int64]*domain.Policy),
	}
}

// Create persists a new policy or returns StoreErr if set.
func (mock *MockPolicyStore) Create(ctx context.Context, name string) (*domain.Policy, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	for _, existing := range mock.policies {
		if existing.Name == name {
			return nil, fmt.Errorf("policy %s already exists", name)
		}
	}

	mock.nextID++
	now := time.Now()
	policy := &domain.Policy{
		ID:        mock.nextID,
		Name:      name,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mock.policies[policy.ID] = policy

	return policy, nil
}

// GetByID retrieves a policy by its unique ID or returns StoreErr if set.
func (mock *MockPolicyStore) GetByID(ctx context.Context, id int64) (*domain.Policy, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	policy, exists := mock.policies[id]
	if !exists {
		return nil, nil
	}

	return policy, nil
}

// GetByName retrieves a policy by name or returns StoreErr if set.
func (mock *MockPolicyStore) GetByName(ctx context.Context, name string) (*domain.Policy, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	for _, policy := range mock.policies {
		if policy.Name == name {
			return policy, nil
		}
	}

	return nil, nil
}

// Exists checks whether a policy with the given name exists.
func (mock *MockPolicyStore) Exists(ctx context.Context, name string) (bool, error) {
	if mock.StoreErr != nil {
		return false, mock.StoreErr
	}

	policy, err := mock.GetByName(ctx, name)
	if err != nil {
		return false, err
	}

	return policy != nil, nil
}

// Update modifies an existing policy or returns StoreErr if set.
func (mock *MockPolicyStore) Update(ctx context.Context, policy domain.Policy) (*domain.Policy, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	stored, exists := mock.policies[policy.ID]
	if !exists {
		return nil, fmt.Errorf("policy with id %d not found", policy.ID)
	}

	policy.Version = stored.Version + 1
	policy.UpdatedAt = time.Now()
	mock.policies[policy.ID] = &policy

	return &policy, nil
}

// List returns all stored policies or returns StoreErr if set.
func (mock *MockPolicyStore) List(ctx context.Context) ([]domain.Policy, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	list := make([]domain.Policy, 0, len(mock.policies))
	for _, policy := range mock.policies {
		list = append(list, *policy)
	}

	return list, nil
}

// Remove deletes a policy by id or returns StoreErr if set.
func (mock *MockPolicyStore) Remove(ctx context.Context, id int64) error {
	if mock.StoreErr != nil {
		return mock.StoreErr
	}

	policy, err := mock.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if policy == nil {
		return fmt.Errorf("policy of id %d not found", id)
	}

	delete(mock.policies, policy.ID)
	return nil
}
